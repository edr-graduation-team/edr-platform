// Package api — out-of-band approval handlers for manual endpoint commands.
//
// Exposes:
//
//	POST /api/v1/commands/approval        → IssueCommandApproval
//	POST /api/v1/commands/approval/verify → VerifyCommandApproval
//
// And a helper used by ExecuteAgentCommand:
//
//	consumeApprovalIfRequired(c) → enforces the gate, fail-closed.
package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/edr-platform/connection-manager/internal/service"
)

// errGateBlocked is a sentinel returned by consumeApprovalIfRequired after
// it has already written an HTTP error response (via c.JSON). The calling
// handler MUST propagate this error to Echo so no further response body is
// appended. Echo's DefaultHTTPErrorHandler checks c.Response().Committed
// and skips writing when the response is already on the wire, so the
// client sees only the JSON we wrote.
var errGateBlocked = errors.New("approval gate: request blocked (response already written)")

// ApprovalServiceProvider is implemented by Handlers (see middleware.go).
// We keep the name on the receiver here just so the wiring in main.go
// reads naturally: handlers.SetCommandApprovalService(svc).
type ApprovalServiceProvider interface {
	commandApprovalService() service.CommandApprovalService
}

// SetCommandApprovalService wires the approval service. May be called
// after NewHandlers — when nil, the gate is disabled and commands behave
// as they did before this feature was introduced (backwards compatible).
func (h *Handlers) SetCommandApprovalService(svc service.CommandApprovalService) {
	h.commandApprovalSvc = svc
}

// commandApprovalService returns the wired service or nil. Helper used by
// the command-execution handler so it can short-circuit when the feature
// is disabled (i.e. EC2_EMAIL_VERIFY not set, or SMTP unavailable).
func (h *Handlers) commandApprovalService() service.CommandApprovalService {
	if h == nil {
		return nil
	}
	return h.commandApprovalSvc
}

// ────────────────────────────────────────────────────────────────────────────
// Issue
// ────────────────────────────────────────────────────────────────────────────

// IssueCommandApprovalRequest carries the operator's context that ends up in
// the approval email so the second human can review what's being approved.
type IssueCommandApprovalRequest struct {
	Summary     string `json:"summary,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	CommandType string `json:"command_type,omitempty"`
}

// IssueCommandApprovalResponse mirrors what the dashboard needs to drive
// the OTP modal.
type IssueCommandApprovalResponse struct {
	ApprovalID  string `json:"approval_id"`
	MaskedEmail string `json:"masked_email"`
	ExpiresAt   string `json:"expires_at"`
}

// IssueCommandApproval handles POST /api/v1/commands/approval.
func (h *Handlers) IssueCommandApproval(c echo.Context) error {
	svc := h.commandApprovalService()
	if svc == nil || !svc.Available() {
		// Feature disabled / not configured. Tell the dashboard so it
		// can fall back to a no-OTP flow without prompting the operator
		// for a code that will never arrive.
		return errorResponse(c, http.StatusServiceUnavailable, "APPROVAL_DISABLED",
			"Command approval is not configured on this server (EC2_EMAIL_VERIFY / SMTP missing).")
	}

	user := getCurrentUser(c)
	if user == nil {
		return errorResponse(c, http.StatusUnauthorized, "AUTH_REQUIRED", "Authentication required")
	}
	uid, err := uuid.Parse(user.UserID)
	if err != nil {
		return errorResponse(c, http.StatusUnauthorized, "AUTH_REQUIRED", "Invalid user id in token")
	}

	var req IssueCommandApprovalRequest
	_ = c.Bind(&req) // body is fully optional

	ip, ua := auditContext(c)
	challenge, err := svc.IssueChallenge(c.Request().Context(), service.ApprovalRequest{
		UserID:      uid,
		Username:    user.Username,
		IPAddress:   ip,
		UserAgent:   ua,
		Summary:     strings.TrimSpace(req.Summary),
		AgentID:     strings.TrimSpace(req.AgentID),
		CommandType: strings.TrimSpace(req.CommandType),
	})
	if err != nil {
		h.logger.WithError(err).Warn("IssueCommandApproval failed")
		return errorResponse(c, http.StatusServiceUnavailable, "APPROVAL_UNAVAILABLE",
			"Failed to issue approval challenge — see server logs")
	}

	return c.JSON(http.StatusOK, IssueCommandApprovalResponse{
		ApprovalID:  challenge.ID,
		MaskedEmail: challenge.MaskedEmail,
		ExpiresAt:   challenge.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// ────────────────────────────────────────────────────────────────────────────
// Verify
// ────────────────────────────────────────────────────────────────────────────

// VerifyCommandApprovalRequest is the payload from the OTP form.
type VerifyCommandApprovalRequest struct {
	ApprovalID string `json:"approval_id" validate:"required"`
	Code       string `json:"code" validate:"required,len=6"`
}

// VerifyCommandApprovalResponse returns the single-use approval token. The
// dashboard then sends this token in the X-Approval-Token header on the
// retried command request.
type VerifyCommandApprovalResponse struct {
	ApprovalToken string `json:"approval_token"`
	ExpiresAt     string `json:"expires_at"`
}

// VerifyCommandApproval handles POST /api/v1/commands/approval/verify.
func (h *Handlers) VerifyCommandApproval(c echo.Context) error {
	svc := h.commandApprovalService()
	if svc == nil || !svc.Available() {
		return errorResponse(c, http.StatusServiceUnavailable, "APPROVAL_DISABLED",
			"Command approval is not configured on this server.")
	}

	user := getCurrentUser(c)
	if user == nil {
		return errorResponse(c, http.StatusUnauthorized, "AUTH_REQUIRED", "Authentication required")
	}
	uid, err := uuid.Parse(user.UserID)
	if err != nil {
		return errorResponse(c, http.StatusUnauthorized, "AUTH_REQUIRED", "Invalid user id in token")
	}

	var req VerifyCommandApprovalRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}
	req.ApprovalID = strings.TrimSpace(req.ApprovalID)
	req.Code = strings.TrimSpace(req.Code)
	if req.ApprovalID == "" || req.Code == "" {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "approval_id and code are required")
	}

	token, expiresAt, vErr := svc.VerifyChallenge(c.Request().Context(), uid, req.ApprovalID, req.Code)
	if vErr != nil {
		switch vErr {
		case service.ErrApprovalChallengeNotFound:
			return errorResponse(c, http.StatusUnauthorized, "APPROVAL_EXPIRED", "Approval challenge not found or expired")
		case service.ErrApprovalAttemptsExceeded:
			return errorResponse(c, http.StatusUnauthorized, "APPROVAL_LOCKED", "Too many incorrect attempts")
		case service.ErrApprovalCodeInvalid:
			return errorResponse(c, http.StatusUnauthorized, "APPROVAL_INVALID", "Invalid approval code")
		default:
			h.logger.WithError(vErr).Warn("VerifyCommandApproval failed")
			return errorResponse(c, http.StatusServiceUnavailable, "APPROVAL_UNAVAILABLE", "Approval verification failed")
		}
	}

	return c.JSON(http.StatusOK, VerifyCommandApprovalResponse{
		ApprovalToken: token,
		ExpiresAt:     expiresAt.UTC().Format(time.RFC3339),
	})
}

// ────────────────────────────────────────────────────────────────────────────
// Gate helper used by ExecuteAgentCommand
// ────────────────────────────────────────────────────────────────────────────

// consumeApprovalIfRequired enforces the manual-command gate.
//
//   - If the approval service is wired AND available, the request MUST carry
//     a valid X-Approval-Token header (or `approval_token` field on the body
//     — though the header is preferred so the JSON body schema is unchanged).
//     The token is consumed atomically; replays fail.
//
//   - If the approval service is NOT wired or NOT available, the gate is
//     skipped (backwards compatible). This is intentional so deployments
//     without SMTP can still operate exactly as before.
//
// On gate failure the function writes the response itself and returns a
// non-nil error so the caller can early-return.
func (h *Handlers) consumeApprovalIfRequired(c echo.Context, fallbackToken string) error {
	svc := h.commandApprovalService()
	if svc == nil || !svc.Available() {
		return nil // feature off → no gate
	}

	user := getCurrentUser(c)
	if user == nil {
		errorResponse(c, http.StatusUnauthorized, "AUTH_REQUIRED", "Authentication required")
		return errGateBlocked
	}
	uid, err := uuid.Parse(user.UserID)
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "AUTH_REQUIRED", "Invalid user id in token")
		return errGateBlocked
	}

	token := strings.TrimSpace(c.Request().Header.Get("X-Approval-Token"))
	if token == "" {
		token = strings.TrimSpace(fallbackToken)
	}
	if token == "" {
		errorResponse(c, http.StatusForbidden, "APPROVAL_REQUIRED",
			"This command requires an out-of-band approval. Request a code from /commands/approval and resubmit with X-Approval-Token.")
		return errGateBlocked
	}

	if cErr := svc.ConsumeToken(c.Request().Context(), uid, token); cErr != nil {
		h.logger.WithFields(map[string]interface{}{
			"user_id": uid,
			"path":    c.Request().URL.Path,
		}).WithError(cErr).Warn("Approval token rejected")
		errorResponse(c, http.StatusForbidden, "APPROVAL_INVALID",
			"Approval token is missing, expired, or already consumed. Request a new approval.")
		return errGateBlocked
	}

	return nil
}
