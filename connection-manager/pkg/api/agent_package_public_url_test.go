package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestResolveAgentPackageDownloadBase_loopbackToServerIP(t *testing.T) {
	t.Setenv("PUBLIC_API_BASE_URL", "")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "http://localhost:30088/api/v1/agent/packages", nil)
	req.Host = "localhost:30088"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	got := resolveAgentPackageDownloadBase(c, CreateAgentPackageRequest{
		ServerIP:     "192.168.152.1",
		ServerDomain: "edr.local",
	})
	want := "http://192.168.152.1:30088"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveAgentPackageDownloadBase_loopbackPrefersIPOverDomain(t *testing.T) {
	t.Setenv("PUBLIC_API_BASE_URL", "")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/agent/packages", nil)
	req.Host = "localhost"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	got := resolveAgentPackageDownloadBase(c, CreateAgentPackageRequest{
		ServerIP:     "10.0.0.5",
		ServerDomain: "edr.example",
	})
	want := "http://10.0.0.5"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveAgentPackageDownloadBase_publicEnvOverrides(t *testing.T) {
	t.Setenv("PUBLIC_API_BASE_URL", "https://edr.example.com")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8082/x", nil)
	req.Host = "localhost:8082"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	got := resolveAgentPackageDownloadBase(c, CreateAgentPackageRequest{
		ServerIP: "192.168.1.1",
	})
	want := "https://edr.example.com"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveAgentPackageDownloadBase_bodyOverride(t *testing.T) {
	t.Setenv("PUBLIC_API_BASE_URL", "")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8082/x", nil)
	req.Host = "localhost:8082"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	got := resolveAgentPackageDownloadBase(c, CreateAgentPackageRequest{
		PublicAPIBaseURL: "http://192.168.55.2:30088",
		ServerIP:         "10.0.0.1",
	})
	want := "http://192.168.55.2:30088"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveAgentPackageDownloadBase_forwardedHeaders(t *testing.T) {
	t.Setenv("PUBLIC_API_BASE_URL", "")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "http://10.0.0.1:8082/x", nil)
	req.Host = "10.0.0.1:8082"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "dashboard.corp.example, internal.local")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	got := resolveAgentPackageDownloadBase(c, CreateAgentPackageRequest{})
	want := "https://dashboard.corp.example"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveAgentPackageDownloadBase_nonLoopbackUnchanged(t *testing.T) {
	t.Setenv("PUBLIC_API_BASE_URL", "")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "http://192.168.1.10:8082/x", nil)
	req.Host = "192.168.1.10:8082"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	got := resolveAgentPackageDownloadBase(c, CreateAgentPackageRequest{
		ServerIP: "10.0.0.1",
	})
	want := "http://192.168.1.10:8082"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
