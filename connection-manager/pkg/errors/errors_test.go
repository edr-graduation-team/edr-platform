// Package errors provides unit tests for error handling.
package errors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"

	edrv1 "github.com/edr-platform/connection-manager/proto/v1"
)

func TestError_Error(t *testing.T) {
	t.Run("error without underlying error", func(t *testing.T) {
		err := New(edrv1.ErrorCode_ERROR_CODE_INVALID_TOKEN, "invalid token")
		assert.Equal(t, "invalid token", err.Error())
	})

	t.Run("error with underlying error", func(t *testing.T) {
		underlying := New(edrv1.ErrorCode_ERROR_CODE_DATABASE_ERROR, "db failed")
		err := Wrap(edrv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "operation failed", underlying)
		assert.Contains(t, err.Error(), "operation failed")
		assert.Contains(t, err.Error(), "db failed")
	})
}

func TestError_Unwrap(t *testing.T) {
	underlying := New(edrv1.ErrorCode_ERROR_CODE_DATABASE_ERROR, "db failed")
	err := Wrap(edrv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "wrapped", underlying)
	assert.Equal(t, underlying, err.Unwrap())
}

func TestError_ToGRPCStatus(t *testing.T) {
	tests := []struct {
		name         string
		code         edrv1.ErrorCode
		expectedGRPC codes.Code
	}{
		{"invalid token", edrv1.ErrorCode_ERROR_CODE_INVALID_TOKEN, codes.Unauthenticated},
		{"permission denied", edrv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, codes.PermissionDenied},
		{"invalid request", edrv1.ErrorCode_ERROR_CODE_INVALID_REQUEST, codes.InvalidArgument},
		{"duplicate batch", edrv1.ErrorCode_ERROR_CODE_DUPLICATE_BATCH, codes.AlreadyExists},
		{"rate limited", edrv1.ErrorCode_ERROR_CODE_RATE_LIMITED, codes.ResourceExhausted},
		{"internal error", edrv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR, codes.Internal},
		{"unavailable", edrv1.ErrorCode_ERROR_CODE_UNAVAILABLE, codes.Unavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.code, "test message")
			st := err.ToGRPCStatus()
			assert.Equal(t, tt.expectedGRPC, st.Code())
		})
	}
}

func TestError_WithDetail(t *testing.T) {
	err := New(edrv1.ErrorCode_ERROR_CODE_INVALID_REQUEST, "bad request").
		WithDetail("field", "username").
		WithDetail("reason", "too short")

	assert.Equal(t, "username", err.Details["field"])
	assert.Equal(t, "too short", err.Details["reason"])
}

func TestError_WithRequestID(t *testing.T) {
	err := New(edrv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "error").
		WithRequestID("req-123")

	assert.Equal(t, "req-123", err.RequestID)
}

func TestErrorConstructors(t *testing.T) {
	t.Run("InvalidToken", func(t *testing.T) {
		err := InvalidToken("bad token")
		assert.Equal(t, edrv1.ErrorCode_ERROR_CODE_INVALID_TOKEN, err.Code)
	})

	t.Run("ExpiredToken", func(t *testing.T) {
		err := ExpiredToken()
		assert.Equal(t, edrv1.ErrorCode_ERROR_CODE_EXPIRED_TOKEN, err.Code)
	})

	t.Run("RateLimited", func(t *testing.T) {
		err := RateLimited(100, 60)
		assert.Equal(t, edrv1.ErrorCode_ERROR_CODE_RATE_LIMITED, err.Code)
		assert.Contains(t, err.Message, "100")
	})

	t.Run("PayloadTooLarge", func(t *testing.T) {
		err := PayloadTooLarge(1024*1024*20, 1024*1024*10)
		assert.Equal(t, edrv1.ErrorCode_ERROR_CODE_PAYLOAD_TOO_LARGE, err.Code)
	})

	t.Run("DuplicateBatch", func(t *testing.T) {
		err := DuplicateBatch("batch-123")
		assert.Equal(t, edrv1.ErrorCode_ERROR_CODE_DUPLICATE_BATCH, err.Code)
		assert.Equal(t, "batch-123", err.Details["batch_id"])
	})
}
