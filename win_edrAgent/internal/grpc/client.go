// Package grpcclient provides gRPC client for Connection Manager communication.
package grpcclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/edr-platform/win-agent/internal/command"
	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/event"
	"github.com/edr-platform/win-agent/internal/logging"
	pb "github.com/edr-platform/win-agent/internal/pb"
)

// Client handles gRPC communication with Connection Manager.
type Client struct {
	cfg    *config.Config
	logger *logging.Logger

	conn          *grpc.ClientConn
	serviceClient EventIngestionServiceClient
	mu            sync.RWMutex

	// Long-lived bidirectional stream for events + commands
	stream   EventIngestionService_StreamEventsClient
	streamMu sync.Mutex

	// State
	connected    atomic.Bool
	reconnecting atomic.Bool
	lastError    error

	// Channels
	batchChan   chan *EventBatch
	commandChan chan *Command
	doneChan    chan struct{}

	// Metrics
	batchesSent   atomic.Uint64
	batchesFailed atomic.Uint64
	bytesTotal    atomic.Uint64

	// Re-enrollment signal: closed once when the server rejects with Unauthenticated
	reEnrollCh   chan struct{}
	reEnrollOnce sync.Once
}

// Command represents a command received from server.
type Command struct {
	ID         string
	Type       string
	Parameters map[string]string
	Priority   int
	ExpiresAt  time.Time
}

// NewClient creates a new gRPC client.
func NewClient(cfg *config.Config, logger *logging.Logger) *Client {
	return &Client{
		cfg:         cfg,
		logger:      logger,
		batchChan:   make(chan *EventBatch, 100),
		commandChan: make(chan *Command, 50),
		doneChan:    make(chan struct{}),
		reEnrollCh:  make(chan struct{}),
	}
}

// ReEnrollSignal returns a channel that is closed when the server rejects
// this agent with Unauthenticated, indicating the agent must wipe its
// local state and re-enroll to obtain a new identity.
func (c *Client) ReEnrollSignal() <-chan struct{} {
	return c.reEnrollCh
}

// Connect establishes connection to Connection Manager.
// It first tries the configured address, then falls back to
// auto-discovered addresses (default gateway) if the primary fails.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil // Already connected
	}

	var transportCreds credentials.TransportCredentials
	if c.cfg.Server.Insecure {
		transportCreds = insecure.NewCredentials()
		c.logger.Warn("Using PLAINTEXT gRPC (no TLS) — for debugging only")
	} else {
		tlsConfig, err := c.loadTLSConfig()
		if err != nil {
			c.lastError = err
			return fmt.Errorf("failed to load TLS config: %w", err)
		}
		transportCreds = credentials.NewTLS(tlsConfig)
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(transportCreds),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	// Build candidate addresses: primary + gateway fallbacks
	candidates := c.resolveServerAddresses()

	var lastErr error
	for _, addr := range candidates {
		c.logger.Infof("Trying server address: %s (insecure=%v)", addr, c.cfg.Server.Insecure)

		dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		conn, err := grpc.DialContext(dialCtx, addr, append(opts, grpc.WithBlock())...)
		cancel()

		if err != nil {
			c.logger.Warnf("Failed to connect to %s: %v", addr, err)
			lastErr = err
			continue
		}

		c.conn = conn
		c.serviceClient = NewEventIngestionServiceClient(conn)
		c.connected.Store(true)
		c.logger.Infof("Connected to server at %s", addr)

		// Persist successful address for next reconnection
		if addr != c.cfg.Server.Address {
			c.logger.Infof("Updating server address from %s → %s", c.cfg.Server.Address, addr)
			c.cfg.Server.Address = addr
		}
		return nil
	}

	c.lastError = lastErr
	return fmt.Errorf("all addresses failed, last error: %w", lastErr)
}

// resolveServerAddresses returns a list of candidate server addresses.
// It starts with the configured address, then adds the default gateway
// with the same port. This makes the agent resilient to network interface
// changes (NAT → Host-Only, DHCP renews, etc.).
func (c *Client) resolveServerAddresses() []string {
	primary := c.cfg.Server.Address
	_, port, err := net.SplitHostPort(primary)
	if err != nil {
		port = "50051"
	}

	candidates := []string{primary}
	seen := map[string]bool{primary: true}

	// 1) Try gateway IPs (the host machine on a VM network)
	for _, gw := range discoverGatewayIPs() {
		addr := net.JoinHostPort(gw, port)
		if !seen[addr] {
			candidates = append(candidates, addr)
			seen[addr] = true
		}
	}

	// 2) Always try localhost as last resort
	for _, host := range []string{"localhost", "127.0.0.1"} {
		addr := net.JoinHostPort(host, port)
		if !seen[addr] {
			candidates = append(candidates, addr)
			seen[addr] = true
		}
	}

	c.logger.Debugf("Server candidates: %v", candidates)
	return candidates
}

// discoverGatewayIPs returns likely gateway IPs by inspecting network
// interfaces. The gateway is typically the .1 address of each non-loopback,
// non-link-local IPv4 subnet the agent is connected to.
func discoverGatewayIPs() []string {
	var gateways []string

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	for _, iface := range ifaces {
		// Skip down, loopback, and virtual interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		// Skip common virtual adapter names
		name := strings.ToLower(iface.Name)
		if strings.Contains(name, "loopback") || strings.Contains(name, "pseudo") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			// Compute .1 of the subnet (typical gateway)
			gw := make(net.IP, len(ip))
			for i := range ip {
				gw[i] = ip[i] & ipNet.Mask[i]
			}
			gw[3] |= 1         // .1
			if !gw.Equal(ip) { // Don't add our own IP
				gateways = append(gateways, gw.String())
			}
		}
	}

	return gateways
}

// loadTLSConfig loads mTLS configuration from certificate files.
func (c *Client) loadTLSConfig() (*tls.Config, error) {
	// Load client certificate
	cert, err := tls.LoadX509KeyPair(c.cfg.Certs.CertPath, c.cfg.Certs.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	// Load CA certificate
	caCert, err := os.ReadFile(c.cfg.Certs.CAPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// Disconnect closes the connection and clears the active stream.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.clearStream()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.serviceClient = nil
		c.connected.Store(false)
		return err
	}
	return nil
}

// IsConnected returns connection status.
func (c *Client) IsConnected() bool {
	return c.connected.Load()
}

// SendBatch queues a proto EventBatch for sending (asynchronous).
func (c *Client) SendBatch(batch *EventBatch) error {
	if !c.connected.Load() {
		return fmt.Errorf("not connected")
	}

	select {
	case c.batchChan <- batch:
		return nil
	default:
		return fmt.Errorf("send queue full")
	}
}

// SendBatchSync sends a proto EventBatch on the active stream synchronously.
// Returns an error if the stream is not established (e.g. not connected).
// Caller should use this for guaranteed delivery with retry (e.g. disk queue processor).
func (c *Client) SendBatchSync(ctx context.Context, batch *EventBatch) error {
	c.streamMu.Lock()
	stream := c.stream
	c.streamMu.Unlock()

	if stream == nil {
		return fmt.Errorf("stream not established")
	}
	return stream.Send(batch)
}

// SendCommandResult sends the command execution result to the server (C2 feedback).
func (c *Client) SendCommandResult(ctx context.Context, res *command.Result, agentID string) error {
	if res == nil {
		return nil
	}
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		c.logger.Warnf("SendCommandResult skipped: not connected")
		return fmt.Errorf("not connected")
	}
	req := pb.NewCommandResultProto(
		res.CommandID,
		agentID,
		res.Status,
		res.Output,
		res.Error,
		res.Duration,
		res.Timestamp,
	)
	out := &emptypb.Empty{}
	err := conn.Invoke(ctx, pb.EventIngestionService_SendCommandResult_FullMethodName, req, out)
	if err != nil {
		c.logger.Warnf("SendCommandResult failed: %v", err)
		return err
	}
	c.logger.Debugf("Command result sent: id=%s status=%s", res.CommandID, res.Status)
	return nil
}

// SendHeartbeat sends a heartbeat to the server via unary Heartbeat RPC.
// This converts the local HeartbeatRequest struct to the proto HeartbeatRequest
// message and invokes the Heartbeat RPC.
func (c *Client) SendHeartbeat(req *HeartbeatRequest) (*HeartbeatResponse, error) {
	c.mu.RLock()
	sc := c.serviceClient
	conn := c.conn
	c.mu.RUnlock()

	if sc == nil || conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Map local status to proto enum
	var protoStatus pb.AgentStatus
	switch req.Status {
	case StatusHealthy:
		protoStatus = pb.AgentStatus_AGENT_STATUS_HEALTHY
	case StatusDegraded:
		protoStatus = pb.AgentStatus_AGENT_STATUS_DEGRADED
	case StatusCritical:
		protoStatus = pb.AgentStatus_AGENT_STATUS_CRITICAL
	case StatusUpdating:
		protoStatus = pb.AgentStatus_AGENT_STATUS_UPDATING
	case StatusIsolated:
		protoStatus = pb.AgentStatus_AGENT_STATUS_ISOLATED
	default:
		protoStatus = pb.AgentStatus_AGENT_STATUS_UNKNOWN
	}

	// Build proto HeartbeatRequest with ALL fields
	protoReq := &pb.HeartbeatRequest{
		AgentId:         req.AgentID,
		Timestamp:       timestamppb.New(req.Timestamp),
		Status:          protoStatus,
		CpuUsage:        float32(req.CPUUsage),
		MemoryUsedMb:    int64(req.MemoryUsedMB),
		MemoryTotalMb:   int64(req.MemoryTotalMB),
		DiskTotalMb:     int64(req.CPUCount), // Repurpose unused field for CPU count
		EventsGenerated: int64(req.EventsGenerated),
		EventsSent:      int64(req.EventsSent),
		QueueDepth:      int32(req.QueueDepth),
		EventsDropped:   int64(req.EventsDropped),
		IpAddresses:     req.IPAddresses,
		AgentVersion:    req.Version,
		CertExpiresAt:   req.CertExpiresAt,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	protoResp, err := sc.Heartbeat(ctx, protoReq)
	if err != nil {
		return nil, fmt.Errorf("heartbeat RPC failed: %w", err)
	}

	resp := &HeartbeatResponse{
		ServerStatus:        protoResp.GetServerStatus().String(),
		HasPendingCommands:  protoResp.GetHasPendingCommands(),
		CertRenewalRequired: protoResp.GetCertRenewalRequired(),
	}
	if protoResp.GetServerTimestamp() != nil {
		resp.AckTimestamp = protoResp.GetServerTimestamp().AsTime()
	}

	return resp, nil
}

// Commands returns channel for receiving commands.
func (c *Client) Commands() <-chan *Command {
	return c.commandChan
}

// RunSender starts the batch sending loop.
func (c *Client) RunSender(ctx context.Context) {
	c.logger.Debug("gRPC sender started")

	for {
		select {
		case <-ctx.Done():
			c.logger.Debug("gRPC sender stopped")
			return

		case batch := <-c.batchChan:
			if err := c.sendBatchInternal(ctx, batch); err != nil {
				c.logger.Errorf("Failed to send batch: %v", err)
				c.batchesFailed.Add(1)
			} else {
				c.batchesSent.Add(1)
				c.bytesTotal.Add(uint64(len(batch.Payload)))
			}
		}
	}
}

// sendBatchInternal sends a proto EventBatch to the server via the long-lived stream when set,
// otherwise opens a short-lived stream for this batch.
func (c *Client) sendBatchInternal(ctx context.Context, batch *EventBatch) error {
	if !c.connected.Load() {
		return fmt.Errorf("not connected")
	}

	c.streamMu.Lock()
	stream := c.stream
	c.streamMu.Unlock()

	if stream != nil {
		if err := stream.Send(batch); err != nil {
			c.clearStream()
			return fmt.Errorf("failed to send batch: %w", err)
		}
		c.logger.Debugf("Batch sent: id=%s events=%d size=%d",
			batch.BatchId, batch.EventCount, len(batch.Payload))
		return nil
	}

	// Fallback: short-lived stream when RunStream has not established one yet
	c.mu.RLock()
	sc := c.serviceClient
	c.mu.RUnlock()
	if sc == nil {
		return fmt.Errorf("service client not initialized")
	}
	stream, err := sc.StreamEvents(ctx)
	if err != nil {
		c.connected.Store(false)
		return fmt.Errorf("failed to open stream: %w", err)
	}
	if err := stream.Send(batch); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}
	if err := stream.CloseSend(); err != nil {
		return fmt.Errorf("failed to close send: %w", err)
	}
	resp, _ := stream.Recv()
	if resp != nil && len(resp.Commands) > 0 {
		for _, cmd := range resp.Commands {
			if cmd == nil {
				continue
			}
			select {
			case c.commandChan <- &Command{
				ID:         cmd.GetCommandId(),
				Type:       cmd.GetType().String(),
				Parameters: cmd.GetParameters(),
				Priority:   int(cmd.GetPriority()),
				ExpiresAt:  commandExpiresAt(cmd),
			}:
			default:
				c.logger.Warn("Command channel full, dropping command")
			}
		}
	}
	c.logger.Debugf("Batch sent: id=%s events=%d size=%d",
		batch.BatchId, batch.EventCount, len(batch.Payload))
	return nil
}

// BuildEventBatchProto builds a proto EventBatch from an internal event.Batch (for StreamClient and tests).
func (c *Client) BuildEventBatchProto(batch *event.Batch) *EventBatch {
	if batch == nil {
		return nil
	}
	comp := CompressionNone
	switch batch.Compression {
	case "snappy":
		comp = CompressionSnappy
	case "gzip":
		comp = CompressionGzip
	}
	return &EventBatch{
		BatchId:     batch.ID,
		AgentId:     batch.AgentID,
		Timestamp:   timestamppb.New(batch.Timestamp),
		Compression: comp,
		Payload:     batch.Payload,
		EventCount:  int32(batch.EventCount),
		Checksum:    batch.Checksum,
		Metadata: map[string]string{
			"timestamp": batch.Timestamp.Format(time.RFC3339),
		},
	}
}

// RunStream establishes a long-lived bidirectional stream, runs a reconnection loop,
// and spawns a goroutine that Recv()s CommandBatch messages and forwards commands to commandChan.
// RunStream returns when ctx is cancelled.
func (c *Client) RunStream(ctx context.Context) {
	c.logger.Debug("gRPC RunStream started")

	backoff := c.cfg.Server.ReconnectDelay
	maxBackoff := c.cfg.Server.MaxReconnectDelay

	for {
		select {
		case <-ctx.Done():
			c.clearStream()
			c.logger.Debug("gRPC RunStream stopped (context)")
			return
		default:
		}

		if !c.connected.Load() {
			time.Sleep(backoff)
			continue
		}

		c.mu.RLock()
		sc := c.serviceClient
		c.mu.RUnlock()
		if sc == nil {
			time.Sleep(backoff)
			continue
		}

		stream, err := sc.StreamEvents(ctx)
		if err != nil {
			// ── Detect server-side rejection for unknown/revoked agents ──
			if st, ok := grpcstatus.FromError(err); ok && st.Code() == codes.Unauthenticated {
				c.logger.Warnf("Server rejected agent: %s — triggering re-enrollment", st.Message())
				c.reEnrollOnce.Do(func() { close(c.reEnrollCh) })
				return // Stop reconnecting — agent must re-enroll
			}
			c.logger.Warnf("StreamEvents failed: %v", err)
			backoff = c.nextBackoff(backoff, maxBackoff)
			time.Sleep(backoff)
			continue
		}

		c.streamMu.Lock()
		c.stream = stream
		c.streamMu.Unlock()

		backoff = c.cfg.Server.ReconnectDelay
		c.logger.Info("Bidirectional stream established")

		recvDone := make(chan struct{})
		var recvErr error
		go func() {
			defer close(recvDone)
			recvErr = c.recvLoop(ctx, stream)
		}()

		select {
		case <-ctx.Done():
			c.clearStream()
			<-recvDone
			return
		case <-recvDone:
			c.clearStream()
			// ── Check if recv got Unauthenticated (server rejected unknown/revoked agent) ──
			if recvErr != nil {
				if st, ok := grpcstatus.FromError(recvErr); ok && st.Code() == codes.Unauthenticated {
					c.logger.Warnf("Server rejected agent (recv): %s — triggering re-enrollment", st.Message())
					c.reEnrollOnce.Do(func() { close(c.reEnrollCh) })
					return // Stop reconnecting — agent must re-enroll
				}
			}
			backoff = c.nextBackoff(backoff, maxBackoff)
			c.logger.Debugf("Stream recv ended; reconnecting in %v", backoff)
			time.Sleep(backoff)
		}
	}
}

// recvLoop continuously calls stream.Recv() and forwards CommandBatch commands to commandChan.
// It returns the error from Recv() so the caller can check for Unauthenticated status.
func (c *Client) recvLoop(ctx context.Context, stream EventIngestionService_StreamEventsClient) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := stream.Recv()
		if err != nil {
			c.logger.Debugf("Stream Recv error: %v", err)
			return err
		}
		if resp == nil {
			continue
		}

		for _, cmd := range resp.Commands {
			if cmd == nil {
				continue
			}
			select {
			case c.commandChan <- &Command{
				ID:         cmd.GetCommandId(),
				Type:       cmd.GetType().String(),
				Parameters: cmd.GetParameters(),
				Priority:   int(cmd.GetPriority()),
				ExpiresAt:  commandExpiresAt(cmd),
			}:
			default:
				c.logger.Warn("Command channel full, dropping command")
			}
		}
	}
}

// commandExpiresAt returns the ExpiresAt time from a proto Command, or zero time if unset.
func commandExpiresAt(cmd *pb.Command) time.Time {
	if cmd == nil {
		return time.Time{}
	}
	ts := cmd.GetExpiresAt()
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

// clearStream sets stream to nil under streamMu so senders stop using a dead stream.
func (c *Client) clearStream() {
	c.streamMu.Lock()
	c.stream = nil
	c.streamMu.Unlock()
}

// nextBackoff returns the next backoff duration (exponential, capped by max).
func (c *Client) nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}

// RunReceiver starts the command receiving loop using a persistent gRPC stream.
// Deprecated: use RunStream for a single long-lived bidirectional stream.
func (c *Client) RunReceiver(ctx context.Context) {
	c.RunStream(ctx)
}

// RunReconnector handles automatic reconnection.
func (c *Client) RunReconnector(ctx context.Context) {
	c.logger.Debug("Reconnector started")

	delay := c.cfg.Server.ReconnectDelay
	maxDelay := c.cfg.Server.MaxReconnectDelay

	for {
		select {
		case <-ctx.Done():
			c.logger.Debug("Reconnector stopped")
			return
		default:
			if !c.connected.Load() && !c.reconnecting.Load() {
				c.reconnecting.Store(true)

				c.logger.Infof("Attempting reconnection in %v...", delay)
				time.Sleep(delay)

				if err := c.Connect(ctx); err != nil {
					c.logger.Warnf("Reconnection failed: %v", err)
					// Exponential backoff
					delay = delay * 2
					if delay > maxDelay {
						delay = maxDelay
					}
				} else {
					delay = c.cfg.Server.ReconnectDelay // Reset delay on success
				}

				c.reconnecting.Store(false)
			}
			time.Sleep(time.Second)
		}
	}
}

// Stats returns client statistics.
func (c *Client) Stats() ClientStats {
	return ClientStats{
		Connected:     c.connected.Load(),
		BatchesSent:   c.batchesSent.Load(),
		BatchesFailed: c.batchesFailed.Load(),
		BytesTotal:    c.bytesTotal.Load(),
	}
}

// ClientStats holds gRPC client statistics.
type ClientStats struct {
	Connected     bool
	BatchesSent   uint64
	BatchesFailed uint64
	BytesTotal    uint64
}
