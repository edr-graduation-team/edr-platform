//go:build windows

package grpcclient

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

// memoryStatusEx matches the Windows MEMORYSTATUSEX struct.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// getSystemMemoryMB returns the actual system Total and Used RAM in MB
// by calling the Windows GlobalMemoryStatusEx API.
func getSystemMemoryMB() (totalMB, usedMB uint64) {
	var mem memoryStatusEx
	mem.Length = uint32(unsafe.Sizeof(mem))
	ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem)))
	if ret != 0 {
		totalMB = mem.TotalPhys / 1024 / 1024
		usedMB = (mem.TotalPhys - mem.AvailPhys) / 1024 / 1024
	}
	return
}

// getSystemCPUCount returns the number of logical CPUs available on the system.
func getSystemCPUCount() int {
	return runtime.NumCPU()
}

// getDeviceProfile classifies this Windows endpoint as one of:
// "Domain Controller", "Server", "Laptop", or "Workstation".
//
// Detection order:
//  1. DomainRole ≥ 4  → Domain Controller (most specific)
//  2. ProductType = 3 → Server OS
//  3. PCSystemType = 2 → Mobile / Laptop chassis
//  4. Default         → Workstation
func getDeviceProfile() string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	script := `$cs = Get-CimInstance Win32_ComputerSystem -Property PCSystemType,DomainRole; ` +
		`$os = Get-CimInstance Win32_OperatingSystem -Property ProductType; ` +
		`[PSCustomObject]@{DomainRole=$cs.DomainRole;PCSystemType=$cs.PCSystemType;ProductType=$os.ProductType} | ConvertTo-Json`

	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return ""
	}

	var info struct {
		DomainRole   int `json:"DomainRole"`
		PCSystemType int `json:"PCSystemType"`
		ProductType  int `json:"ProductType"`
	}
	if json.Unmarshal(bytes.TrimSpace(out), &info) != nil {
		return ""
	}

	if info.DomainRole >= 4 {
		return "Domain Controller"
	}
	if info.ProductType == 3 {
		return "Server"
	}
	if info.PCSystemType == 2 {
		return "Laptop"
	}
	return "Workstation"
}

// getLoggedInUser returns the currently logged-in interactive user via
// Win32_ComputerSystem.UserName.  Falls back to the LogonUI registry key
// (LastLoggedOnUser) when no interactive session is active (e.g. on servers).
func getLoggedInUser() string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		`(Get-CimInstance Win32_ComputerSystem).UserName`).Output()
	if err == nil {
		if u := strings.TrimSpace(string(out)); u != "" {
			return u
		}
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	out2, err2 := exec.CommandContext(ctx2, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		`(Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Authentication\LogonUI' `+
			`-Name 'LastLoggedOnUser' -ErrorAction SilentlyContinue).LastLoggedOnUser`).Output()
	if err2 == nil {
		if u := strings.TrimSpace(string(out2)); u != "" {
			return u
		}
	}
	return ""
}
