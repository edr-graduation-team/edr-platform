//go:build windows

package grpcclient

import (
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetSystemPowerStatus = kernel32.NewProc("GetSystemPowerStatus")
)

// systemPowerStatus mirrors the Win32 SYSTEM_POWER_STATUS struct.
type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

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
// Implementation uses only native Win32 registry + API reads (no child
// processes) to avoid generating self-inflicted process-creation telemetry
// that would match Atomic Red Team / Sigma T1059 rules on our own agent.
//
// Detection order:
//  1. Registry ProductType = "LanmanNT" → Domain Controller
//  2. Registry ProductType = "ServerNT" → Server
//  3. GetSystemPowerStatus reports a battery present → Laptop
//  4. Default → Workstation
func getDeviceProfile() string {
	productType := readRegistryString(
		registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\ProductOptions`,
		"ProductType",
	)
	switch productType {
	case "LanmanNT":
		return "Domain Controller"
	case "ServerNT":
		return "Server"
	}

	if hasBattery() {
		return "Laptop"
	}
	return "Workstation"
}

// getLoggedInUser returns the last interactively logged-on user.
//
// Reads HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Authentication\LogonUI
// value LastLoggedOnUser directly via the registry API — no powershell.exe
// spawn — so the agent does not trigger its own T1059 detections.
func getLoggedInUser() string {
	u := readRegistryString(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Authentication\LogonUI`,
		"LastLoggedOnUser",
	)
	return strings.TrimSpace(u)
}

// readRegistryString opens a registry key read-only and returns the string
// value for the given name. Returns "" on any error (missing key/value/type).
func readRegistryString(root registry.Key, path, name string) string {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	return v
}

// hasBattery reports whether the system exposes a battery, which is a strong
// signal for a laptop/portable device. Uses GetSystemPowerStatus; the
// BatteryFlag byte equals 128 ("No system battery") on desktops/servers.
func hasBattery() bool {
	var st systemPowerStatus
	ret, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&st)))
	if ret == 0 {
		return false
	}
	return st.BatteryFlag != 128 && st.BatteryFlag != 255
}
