// Package api provides zero-touch provisioning endpoints.
package api

import (
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
)

// ServeCA serves the public CA certificate so agents can auto-bootstrap TLS
// trust without manual file distribution. The CA certificate is public data
// (only the public key) — serving it over plain HTTP is standard practice
// (identical to CRL / AIA distribution).
func (h *Handlers) ServeCA(c echo.Context) error {
	if h.caCertPath == "" {
		h.logger.Error("ServeCA: caCertPath not configured")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "CA certificate path not configured on server",
		})
	}

	pemData, err := os.ReadFile(h.caCertPath)
	if err != nil {
		h.logger.Errorf("ServeCA: failed to read CA certificate at %s: %v", h.caCertPath, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to read CA certificate",
		})
	}

	return c.Blob(http.StatusOK, "application/x-pem-file", pemData)
}
