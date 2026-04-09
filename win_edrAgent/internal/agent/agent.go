// Package agent provides the main orchestrator for the EDR Windows Agent.
// It coordinates all components: collectors, batcher, gRPC client, and command handler.
package agent

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/edr-platform/win-agent/internal/command"
	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/enrollment"
	"github.com/edr-platform/win-agent/internal/event"
	grpcclient "github.com/edr-platform/win-agent/internal/grpc"
	"github.com/edr-platform/win-agent/internal/logging"
	pb "github.com/edr-platform/win-agent/internal/pb"
	"github.com/edr-platform/win-agent/internal/queue"
)

// Agent is the main EDR agent orchestrator.
type Agent struct {
	cfg    *config.Config
	logger *logging.Logger

	// Event pipeline
	eventChan chan *event.Event
	batcher   *event.Batcher

	// gRPC and commands
	grpcClient     *grpcclient.Client
	commandHandler *command.Handler
	heartbeat      *grpcclient.Heartbeat

	// Offline disk queue (WAL)
	diskQueue *queue.DiskQueue

	// State tracking
	running     atomic.Bool
	startTime   time.Time
	eventsTotal atomic.Uint64
	eventsSent  atomic.Uint64

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex

	// Config file path for re-enrollment persistence and hot-reload saves.
	configFilePath string

	// configUpdateFn is an optional hook registered by the C2 command handler
	// so it can trigger UpdateConfig() without a direct import cycle.
	configUpdateFn func(newCfg *config.Config) error
}

// New creates a new Agent instance.
func New(cfg *config.Config, logger *logging.Logger) (*Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	eventChan := make(chan *event.Event, cfg.Agent.BufferSize)

	queueDir := cfg.Agent.QueueDir
	if queueDir == "" {
		queueDir = "C:\\ProgramData\\EDR\\queue"
	}
	if err := os.MkdirAll(queueDir, 0700); err != nil {
		return nil, fmt.Errorf("create queue dir: %w", err)
	}
	maxQueueMB := cfg.Agent.MaxQueueSizeMB
	if maxQueueMB <= 0 {
		maxQueueMB = 500
	}
	diskQueue := queue.NewDiskQueue(queueDir, maxQueueMB)

	grpcCli := grpcclient.NewClient(cfg, logger)
	cmdHandler := command.NewHandler(logger, cfg.Server.Address)

	// Wire the gRPC client into the command handler so the isolation watchdog
	// can probe IsConnected() when the stream drops during isolation, and
	// trigger self-healing firewall rule updates if the C2 IP has changed.
	cmdHandler.SetGRPCHealthChecker(grpcCli)

	a := &Agent{
		cfg:            cfg,
		logger:         logger,
		eventChan:      eventChan,
		batcher:        event.NewBatcher(cfg.Agent.BatchSize, cfg.Agent.BatchInterval, cfg.Agent.Compression, logger),
		grpcClient:     grpcCli,
		commandHandler: cmdHandler,
		diskQueue:      diskQueue,
		heartbeat:      grpcclient.NewHeartbeat(cfg, logger),
	}

	return a, nil
}

// Start starts all agent components.
func (a *Agent) Start(ctx context.Context) error {
	if a.running.Load() {
		return fmt.Errorf("agent already running")
	}

	a.ctx, a.cancel = context.WithCancel(ctx)
	a.startTime = time.Now()
	a.running.Store(true)

	a.logger.Info("Starting EDR Agent...")
	a.logger.Infof("Agent ID: %s", a.cfg.Agent.ID)
	a.logger.Infof("Hostname: %s", a.cfg.Agent.Hostname)
	a.logger.Infof("Go Version: %s", runtime.Version())
	a.logger.Infof("OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)

	// ── Security hardening (ACLs, encryption, self-protection, retention) ──
	a.initSecurity()

	// Start event batcher
	a.wg.Add(1)
	go a.runBatcher()

	// Start event processor (reads from batcher, sends to server)
	a.wg.Add(1)
	go a.runSender()

	// Start health reporter
	a.wg.Add(1)
	go a.runHealthReporter()

	// Start platform-specific collectors (ETW on Windows)
	startPlatformCollectors(a.ctx, a.cfg, a.eventChan, a.logger)

	// Attempt initial gRPC connect; background routines start regardless so reconnector can establish connection later.
	if err := a.grpcClient.Connect(a.ctx); err != nil {
		a.logger.Warnf("Initial gRPC connect failed (reconnector will retry): %v", err)
	}

	// Wire heartbeat metrics collectors and start heartbeat loop
	// IMPORTANT: Start AFTER gRPC connect so the first heartbeat doesn't fail with "not connected"
	a.heartbeat.SetMetricsCollectors(
		func() uint64 { return a.eventsTotal.Load() },
		func() uint64 { return a.eventsSent.Load() },
		func() int { return a.diskQueue.FileCount() },
		nil, // events dropped — no filter/rate-limiter integrated yet
	)
	a.heartbeat.Start(a.ctx, a.grpcClient.SendHeartbeat)

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				a.logger.Errorf("Panic recovered in RunReconnector: %v", r)
			}
		}()
		a.grpcClient.RunReconnector(a.ctx)
	}()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				a.logger.Errorf("Panic recovered in RunStream: %v", r)
			}
		}()
		a.grpcClient.RunStream(a.ctx)
	}()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				a.logger.Errorf("Panic recovered in runCommandLoop: %v", r)
			}
		}()
		a.runCommandLoop()
	}()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				a.logger.Errorf("Panic recovered in RunSender: %v", r)
			}
		}()
		a.grpcClient.RunSender(a.ctx)
	}()

	// Start re-enrollment watcher: monitors the gRPC client for Unauthenticated
	// rejections and triggers automatic re-enrollment when detected.
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				a.logger.Errorf("Panic recovered in watchReEnrollSignal: %v", r)
			}
		}()
		a.watchReEnrollSignal()
	}()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				a.logger.Errorf("Panic recovered in runQueueProcessor: %v", r)
			}
		}()
		a.runQueueProcessor()
	}()

	a.logger.Info("Agent started successfully")
	return nil
}

// Stop gracefully stops all agent components.
func (a *Agent) Stop() error {
	if !a.running.Load() {
		return nil
	}

	a.logger.Info("Stopping agent...")
	a.running.Store(false)

	// Cancel context to signal all goroutines
	if a.cancel != nil {
		a.cancel()
	}

	// Disconnect gRPC client
	_ = a.grpcClient.Disconnect()

	// Stop heartbeat
	if a.heartbeat != nil {
		a.heartbeat.Stop()
	}

	// Close event channel to flush remaining events
	close(a.eventChan)

	// Wait for all goroutines with timeout
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		a.logger.Info("All components stopped")
	case <-time.After(10 * time.Second):
		a.logger.Warn("Shutdown timed out, some components may not have stopped cleanly")
	}

	uptime := time.Since(a.startTime)
	a.logger.Infof("Agent uptime: %s", uptime)
	a.logger.Infof("Events processed: %d", a.eventsTotal.Load())
	a.logger.Infof("Events sent: %d", a.eventsSent.Load())

	return nil
}

// SetConfigFilePath sets the path used to persist config changes during re-enrollment
// and hot-reload saves.
func (a *Agent) SetConfigFilePath(path string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.configFilePath = path
}

// SetRestartInfo passes the config file path to the command handler so it can
// relaunch the agent in standalone mode (taskkill /F /PID + start <exe> -config <cfg>).
// Call this immediately after agent.New() in both standalone and service modes.
func (a *Agent) SetRestartInfo(configPath string) {
	a.commandHandler.SetRestartInfo(configPath)
}

// SetConfigUpdateHandler registers a callback function that the command handler
// will invoke when a C2 "UPDATE_CONFIG" or "PUSH_POLICY" command arrives.
// This decouples the command package from the agent package to avoid import cycles.
//
// The fn must NOT block — it should apply the config and return quickly.
// Use UpdateConfig() as the fn in most cases.
func (a *Agent) SetConfigUpdateHandler(fn func(newCfg *config.Config) error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.configUpdateFn = fn
	// Wire it into the command handler immediately.
	a.commandHandler.SetConfigUpdateCallback(fn)
	// Also give the handler a reference to the live config so it can clone it
	// when processing filter policy JSON payloads (params["policy"]).
	a.commandHandler.SetCurrentConfig(a.cfg)
}

// UpdateConfig atomically applies a validated new configuration to the running agent.
//
// Hot-reload behaviour:
//  1. Validates the incoming config.
//  2. Persists the new config to disk (overwrites config.yaml).
//  3. Atomically updates a.cfg under the write lock.
//  4. Resets the event Batcher so new batch-size / batch-interval take effect
//     without restarting the Windows Service.
//
// Collectors (ETW, WMI, Registry, Network) are NOT restarted here because they
// run in goroutines bound to a.ctx — restarting them would require cancelling
// and re-spawning goroutines, which is invasive. Instead, the filter object on
// each collector can be swapped by watching a.cfg under a read lock.
// Full collector reconfiguration is deferred to the next service restart.
func (a *Agent) UpdateConfig(newCfg *config.Config) error {
	if newCfg == nil {
		return fmt.Errorf("UpdateConfig: newCfg must not be nil")
	}

	// 1. Validate before touching anything.
	if err := newCfg.Validate(); err != nil {
		return fmt.Errorf("UpdateConfig: validation failed: %w", err)
	}

	// 2. Persist to disk so the new config survives a service restart.
	a.mu.RLock()
	cfgPath := a.configFilePath
	a.mu.RUnlock()

	if cfgPath == "" {
		cfgPath = `C:\ProgramData\EDR\config\config.yaml`
	}
	if err := newCfg.Save(cfgPath); err != nil {
		return fmt.Errorf("UpdateConfig: failed to persist config: %w", err)
	}

	// 3. Atomic config swap.
	a.mu.Lock()
	oldCfg := a.cfg
	a.cfg = newCfg
	a.mu.Unlock()

	a.logger.Infof("[HotReload] Config updated: server=%s batchSize=%d (was %s/%d)",
		newCfg.Server.Address, newCfg.Agent.BatchSize,
		oldCfg.Server.Address, oldCfg.Agent.BatchSize)

	// 4. Reset batcher with new parameters (non-blocking — batcher is goroutine-safe).
	a.batcher.Reconfigure(newCfg.Agent.BatchSize, newCfg.Agent.BatchInterval, newCfg.Agent.Compression)

	a.logger.Info("[HotReload] Batcher reconfigured — new policy active without service restart")
	return nil
}

// watchReEnrollSignal monitors the gRPC client's ReEnrollSignal channel.
// When the server rejects the agent with Unauthenticated (unknown/revoked),
// this method orchestrates a full self-healing re-enrollment:
//  1. Disconnect from server
//  2. Wipe old certificates from disk
//  3. Clear the agent ID (forces fresh registration)
//  4. Call EnsureEnrolled() to obtain a new identity from the server
//  5. Reconnect with the new certificate
func (a *Agent) watchReEnrollSignal() {
	select {
	case <-a.ctx.Done():
		return
	case <-a.grpcClient.ReEnrollSignal():
		a.logger.Warn("═══ RE-ENROLLMENT TRIGGERED: Server rejected this agent ═══")
	}

	// 1. Disconnect the current (rejected) connection
	a.logger.Info("[Re-Enroll] Step 1/5: Disconnecting from server...")
	_ = a.grpcClient.Disconnect()

	// 2. Wipe old certificates
	a.logger.Info("[Re-Enroll] Step 2/5: Wiping old certificates...")
	for _, path := range []string{a.cfg.Certs.CertPath, a.cfg.Certs.KeyPath} {
		if path != "" {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				a.logger.Warnf("[Re-Enroll] Failed to remove %s: %v", path, err)
			} else {
				a.logger.Infof("[Re-Enroll] Removed: %s", path)
			}
		}
	}

	// 3. Clear agent ID to force fresh registration
	a.logger.Info("[Re-Enroll] Step 3/5: Clearing agent ID...")
	oldID := a.cfg.Agent.ID
	a.cfg.Agent.ID = ""

	// 4. Re-enroll with the server
	a.mu.RLock()
	cfgPath := a.configFilePath
	a.mu.RUnlock()

	a.logger.Info("[Re-Enroll] Step 4/5: Requesting fresh enrollment from server...")
	if err := enrollment.EnsureEnrolled(a.cfg, a.logger, cfgPath); err != nil {
		a.logger.Errorf("[Re-Enroll] FAILED: %v — agent cannot recover, manual intervention required", err)
		a.logger.Errorf("[Re-Enroll] Previous agent ID was: %s", oldID)
		return
	}
	a.logger.Infof("[Re-Enroll] SUCCESS: New agent ID: %s (was: %s)", a.cfg.Agent.ID, oldID)

	// 5. Reconnect with the new certificate
	a.logger.Info("[Re-Enroll] Step 5/5: Reconnecting to server with new identity...")

	// Create a new gRPC client with the updated config (new cert paths + agent ID)
	newClient := grpcclient.NewClient(a.cfg, a.logger)
	if err := newClient.Connect(a.ctx); err != nil {
		a.logger.Errorf("[Re-Enroll] Reconnect failed: %v — will retry via reconnector", err)
	}

	// Swap in the new client
	a.mu.Lock()
	a.grpcClient = newClient
	a.mu.Unlock()

	// Start the stream + reconnector for the new client
	a.wg.Add(2)
	go func() {
		defer a.wg.Done()
		newClient.RunReconnector(a.ctx)
	}()
	go func() {
		defer a.wg.Done()
		newClient.RunStream(a.ctx)
	}()

	// Restart heartbeat with the new client's SendHeartbeat
	// Without this, the old sendFunc closure uses the disconnected client
	a.heartbeat.Stop()
	a.heartbeat.Start(a.ctx, newClient.SendHeartbeat)

	a.logger.Info("═══ RE-ENROLLMENT COMPLETE: Agent is operational with new identity ═══")
}

// SubmitEvent adds an event to the processing pipeline.
func (a *Agent) SubmitEvent(evt *event.Event) {
	if !a.running.Load() {
		return
	}

	select {
	case a.eventChan <- evt:
		a.eventsTotal.Add(1)
	default:
		// Buffer full, drop oldest (log at debug level to avoid spam)
		a.logger.Debug("Event buffer full, dropping event")
	}
}

// runBatcher reads events from channel and creates batches.
func (a *Agent) runBatcher() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			a.logger.Errorf("Panic recovered in runBatcher: %v\n%s", r, buf[:n])
		}
	}()
	a.logger.Debug("Batcher started")

	for {
		select {
		case <-a.ctx.Done():
			if batch := a.batcher.Flush(); batch != nil {
				a.processBatch(batch)
			}
			a.logger.Debug("Batcher stopped")
			return

		case evt, ok := <-a.eventChan:
			if !ok {
				if batch := a.batcher.Flush(); batch != nil {
					a.processBatch(batch)
				}
				return
			}

			if batch := a.batcher.Add(evt); batch != nil {
				a.processBatch(batch)
			}
		}
	}
}

// runSender handles batch sending to server.
func (a *Agent) runSender() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			a.logger.Errorf("Panic recovered in runSender: %v\n%s", r, buf[:n])
		}
	}()
	a.logger.Debug("Sender started")

	ticker := time.NewTicker(a.cfg.Agent.BatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			a.logger.Debug("Sender stopped")
			return

		case <-ticker.C:
			if batch := a.batcher.FlushIfReady(); batch != nil {
				a.processBatch(batch)
			}
		}
	}
}

// processBatch serializes the batch to proto and sends it.
//
// FAST PATH (connection healthy): sends directly via gRPC stream without
// touching the disk. This eliminates 3 disk I/O operations per batch
// (write + read + delete) and is the primary throughput optimization.
//
// SLOW PATH (connection down): falls back to the disk queue (WAL) so
// events are never lost. The queue processor drains these files when
// the connection is restored.
func (a *Agent) processBatch(batch *event.Batch) {
	if batch == nil || len(batch.Events) == 0 {
		return
	}
	if len(batch.Payload) == 0 {
		a.logger.Warn("Batch has no payload bytes, skipping")
		return
	}

	compression := pb.Compression_COMPRESSION_NONE
	switch batch.Compression {
	case "snappy":
		compression = pb.Compression_COMPRESSION_SNAPPY
	case "gzip":
		compression = pb.Compression_COMPRESSION_GZIP
	}

	// Use the exact byte array produced by the Batcher so the checksum remains valid on the server.
	pbBatch := &pb.EventBatch{
		BatchId:     batch.ID,
		AgentId:     a.cfg.Agent.ID,
		Timestamp:   timestamppb.New(batch.Timestamp),
		Compression: compression,
		Payload:     batch.Payload,
		EventCount:  int32(len(batch.Events)),
		Checksum:    batch.Checksum,
		Metadata: map[string]string{
			"timestamp": batch.Timestamp.Format(time.RFC3339),
		},
	}

	// ── FAST PATH: direct send (no disk I/O) ─────────────────────────────────
	// Always attempt a direct send first. SendBatchSync has its own fallback
	// (long-lived stream → short-lived stream). We only fall to disk when
	// the actual send fails, NOT based on conn state checks — because gRPC
	// transitions to Idle between RPCs, making IsConnected() unreliable.
	if err := a.grpcClient.SendBatchSync(a.ctx, pbBatch); err == nil {
		a.eventsSent.Add(uint64(pbBatch.GetEventCount()))
		return
	}

	// ── SLOW PATH: persist to disk for later delivery ────────────────────────
	if err := a.diskQueue.Enqueue(pbBatch); err != nil {
		a.logger.Warnf("Enqueue batch to disk failed: %v", err)
		return
	}
	a.logger.Debugf("Batch enqueued to disk: id=%s events=%d", batch.ID, len(batch.Events))
}

// runQueueProcessor drains the disk queue by sending batches to the server.
//
// With the fast-path in processBatch, this goroutine primarily handles:
//   - Backlog files from when the connection was down
//   - Files that accumulated during agent restart
//
// The processor uses file listing (ReadDir) instead of PeekOldest-per-iteration
// to amortise the directory scan cost. Files are processed sequentially (safe)
// but the list is refreshed in bulk to avoid O(n²) ReadDir calls.
func (a *Agent) runQueueProcessor() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			a.logger.Errorf("Panic recovered in runQueueProcessor: %v\n%s", r, buf[:n])
		}
	}()
	a.logger.Debug("Queue processor started")

	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second
	emptyPoll := 500 * time.Millisecond

	for {
		select {
		case <-a.ctx.Done():
			a.logger.Debug("Queue processor stopped")
			return
		default:
		}

		pbBatch, filename, err := a.diskQueue.PeekOldest()
		if err != nil {
			a.logger.Warnf("Peek queue: %v", err)
			time.Sleep(backoff)
			continue
		}
		if pbBatch == nil || filename == "" {
			// Queue is empty — with the fast-path, this is the normal steady state.
			time.Sleep(emptyPoll)
			continue
		}

		err = a.grpcClient.SendBatchSync(a.ctx, pbBatch)
		if err != nil {
			a.logger.Debugf("Send batch sync failed (will retry): %v", err)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		if err := a.diskQueue.Remove(filename); err != nil {
			a.logger.Warnf("Remove queue file %s: %v", filename, err)
		}
		a.eventsSent.Add(uint64(pbBatch.GetEventCount()))
		a.logger.Debugf("Batch sent and removed: id=%s events=%d", pbBatch.GetBatchId(), pbBatch.GetEventCount())
		backoff = 1 * time.Second
	}
}

// runCommandLoop receives commands from the gRPC client and dispatches them
// asynchronously so the Recv loop is NEVER blocked by command execution.
//
// Architecture:
//   - Incoming commands are dispatched to worker goroutines immediately.
//   - A bounded semaphore (cmdSem, capacity 8) limits concurrent execution
//     to prevent goroutine explosion under rapid-fire commands.
//   - Each worker calls Execute() → SendCommandResult() independently.
//   - The Recv loop remains free to process the next command instantly.
func (a *Agent) runCommandLoop() {
	a.logger.Debug("Command loop started (async dispatch, max 8 workers)")

	// Bounded semaphore: at most 8 commands execute concurrently.
	cmdSem := make(chan struct{}, 8)

	for {
		select {
		case <-a.ctx.Done():
			a.logger.Debug("Command loop stopped")
			return
		case cmd, ok := <-a.grpcClient.Commands():
			if !ok {
				return
			}
			a.logger.Infof("[C2] Command received — dispatching async: id=%s type=%s", cmd.ID, cmd.Type)

			c := &command.Command{
				ID:         cmd.ID,
				Type:       mapProtoCommandType(cmd.Type),
				Parameters: cmd.Parameters,
				Priority:   cmd.Priority,
				ExpiresAt:  cmd.ExpiresAt,
				ReceivedAt: time.Now(),
			}

			// Acquire semaphore slot (blocks only if 8 commands are already running).
			cmdSem <- struct{}{}

			// Dispatch execution + ACK to background goroutine.
			go func(c *command.Command) {
				defer func() { <-cmdSem }() // release slot
				defer func() {
					if r := recover(); r != nil {
						a.logger.Errorf("[C2] Panic in command worker id=%s: %v", c.ID, r)
					}
				}()

				result := a.commandHandler.Execute(a.ctx, c)
				if result != nil {
					a.logger.Infof("[C2] Command result: id=%s status=%s duration=%v", result.CommandID, result.Status, result.Duration)
					if err := a.grpcClient.SendCommandResult(a.ctx, result, a.cfg.Agent.ID); err != nil {
						a.logger.Warnf("[C2] SendCommandResult failed for id=%s: %v", result.CommandID, err)
					}
				}
			}(c)
		}
	}
}

// mapProtoCommandType maps proto CommandType enum string names to internal command types.
// Proto enum names are like "COMMAND_TYPE_TERMINATE_PROCESS"; we strip the prefix.
// For RUN_CMD (enum value 9, not in generated code), the proto returns "9".
func mapProtoCommandType(protoType string) command.CommandType {
	switch protoType {
	case "COMMAND_TYPE_TERMINATE_PROCESS":
		return command.CmdTerminateProcess
	case "COMMAND_TYPE_COLLECT_FORENSICS":
		return command.CmdCollectForensics
	case "COMMAND_TYPE_ISOLATE":
		return command.CmdIsolateNetwork
	case "COMMAND_TYPE_UNISOLATE":
		return command.CmdUnisolateNetwork
	case "COMMAND_TYPE_RESTART_SERVICE":
		return command.CmdRestartService
	case "COMMAND_TYPE_UPDATE_AGENT":
		return command.CmdUpdateAgent
	case "COMMAND_TYPE_UPDATE_CONFIG":
		return command.CmdUpdateConfig
	case "COMMAND_TYPE_ADJUST_RATE":
		return command.CmdAdjustRate
	case "COMMAND_TYPE_RESTART", "10": // Machine reboot (enum value 10)
		return command.CmdRestart
	case "COMMAND_TYPE_SHUTDOWN", "11": // Machine shutdown (enum value 11)
		return command.CmdShutdown
	case "9": // RUN_CMD enum value 9 (not in generated proto code)
		return command.CmdRunCommand
	default:
		// Fall through: try using the raw string (e.g., "TERMINATE_PROCESS")
		return command.CommandType(protoType)
	}
}

// runHealthReporter periodically logs health metrics.
func (a *Agent) runHealthReporter() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			a.logger.Errorf("Panic recovered in runHealthReporter: %v\n%s", r, buf[:n])
		}
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.reportHealth()
		}
	}
}

// reportHealth logs current health metrics.
func (a *Agent) reportHealth() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	a.logger.Infof("Health: events=%d sent=%d goroutines=%d mem=%dMB",
		a.eventsTotal.Load(),
		a.eventsSent.Load(),
		runtime.NumGoroutine(),
		memStats.Alloc/1024/1024,
	)
}

// GetStats returns current agent statistics.
func (a *Agent) GetStats() Stats {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return Stats{
		AgentID:     a.cfg.Agent.ID,
		Hostname:    a.cfg.Agent.Hostname,
		Version:     "1.0.0", // TODO: Get from build
		Uptime:      time.Since(a.startTime),
		EventsTotal: a.eventsTotal.Load(),
		EventsSent:  a.eventsSent.Load(),
		QueueDepth:  len(a.eventChan),
		MemoryMB:    memStats.Alloc / 1024 / 1024,
		Goroutines:  runtime.NumGoroutine(),
	}
}

// Stats holds agent statistics.
type Stats struct {
	AgentID     string
	Hostname    string
	Version     string
	Uptime      time.Duration
	EventsTotal uint64
	EventsSent  uint64
	QueueDepth  int
	MemoryMB    uint64
	Goroutines  int
}
