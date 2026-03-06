// Package grpcclient re-exports the generated gRPC client and message types
// from internal/pb so that callers use a single import path.
//
// Generated from internal/proto/v1/edr.proto. Run `make proto` or scripts/genproto.ps1 to regenerate.
package grpcclient

import (
	"google.golang.org/grpc"

	pb "github.com/edr-platform/win-agent/internal/pb"
)

// EventIngestionServiceClient and constructor (generated).
type EventIngestionServiceClient = pb.EventIngestionServiceClient

// NewEventIngestionServiceClient returns the generated gRPC client for EventIngestionService.
func NewEventIngestionServiceClient(cc grpc.ClientConnInterface) EventIngestionServiceClient {
	return pb.NewEventIngestionServiceClient(cc)
}

// Stream client type (generated).
type EventIngestionService_StreamEventsClient = pb.EventIngestionService_StreamEventsClient

// Message types (generated).
// Note: Command for server commands is pb.Command; internal Command type is in client.go.
type EventBatch = pb.EventBatch
type CommandBatch = pb.CommandBatch
type Compression = pb.Compression
type ServerStatus = pb.ServerStatus
type CommandType = pb.CommandType

// Compression enum values.
const (
	CompressionNone   = pb.Compression_COMPRESSION_NONE
	CompressionGzip   = pb.Compression_COMPRESSION_GZIP
	CompressionSnappy = pb.Compression_COMPRESSION_SNAPPY
)

// Registration types (generated).
type AgentRegistrationRequest  = pb.AgentRegistrationRequest
type AgentRegistrationResponse = pb.AgentRegistrationResponse
type RegistrationStatus         = pb.RegistrationStatus

// RegistrationStatus values.
const (
	RegistrationStatusUnspecified = pb.RegistrationStatus_REGISTRATION_STATUS_UNSPECIFIED
	RegistrationStatusPending     = pb.RegistrationStatus_REGISTRATION_STATUS_PENDING
	RegistrationStatusApproved    = pb.RegistrationStatus_REGISTRATION_STATUS_APPROVED
	RegistrationStatusRejected    = pb.RegistrationStatus_REGISTRATION_STATUS_REJECTED
)
