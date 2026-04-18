//go:build !windows
// +build !windows

package responder

func volumeSerialForPath(filePath string) string { return "" }
