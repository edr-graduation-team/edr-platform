// Package grpcclient provides bidirectional streaming for event delivery and command reception.
package grpcclient

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edr-platform/win-agent/internal/config"
	"github.com/edr-platform/win-agent/internal/event"
	"github.com/edr-platform/win-agent/internal/logging"
)

// StreamClient handles bidirectional gRPC streaming.
type StreamClient struct {
	client *Client
	logger *logging.Logger
	cfg    *config.Config

	// State
	streaming atomic.Bool
	mu        sync.Mutex

	// Channels
	batchChan   chan *event.Batch
	commandChan chan *Command
	stopChan    chan struct{}

	// Metrics
	batchesSent      atomic.Uint64
	batchesFailed    atomic.Uint64
	commandsReceived atomic.Uint64
	bytesTotal       atomic.Uint64
}

// NewStreamClient creates a new streaming client.
func NewStreamClient(client *Client, cfg *config.Config, logger *logging.Logger) *StreamClient {
	return &StreamClient{
		client:      client,
		logger:      logger,
		cfg:         cfg,
		batchChan:   make(chan *event.Batch, 100),
		commandChan: make(chan *Command, 50),
		stopChan:    make(chan struct{}),
	}
}

// Start begins bidirectional streaming.
func (s *StreamClient) Start(ctx context.Context) error {
	if s.streaming.Load() {
		return nil
	}

	s.logger.Info("Starting bidirectional stream...")
	s.streaming.Store(true)

	// Start sender goroutine
	go s.senderLoop(ctx)

	// Start receiver goroutine
	go s.receiverLoop(ctx)

	s.logger.Info("Bidirectional stream started")
	return nil
}

// Stop stops the streaming client.
func (s *StreamClient) Stop() {
	if !s.streaming.Load() {
		return
	}

	s.logger.Info("Stopping stream client...")
	s.streaming.Store(false)
	close(s.stopChan)

	s.logger.Infof("Stream stats: sent=%d failed=%d commands=%d bytes=%d",
		s.batchesSent.Load(),
		s.batchesFailed.Load(),
		s.commandsReceived.Load(),
		s.bytesTotal.Load())
}

// SendBatch queues a batch for sending.
func (s *StreamClient) SendBatch(batch *event.Batch) error {
	if !s.streaming.Load() {
		return fmt.Errorf("stream not active")
	}

	select {
	case s.batchChan <- batch:
		return nil
	default:
		return fmt.Errorf("batch queue full")
	}
}

// Commands returns the channel for received commands.
func (s *StreamClient) Commands() <-chan *Command {
	return s.commandChan
}

// senderLoop continuously sends batches to the server.
func (s *StreamClient) senderLoop(ctx context.Context) {
	s.logger.Debug("Sender loop started")

	for {
		select {
		case <-ctx.Done():
			s.flushRemaining()
			s.logger.Debug("Sender loop stopped (context)")
			return

		case <-s.stopChan:
			s.flushRemaining()
			s.logger.Debug("Sender loop stopped (signal)")
			return

		case batch := <-s.batchChan:
			s.sendBatch(ctx, batch)
		}
	}
}

// flushRemaining sends any remaining batches before shutdown.
func (s *StreamClient) flushRemaining() {
	timeout := time.After(5 * time.Second)

	for {
		select {
		case batch := <-s.batchChan:
			s.sendBatch(context.Background(), batch)
		case <-timeout:
			return
		default:
			return
		}
	}
}

// sendBatch sends a single batch via the underlying gRPC client.
func (s *StreamClient) sendBatch(ctx context.Context, batch *event.Batch) {
	if batch == nil {
		return
	}

	batch.AgentID = s.cfg.Agent.ID
	pbBatch := s.client.BuildEventBatchProto(batch)
	if pbBatch == nil {
		return
	}

	if err := s.client.sendBatchInternal(ctx, pbBatch); err != nil {
		s.logger.Errorf("Failed to send batch via gRPC: %v", err)
		s.batchesFailed.Add(1)
		return
	}

	s.batchesSent.Add(1)
	s.bytesTotal.Add(uint64(len(batch.Payload)))
	s.logger.Debugf("Batch sent: id=%s events=%d size=%d",
		batch.ID, batch.EventCount, len(batch.Payload))
}

// receiverLoop continuously receives commands from the server via gRPC stream.
func (s *StreamClient) receiverLoop(ctx context.Context) {
	s.logger.Debug("Receiver loop started")

	// Delegate to the client's RunReceiver which handles the actual gRPC stream
	// We just need to monitor for stop signals here
	for {
		select {
		case <-ctx.Done():
			s.logger.Debug("Receiver loop stopped (context)")
			return

		case <-s.stopChan:
			s.logger.Debug("Receiver loop stopped (signal)")
			return

		case cmd := <-s.client.Commands():
			if cmd != nil {
				s.processCommand(cmd)
			}
		}
	}
}

// processCommand handles a received command.
func (s *StreamClient) processCommand(cmd *Command) {
	s.commandsReceived.Add(1)

	s.logger.Infof("Command received: type=%s id=%s", cmd.Type, cmd.ID)

	// Send to command channel
	select {
	case s.commandChan <- cmd:
	default:
		s.logger.Warn("Command channel full, dropping command")
	}
}

// IsStreaming returns whether streaming is active.
func (s *StreamClient) IsStreaming() bool {
	return s.streaming.Load()
}

// Stats returns streaming statistics.
func (s *StreamClient) Stats() StreamStats {
	return StreamStats{
		Streaming:        s.streaming.Load(),
		BatchesSent:      s.batchesSent.Load(),
		BatchesFailed:    s.batchesFailed.Load(),
		CommandsReceived: s.commandsReceived.Load(),
		BytesTotal:       s.bytesTotal.Load(),
		QueueDepth:       len(s.batchChan),
	}
}

// StreamStats holds streaming statistics.
type StreamStats struct {
	Streaming        bool
	BatchesSent      uint64
	BatchesFailed    uint64
	CommandsReceived uint64
	BytesTotal       uint64
	QueueDepth       int
}
