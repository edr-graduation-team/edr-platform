package api

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/edr-platform/connection-manager/internal/repository"
)

type CreateAgentPackageRequest struct {
	ServerIP      string `json:"server_ip"`
	ServerDomain  string `json:"server_domain"`
	ServerPort    string `json:"server_port"`
	TokenID       string `json:"token_id" validate:"required"`
	SkipConfig    bool   `json:"skip_config"`
	InstallSysmon bool   `json:"install_sysmon"`

	ExpiresInSeconds int `json:"expires_in_seconds"` // default 900
}

type CreateAgentPackageResponse struct {
	PackageID string `json:"package_id"`
	SHA256    string `json:"sha256"`
	Filename  string `json:"filename"`
	ExpiresAt string `json:"expires_at"`
	URL       string `json:"url"`
}

func (h *Handlers) CreateAgentPackage(c echo.Context) error {
	if h.agentPackageRepo == nil || h.enrollmentTokenRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Database unavailable")
	}
	var req CreateAgentPackageRequest
	if err := c.Bind(&req); err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
	}
	if req.TokenID == "" {
		return errorResponse(c, http.StatusBadRequest, "TOKEN_REQUIRED", "token_id is required")
	}
	if !req.SkipConfig && (req.ServerIP == "" || req.ServerDomain == "") {
		return errorResponse(c, http.StatusBadRequest, "MISSING_CONFIG", "server_ip and server_domain are required when not skipping config")
	}
	if req.ServerPort == "" {
		req.ServerPort = "50051"
	}
	exp := req.ExpiresInSeconds
	if exp <= 0 {
		exp = 900
	}
	if exp > 86400 {
		exp = 86400
	}
	expiresAt := time.Now().Add(time.Duration(exp) * time.Second)

	// Resolve token value
	tokens, err := h.enrollmentTokenRepo.List(c.Request().Context())
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "TOKEN_FETCH_ERROR", "Failed to fetch enrollment tokens")
	}
	var tokenValue string
	for _, t := range tokens {
		if t.ID.String() == req.TokenID {
			tokenValue = t.Token
			break
		}
	}
	if tokenValue == "" {
		return errorResponse(c, http.StatusNotFound, "TOKEN_NOT_FOUND", "Token not found")
	}

	// Read CA PEM
	var caCertPEM string
	if h.caCertPath != "" {
		if data, err := os.ReadFile(h.caCertPath); err == nil {
			caCertPEM = string(data)
		}
	}

	// Call builder
	builderURL := os.Getenv("AGENT_BUILDER_URL")
	if builderURL == "" {
		builderURL = "http://agent-builder:8090"
	}
	buildReq := builderRequest{
		ServerIP:      req.ServerIP,
		ServerDomain:  req.ServerDomain,
		ServerPort:    req.ServerPort,
		Token:         tokenValue,
		SkipConfig:    req.SkipConfig,
		CACertPEM:     caCertPEM,
		InstallSysmon: req.InstallSysmon,
	}
	body, _ := json.Marshal(buildReq)
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Post(builderURL+"/build", "application/json", bytes.NewReader(body))
	if err != nil {
		return errorResponse(c, http.StatusBadGateway, "BUILDER_UNAVAILABLE", "Agent builder service is not reachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return errorResponse(c, http.StatusInternalServerError, "BUILD_FAILED", fmt.Sprintf("Builder error: %s", strings.TrimSpace(string(raw))))
	}
	bin, err := io.ReadAll(resp.Body)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "READ_ERROR", "Failed to read built binary")
	}
	sum := sha256.Sum256(bin)
	shaStr := hex.EncodeToString(sum[:])

	// Persist to disk
	packageID := uuid.New()
	dataDir := os.Getenv("AGENT_PACKAGES_DIR")
	if dataDir == "" {
		dataDir = `./data/agent-packages`
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return errorResponse(c, http.StatusInternalServerError, "STORAGE_ERROR", "Failed to create packages directory")
	}
	storagePath := filepath.Join(dataDir, packageID.String()+".exe")
	if err := os.WriteFile(storagePath, bin, 0600); err != nil {
		return errorResponse(c, http.StatusInternalServerError, "STORAGE_ERROR", "Failed to write package to disk")
	}

	// Tokenize download URL (short-lived)
	downloadToken, _ := newRandomToken(32)
	downloadTokenHash := sha256Hex(downloadToken)

	// Save package row (store token hash in build_params for MVP)
	params := map[string]any{
		"server_ip":      req.ServerIP,
		"server_domain":  req.ServerDomain,
		"server_port":    req.ServerPort,
		"token_id":       req.TokenID,
		"skip_config":    req.SkipConfig,
		"install_sysmon": req.InstallSysmon,
		"download_token_hash": downloadTokenHash,
	}
	if err := h.agentPackageRepo.Create(c.Request().Context(), repository.AgentPackageRow{
		ID:          packageID,
		SHA256:      shaStr,
		Filename:    "edr-agent.exe",
		StoragePath: storagePath,
		BuildParams: params,
		ExpiresAt:   expiresAt,
	}); err != nil {
		return errorResponse(c, http.StatusInternalServerError, "DB_ERROR", "Failed to save package metadata")
	}

	baseURL := strings.TrimRight(c.Scheme()+"://"+c.Request().Host, "/")
	url := fmt.Sprintf("%s/api/v1/agent/packages/%s/download?token=%s", baseURL, packageID.String(), downloadToken)

	return c.JSON(http.StatusOK, CreateAgentPackageResponse{
		PackageID: packageID.String(),
		SHA256:    shaStr,
		Filename:  "edr-agent.exe",
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
		URL:       url,
	})
}

func (h *Handlers) DownloadAgentPackage(c echo.Context) error {
	if h.agentPackageRepo == nil {
		return errorResponse(c, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Database unavailable")
	}
	idStr := c.Param("id")
	pkgID, err := uuid.Parse(idStr)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, "INVALID_ID", "Invalid package id")
	}
	token := c.QueryParam("token")
	if token == "" {
		return errorResponse(c, http.StatusUnauthorized, "TOKEN_REQUIRED", "token query parameter required")
	}

	row, err := h.agentPackageRepo.Get(c.Request().Context(), pkgID)
	if err != nil {
		return errorResponse(c, http.StatusNotFound, "NOT_FOUND", "Package not found")
	}
	if time.Now().After(row.ExpiresAt) {
		return errorResponse(c, http.StatusGone, "EXPIRED", "Package link expired")
	}
	wantHash := ""
	if row.BuildParams != nil {
		if v, ok := row.BuildParams["download_token_hash"].(string); ok {
			wantHash = v
		}
	}
	if wantHash == "" || sha256Hex(token) != wantHash {
		return errorResponse(c, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid download token")
	}

	data, err := os.ReadFile(row.StoragePath)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, "READ_ERROR", "Failed to read package")
	}
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", row.Filename))
	c.Response().Header().Set("X-Agent-SHA256", row.SHA256)
	return c.Blob(http.StatusOK, "application/octet-stream", data)
}

func newRandomToken(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

