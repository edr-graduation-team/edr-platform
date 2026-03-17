package main

import (
	"fmt"
	"unsafe"
	"golang.org/x/sys/windows"
)

func getProcessPrivileges(pid uint32) (string, string, bool, string) {
	var userSid, userName, integrity string
	var isElevated bool

	handle, err := windows.OpenProcess(0x1000, false, pid)
	if err != nil {
		return "", "", false, ""
	}
	defer windows.CloseHandle(handle)

	var token windows.Token
	err = windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &token)
	if err != nil {
		return "", "", false, ""
	}
	defer token.Close()

	if user, err := token.GetTokenUser(); err == nil {
		userSid = user.User.Sid.String()
		if account, domain, _, err := user.User.Sid.LookupAccount(""); err == nil {
			userName = domain + "\\" + account
		}
	}

	isElevated = token.IsElevated()

	// Integrity Level
	var infoSize uint32
	windows.GetTokenInformation(token, windows.TokenIntegrityLevel, nil, 0, &infoSize)
	if infoSize > 0 {
		infoBuffer := make([]byte, infoSize)
		err := windows.GetTokenInformation(token, windows.TokenIntegrityLevel, &infoBuffer[0], infoSize, &infoSize)
		if err == nil {
			// TOKEN_MANDATORY_LABEL
			tml := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&infoBuffer[0]))
			sidStr := tml.Label.Sid.String()
			// S-1-16-4096 (Low), S-1-16-8192 (Medium), S-1-16-12288 (High), S-1-16-16384 (System)
			switch sidStr {
			case "S-1-16-4096":
				integrity = "Low"
			case "S-1-16-8192":
				integrity = "Medium"
			case "S-1-16-12288":
				integrity = "High"
			case "S-1-16-16384":
				integrity = "System"
			default:
				integrity = sidStr
			}
		}
	}

	return userSid, userName, isElevated, integrity
}

func main() {
	pid := uint32(windows.GetCurrentProcessId())
	s, n, e, i := getProcessPrivileges(pid)
	fmt.Printf("SID: %s\nName: %s\nElevated: %v\nIntegrity: %s\n", s, n, e, i)
}
