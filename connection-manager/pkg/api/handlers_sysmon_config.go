package api

import (
	_ "embed"
	"net/http"

	"github.com/labstack/echo/v4"
)

//go:embed ../../assets/sysmonconfig.xml
var embeddedSysmonConfigXML []byte

// ServeSysmonConfig serves a default Sysmon config XML for agents.
// Public endpoint (no auth) so agents can fetch it during bootstrap.
// GET /api/v1/agent/sysmon/config
func (h *Handlers) ServeSysmonConfig(c echo.Context) error {
	if len(embeddedSysmonConfigXML) == 0 {
		return errorResponse(c, http.StatusServiceUnavailable, "CONFIG_UNAVAILABLE", "Sysmon config is not available")
	}
	return c.Blob(http.StatusOK, "application/xml", embeddedSysmonConfigXML)
}

