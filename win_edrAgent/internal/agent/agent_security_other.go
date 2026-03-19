// agent_security_other.go — No-op security bootstrap for non-Windows platforms.
//
//go:build !windows
// +build !windows

package agent

// initSecurity is a no-op on non-Windows platforms.
// All security modules (ACL, DPAPI encryption, self-protection) are Windows-only.
func (a *Agent) initSecurity() {}
