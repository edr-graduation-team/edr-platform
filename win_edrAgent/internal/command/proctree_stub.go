//go:build !windows
// +build !windows

package command

import "fmt"

func processTreePostOrder(rootPID uint32) ([]uint32, error) {
	return nil, fmt.Errorf("process tree termination is only supported on Windows")
}
