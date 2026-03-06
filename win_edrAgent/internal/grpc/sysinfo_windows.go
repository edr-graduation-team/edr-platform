//go:build windows

package grpcclient

import (
	"runtime"
	"syscall"
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
