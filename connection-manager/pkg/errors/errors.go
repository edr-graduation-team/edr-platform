// Package errors provides standardized error handling utilities for gRPC services.
// All errors are properly mapped to gRPC status codes with detailed error information.
package errors

import (
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
)

// Error represents a structured error with code, message, and metadata.
type Error struct {
	Code      edrv1.ErrorCode
	Message   string
	RequestID string
	Details   map[string]string
	Err       error // underlying error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.Err
}

// ToGRPCStatus converts the error to a gRPC status with details.
func (e *Error) ToGRPCStatus() *status.Status {
	grpcCode := mapErrorCodeToGRPC(e.Code)
	st := status.New(grpcCode, e.Message)

	// Note: When using generated proto code, uncomment this to add error details:
	// details := &edrv1.ErrorDetails{
	// 	Code:      e.Code,
	// 	Message:   e.Message,
	// 	RequestId: e.RequestID,
	// 	Timestamp: timestamppb.Now(),
	// 	Metadata:  e.Details,
	// }
	// st, _ = st.WithDetails(details)

	return st
}

// ToGRPCError converts the error to a gRPC error.
func (e *Error) ToGRPCError() error {
	return e.ToGRPCStatus().Err()
}

// mapErrorCodeToGRPC maps our error codes to gRPC status codes.
func mapErrorCodeToGRPC(code edrv1.ErrorCode) codes.Code {
	switch code {
	// Authentication errors
	case edrv1.ErrorCode_ERROR_CODE_INVALID_TOKEN,
		edrv1.ErrorCode_ERROR_CODE_EXPIRED_TOKEN,
		edrv1.ErrorCode_ERROR_CODE_REVOKED_TOKEN,
		edrv1.ErrorCode_ERROR_CODE_INVALID_CERTIFICATE,
		edrv1.ErrorCode_ERROR_CODE_EXPIRED_CERTIFICATE,
		edrv1.ErrorCode_ERROR_CODE_REVOKED_CERTIFICATE:
		return codes.Unauthenticated

	// Authorization errors
	case edrv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED,
		edrv1.ErrorCode_ERROR_CODE_AGENT_NOT_APPROVED,
		edrv1.ErrorCode_ERROR_CODE_AGENT_SUSPENDED:
		return codes.PermissionDenied

	// Validation errors
	case edrv1.ErrorCode_ERROR_CODE_INVALID_REQUEST,
		edrv1.ErrorCode_ERROR_CODE_INVALID_BATCH,
		edrv1.ErrorCode_ERROR_CODE_PAYLOAD_TOO_LARGE,
		edrv1.ErrorCode_ERROR_CODE_CHECKSUM_MISMATCH:
		return codes.InvalidArgument

	case edrv1.ErrorCode_ERROR_CODE_DUPLICATE_BATCH:
		return codes.AlreadyExists

	// Rate limiting
	case edrv1.ErrorCode_ERROR_CODE_RATE_LIMITED,
		edrv1.ErrorCode_ERROR_CODE_QUOTA_EXCEEDED:
		return codes.ResourceExhausted

	// Server errors
	case edrv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
		edrv1.ErrorCode_ERROR_CODE_DATABASE_ERROR,
		edrv1.ErrorCode_ERROR_CODE_CACHE_ERROR:
		return codes.Internal

	case edrv1.ErrorCode_ERROR_CODE_UNAVAILABLE:
		return codes.Unavailable

	default:
		return codes.Unknown
	}
}

// ============================================================================
// CONSTRUCTOR FUNCTIONS
// ============================================================================

// New creates a new Error with the given code and message.
func New(code edrv1.ErrorCode, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Details: make(map[string]string),
	}
}

// Wrap wraps an existing error with our error type.
func Wrap(code edrv1.ErrorCode, message string, err error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Details: make(map[string]string),
		Err:     err,
	}
}

// WithRequestID adds a request ID to the error.
func (e *Error) WithRequestID(requestID string) *Error {
	e.RequestID = requestID
	return e
}

// WithDetail adds a detail key-value pair to the error.
func (e *Error) WithDetail(key, value string) *Error {
	e.Details[key] = value
	return e
}

// ============================================================================
// AUTHENTICATION ERRORS
// ============================================================================

// InvalidToken returns an error for invalid JWT tokens.
func InvalidToken(message string) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_INVALID_TOKEN, message)
}

// ExpiredToken returns an error for expired JWT tokens.
func ExpiredToken() *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_EXPIRED_TOKEN, "token has expired")
}

// RevokedToken returns an error for revoked JWT tokens.
func RevokedToken() *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_REVOKED_TOKEN, "token has been revoked")
}

// InvalidCertificate returns an error for invalid client certificates.
func InvalidCertificate(message string) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_INVALID_CERTIFICATE, message)
}

// ExpiredCertificate returns an error for expired client certificates.
func ExpiredCertificate() *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_EXPIRED_CERTIFICATE, "certificate has expired")
}

// RevokedCertificate returns an error for revoked client certificates.
func RevokedCertificate() *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_REVOKED_CERTIFICATE, "certificate has been revoked")
}

// ============================================================================
// AUTHORIZATION ERRORS
// ============================================================================

// PermissionDenied returns an error for permission denied.
func PermissionDenied(message string) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, message)
}

// AgentNotApproved returns an error for unapproved agents.
func AgentNotApproved(agentID string) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_AGENT_NOT_APPROVED, "agent not approved").
		WithDetail("agent_id", agentID)
}

// AgentSuspended returns an error for suspended agents.
func AgentSuspended(agentID string) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_AGENT_SUSPENDED, "agent has been suspended").
		WithDetail("agent_id", agentID)
}

// ============================================================================
// VALIDATION ERRORS
// ============================================================================

// InvalidRequest returns an error for invalid requests.
func InvalidRequest(message string) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_INVALID_REQUEST, message)
}

// InvalidBatch returns an error for invalid event batches.
func InvalidBatch(message string) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_INVALID_BATCH, message)
}

// PayloadTooLarge returns an error for oversized payloads.
func PayloadTooLarge(size, maxSize int64) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_PAYLOAD_TOO_LARGE,
		fmt.Sprintf("payload size %d exceeds maximum %d bytes", size, maxSize))
}

// DuplicateBatch returns an error for duplicate batch submissions.
func DuplicateBatch(batchID string) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_DUPLICATE_BATCH, "batch already processed").
		WithDetail("batch_id", batchID)
}

// ChecksumMismatch returns an error for checksum validation failures.
func ChecksumMismatch(expected, actual string) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_CHECKSUM_MISMATCH, "checksum verification failed").
		WithDetail("expected", expected).
		WithDetail("actual", actual)
}

// ============================================================================
// RATE LIMITING ERRORS
// ============================================================================

// RateLimited returns an error for rate limited requests.
func RateLimited(limit int, windowSec int) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_RATE_LIMITED,
		fmt.Sprintf("rate limit exceeded: %d requests per %d seconds", limit, windowSec))
}

// QuotaExceeded returns an error for quota exceeded.
func QuotaExceeded(message string) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_QUOTA_EXCEEDED, message)
}

// ============================================================================
// SERVER ERRORS
// ============================================================================

// Internal returns an internal server error.
func Internal(message string) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR, message)
}

// InternalWrap wraps an internal error.
func InternalWrap(message string, err error) *Error {
	return Wrap(edrv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR, message, err)
}

// DatabaseError returns a database error.
func DatabaseError(err error) *Error {
	return Wrap(edrv1.ErrorCode_ERROR_CODE_DATABASE_ERROR, "database operation failed", err)
}

// CacheError returns a cache error.
func CacheError(err error) *Error {
	return Wrap(edrv1.ErrorCode_ERROR_CODE_CACHE_ERROR, "cache operation failed", err)
}

// Unavailable returns a service unavailable error.
func Unavailable(message string) *Error {
	return New(edrv1.ErrorCode_ERROR_CODE_UNAVAILABLE, message)
}
