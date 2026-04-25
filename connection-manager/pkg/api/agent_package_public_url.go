package api

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

// resolveAgentPackageDownloadBase returns the base URL (no trailing slash) used to
// mint agent package download links. Remote agents must be able to reach this host
// from their network — never rely on loopback when agents run on other machines.
//
// Resolution order:
//  1. PUBLIC_API_BASE_URL — explicit ops setting (prod / split-horizon).
//  2. public_api_base_url in the JSON body — e.g. browser window.location.origin.
//  3. X-Forwarded-Proto + X-Forwarded-Host — when behind a trusted reverse proxy.
//  4. Request scheme + Host, with loopback replaced by server_ip / server_domain
//     from the same create request when possible.
func resolveAgentPackageDownloadBase(c echo.Context, req CreateAgentPackageRequest) string {
	if v := strings.TrimSpace(os.Getenv("PUBLIC_API_BASE_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(req.PublicAPIBaseURL); v != "" {
		return strings.TrimRight(v, "/")
	}
	if v := forwardedProtoHostBase(c); v != "" {
		return strings.TrimRight(v, "/")
	}

	base := strings.TrimRight(c.Scheme()+"://"+c.Request().Host, "/")
	u, err := url.Parse(base)
	if err != nil || u == nil || u.Host == "" {
		return base
	}

	host := strings.Trim(strings.ToLower(u.Hostname()), "[]")
	if !isLoopbackHost(host) {
		return base
	}

	ip := strings.TrimSpace(req.ServerIP)
	if ip != "" {
		port := u.Port()
		if port != "" {
			return fmt.Sprintf("%s://%s", u.Scheme, net.JoinHostPort(ip, port))
		}
		return fmt.Sprintf("%s://%s", u.Scheme, ip)
	}

	dom := strings.TrimSpace(req.ServerDomain)
	if dom != "" {
		port := u.Port()
		if port != "" {
			return fmt.Sprintf("%s://%s:%s", u.Scheme, dom, port)
		}
		return fmt.Sprintf("%s://%s", u.Scheme, dom)
	}

	return base
}

func forwardedProtoHostBase(c echo.Context) string {
	proto := strings.TrimSpace(c.Request().Header.Get("X-Forwarded-Proto"))
	rawHost := strings.TrimSpace(c.Request().Header.Get("X-Forwarded-Host"))
	if proto == "" || rawHost == "" {
		return ""
	}
	// X-Forwarded-Host may list multiple hosts; use the left-most (client-facing).
	host := strings.TrimSpace(strings.Split(rawHost, ",")[0])
	if host == "" {
		return ""
	}
	return proto + "://" + host
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
