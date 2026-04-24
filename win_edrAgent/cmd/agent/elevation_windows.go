//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modShell32         = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW  = modShell32.NewProc("ShellExecuteW")
)

func isProcessElevated() (bool, error) {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &tok); err != nil {
		return false, err
	}
	defer tok.Close()

	var elev uint32
	var outLen uint32
	err := windows.GetTokenInformation(tok, windows.TokenElevation, (*byte)(unsafe.Pointer(&elev)), uint32(unsafe.Sizeof(elev)), &outLen)
	if err != nil {
		return false, err
	}
	return elev != 0, nil
}

func tryRelaunchElevated() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	// Preserve original args so the elevated process runs the same command.
	args := strings.Join(os.Args[1:], " ")

	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(args)
	dir, _ := windows.UTF16PtrFromString("")
	show := uintptr(1) // SW_SHOWNORMAL

	r, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		uintptr(unsafe.Pointer(dir)),
		show,
	)
	// Per ShellExecute docs: return value > 32 indicates success.
	return r > 32
}

func requireElevationForUpdate() {
	ok, err := isProcessElevated()
	if err == nil && ok {
		return
	}

	// Best UX: try to relaunch with a UAC prompt automatically.
	if tryRelaunchElevated() {
		fmt.Fprintln(os.Stderr, "\n[i] Update requires elevation — UAC prompt shown. Continue in the elevated window.")
		os.Exit(0)
	}

	// Fall back to explicit instruction.
	fmt.Fprintln(os.Stderr, "\n[X] Error: -update must be run from an elevated (Run as Administrator) console.")
	fmt.Fprintln(os.Stderr, "    The agent data directory is protected (SYSTEM + elevated Administrators only): C:\\ProgramData\\EDR")
	fmt.Fprintln(os.Stderr, "    Fix: open PowerShell as Administrator, then run: .\\edr-agent.exe -update")
	_ = syscall.EPERM
	os.Exit(1)
}

