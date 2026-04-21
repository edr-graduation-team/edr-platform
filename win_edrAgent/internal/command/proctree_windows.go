//go:build windows
// +build windows

package command

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processTreePostOrder returns PIDs in an order safe for termination: children before ancestors, root last.
func processTreePostOrder(rootPID uint32) ([]uint32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("CreateToolhelp32Snapshot: %w", err)
	}
	defer windows.CloseHandle(snap)

	children := make(map[uint32][]uint32)
	seen := make(map[uint32]struct{})

	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	if err := windows.Process32First(snap, &e); err != nil {
		return nil, fmt.Errorf("Process32First: %w", err)
	}
	for {
		seen[e.ProcessID] = struct{}{}
		pp := e.ParentProcessID
		children[pp] = append(children[pp], e.ProcessID)
		if windows.Process32Next(snap, &e) != nil {
			break
		}
	}

	if _, ok := seen[rootPID]; !ok {
		return []uint32{rootPID}, nil
	}

	var out []uint32
	visit := make(map[uint32]bool)
	var walk func(uint32)
	walk = func(p uint32) {
		if visit[p] {
			return
		}
		visit[p] = true
		for _, c := range children[p] {
			walk(c)
		}
		out = append(out, p)
	}
	walk(rootPID)
	return out, nil
}
