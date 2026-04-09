// Package api provides the agent binary build endpoint.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// BuildAgentRequest is the JSON body for agent build requests from the dashboard.
type BuildAgentRequest struct {
	ServerIP     string `json:"server_ip"`
	ServerDomain string `json:"server_domain"`
	ServerPort   string `json:"server_port"`
	TokenID      string `json:"token_id" validate:"required"`
	SkipConfig   bool   `json:"skip_config"` // if true, only token + CA are embedded
}

// builderRequest is the JSON body sent to the agent-builder service.
type builderRequest struct {
	ServerIP     string `json:"server_ip"`
	ServerDomain string `json:"server_domain"`
	ServerPort   string `json:"server_port"`
	Token        string `json:"token"`
	SkipConfig   bool   `json:"skip_config"`
	CACertPEM    string `json:"ca_cert_pem"`
}

// BuildAgent handles POST /api/v1/agent/build
//
// Workflow:
//  1. Validate request + fetch token from DB.
//  2. Read the CA certificate from disk.
//  3. Send build request to the dedicated agent-builder service.
//  4. Stream the resulting binary back to the dashboard as a download.
func (h *Handlers) BuildAgent(c echo.Context) error {
	var req BuildAgentRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}

	// ── Validate token_id ──────────────────────────────────────────────────
	if req.TokenID == "" {
		return errorResponse(c, http.StatusBadRequest, "TOKEN_REQUIRED",
			"Token is required for all agent builds (security policy)")
	}

	if h.enrollmentTokenRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE",
			"Database unavailable — cannot validate token")
	}

	// Fetch all tokens and find the requested one that is valid
	tokens, err := h.enrollmentTokenRepo.List(c.Request().Context())
	if err != nil {
		h.logger.Errorf("BuildAgent: failed to list tokens: %v", err)
		return errorResponse(c, http.StatusInternalServerError, "TOKEN_FETCH_ERROR",
			"Failed to fetch enrollment tokens")
	}

	var tokenValue string
	var tokenDesc string
	for _, t := range tokens {
		if t.ID.String() == req.TokenID {
			// Validate the token is usable
			if !t.IsActive {
				return errorResponse(c, http.StatusBadRequest, "TOKEN_REVOKED",
					"The selected token has been revoked")
			}
			if t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now()) {
				return errorResponse(c, http.StatusBadRequest, "TOKEN_EXPIRED",
					"The selected token has expired")
			}
			if t.MaxUses != nil && t.UseCount >= *t.MaxUses {
				return errorResponse(c, http.StatusBadRequest, "TOKEN_MAXED",
					"The selected token has reached its maximum number of uses")
			}
			tokenValue = t.Token
			tokenDesc = t.Description
			break
		}
	}
	if tokenValue == "" {
		return errorResponse(c, http.StatusNotFound, "TOKEN_NOT_FOUND",
			"Token not found or does not meet validity requirements")
	}

	// ── Validate server config if not skipping ─────────────────────────────
	if !req.SkipConfig {
		if req.ServerIP == "" || req.ServerDomain == "" {
			return errorResponse(c, http.StatusBadRequest, "MISSING_CONFIG",
				"server_ip and server_domain are required when not skipping config")
		}
	}
	if req.ServerPort == "" {
		req.ServerPort = "50051"
	}

	// ── Read CA certificate PEM ────────────────────────────────────────────
	var caCertPEM string
	if h.caCertPath != "" {
		data, err := os.ReadFile(h.caCertPath)
		if err != nil {
			h.logger.Errorf("BuildAgent: failed to read CA cert at %s: %v", h.caCertPath, err)
			return errorResponse(c, http.StatusInternalServerError, "CA_READ_ERROR",
				"Failed to read CA certificate from server")
		}
		caCertPEM = string(data)
	}

	// ── Resolve builder URL ────────────────────────────────────────────────
	builderURL := os.Getenv("AGENT_BUILDER_URL")
	if builderURL == "" {
		builderURL = "http://agent-builder:8090"
	}

	// ── Send build request to agent-builder service ────────────────────────
	h.logger.Infof("BuildAgent: sending build to %s (skip_config=%v, token=%s)",
		builderURL, req.SkipConfig, tokenDesc)

	buildReq := builderRequest{
		ServerIP:     req.ServerIP,
		ServerDomain: req.ServerDomain,
		ServerPort:   req.ServerPort,
		Token:        tokenValue,
		SkipConfig:   req.SkipConfig,
		CACertPEM:    caCertPEM,
	}

	body, err := json.Marshal(buildReq)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "MARSHAL_ERROR",
			"Failed to marshal build request")
	}

	client := &http.Client{Timeout: 10 * time.Minute} // generous timeout for cross-compilation (first build can take 3-5 min)
	resp, err := client.Post(builderURL+"/build", "application/json", bytes.NewReader(body))
	if err != nil {
		h.logger.Errorf("BuildAgent: builder request failed: %v", err)
		return errorResponse(c, http.StatusBadGateway, "BUILDER_UNAVAILABLE",
			"Agent builder service is not reachable. Ensure the agent-builder container is running.")
	}
	defer resp.Body.Close()

	// ── Handle builder error ───────────────────────────────────────────────
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			h.logger.Errorf("BuildAgent: builder returned %d: %v", resp.StatusCode, errResp)
			return c.JSON(resp.StatusCode, errResp)
		}
		return errorResponse(c, http.StatusInternalServerError, "BUILD_FAILED",
			"Agent build failed — check builder logs")
	}

	// ── Read full binary from builder ─────────────────────────────────────
	binaryData, err := io.ReadAll(resp.Body)
	if err != nil {
		h.logger.Errorf("BuildAgent: failed to read builder response: %v", err)
		return errorResponse(c, http.StatusInternalServerError, "READ_ERROR",
			"Failed to read built binary from builder")
	}

	sha256Hash := resp.Header.Get("X-Agent-SHA256")
	buildDuration := resp.Header.Get("X-Build-Duration")

	h.logger.Infof("BuildAgent: build succeeded in %s, size=%d bytes (sha256=%s)",
		buildDuration, len(binaryData), sha256Hash[:16]+"...")

	// Audit log
	h.fireAudit(c, "agent.build", "agent_binary", uuid.Nil, fmt.Sprintf(
		"Agent built: skip_config=%v, token=%s, sha256=%s, duration=%s, size=%d",
		req.SkipConfig, tokenDesc, sha256Hash[:16]+"...", buildDuration, len(binaryData)), false, "")

	// Set download headers
	c.Response().Header().Set("Content-Disposition", `attachment; filename="edr-agent.exe"`)
	c.Response().Header().Set("X-Agent-SHA256", sha256Hash)
	c.Response().Header().Set("X-Agent-Token-Description", tokenDesc)
	c.Response().Header().Set("X-Build-Duration", buildDuration)
	if !req.SkipConfig {
		c.Response().Header().Set("X-Agent-Server",
			fmt.Sprintf("%s:%s", req.ServerDomain, req.ServerPort))
	}
	c.Response().Header().Set("X-Agent-CA-Embedded", fmt.Sprintf("%v", caCertPEM != ""))

	return c.Blob(http.StatusOK, "application/octet-stream", binaryData)
}
