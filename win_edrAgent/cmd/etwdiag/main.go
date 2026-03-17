//go:build windows
// +build windows

package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	bizoneetw "github.com/bi-zone/etw"
	"golang.org/x/sys/windows"
)

func main() {
	// Microsoft-Windows-Kernel-Process GUID
	kernelProcessGUID := windows.GUID{
		Data1: 0x22FB2CD6,
		Data2: 0x0FE7,
		Data3: 0x4212,
		Data4: [8]byte{0xA2, 0x96, 0x1F, 0x7F, 0x7D, 0x3B, 0x40, 0x0C},
	}

	fmt.Println("=== ETW Diagnostic Tool ===")
	fmt.Printf("Provider GUID: %v\n", kernelProcessGUID)

	// Kill any orphaned sessions
	_ = bizoneetw.KillSession("ETWDiagnostic")
	time.Sleep(500 * time.Millisecond)

	// Test 1: Try with MatchAnyKeyword = 0 (should mean "all events")
	fmt.Println("\n--- Test 1: MatchAnyKeyword=0 (default, no keyword filter) ---")
	testSession(kernelProcessGUID, "ETWDiag1", 0)

	// Test 2: Try with MatchAnyKeyword = 0x10 (WINEVENT_KEYWORD_PROCESS)
	fmt.Println("\n--- Test 2: MatchAnyKeyword=0x10 (PROCESS keyword) ---")
	testSession(kernelProcessGUID, "ETWDiag2", 0x10)

	// Test 3: Try with MatchAnyKeyword = 0xFFFFFFFFFFFFFFFF (ALL keywords)
	fmt.Println("\n--- Test 3: MatchAnyKeyword=0xFFFFFFFFFFFFFFFF (ALL keywords) ---")
	testSession(kernelProcessGUID, "ETWDiag3", 0xFFFFFFFFFFFFFFFF)

	// Test 4: Try Microsoft-Windows-Kernel-Process with GUID parsed from string
	parsedGUID, err := windows.GUIDFromString("{22FB2CD6-0FE7-4212-A296-1F7F7D3B400C}")
	if err != nil {
		fmt.Printf("Failed to parse GUID: %v\n", err)
	} else {
		fmt.Println("\n--- Test 4: Parsed GUID from string, MatchAnyKeyword=0x10 ---")
		fmt.Printf("Parsed GUID: %v\n", parsedGUID)
		fmt.Printf("GUIDs equal: %v\n", parsedGUID == kernelProcessGUID)
		testSession(parsedGUID, "ETWDiag4", 0x10)
	}

	// Test 5: Try a known-noisy provider: Microsoft-Windows-Kernel-File
	fileGUID, _ := windows.GUIDFromString("{EDD08927-9CC4-4E65-B970-C2560FB5C289}")
	fmt.Println("\n--- Test 5: Microsoft-Windows-Kernel-File (known noisy), MatchAnyKeyword=0x10 ---")
	testSession(fileGUID, "ETWDiag5", 0x10)

	// Test 6: Long-running session to wait for real process creation
	fmt.Println("\n--- Test 6: Long-running session (30s) with Kernel-Process, AnyKW=0x10 ---")
	fmt.Println("Please run 'whoami' or 'notepad' during this test...")
	testSessionLong(kernelProcessGUID, "ETWDiag6", 0x10, 30*time.Second)
}

func testSession(guid windows.GUID, name string, anyKeyword uint64) {
	_ = bizoneetw.KillSession(name)
	time.Sleep(200 * time.Millisecond)

	session, err := bizoneetw.NewSession(guid,
		bizoneetw.WithName(name),
		bizoneetw.WithLevel(bizoneetw.TRACE_LEVEL_VERBOSE),
		bizoneetw.WithMatchKeywords(anyKeyword, 0),
	)
	if err != nil {
		fmt.Printf("  FAIL: NewSession error: %v\n", err)
		return
	}
	fmt.Printf("  Session '%s' created successfully\n", name)

	var count atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := session.Process(func(e *bizoneetw.Event) {
			n := count.Add(1)
			if n <= 5 {
				fmt.Printf("  EVENT: ID=%d PID=%d Provider=%v Keyword=0x%x\n",
					e.Header.ID, e.Header.ProcessID, e.Header.ProviderID, e.Header.Keyword)
			}
		})
		if err != nil {
			fmt.Printf("  Process error: %v\n", err)
		}
	}()

	// Wait 5 seconds
	time.Sleep(5 * time.Second)
	n := count.Load()
	fmt.Printf("  Result: %d events in 5 seconds\n", n)

	session.Close()
	wg.Wait()
	fmt.Printf("  Session closed cleanly. Total events: %d\n", count.Load())
}

func testSessionLong(guid windows.GUID, name string, anyKeyword uint64, duration time.Duration) {
	_ = bizoneetw.KillSession(name)
	time.Sleep(200 * time.Millisecond)

	session, err := bizoneetw.NewSession(guid,
		bizoneetw.WithName(name),
		bizoneetw.WithLevel(bizoneetw.TRACE_LEVEL_VERBOSE),
		bizoneetw.WithMatchKeywords(anyKeyword, 0),
	)
	if err != nil {
		fmt.Printf("  FAIL: NewSession error: %v\n", err)
		return
	}
	fmt.Printf("  Session '%s' created, listening for %v...\n", name, duration)

	var count atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err := session.Process(func(e *bizoneetw.Event) {
			n := count.Add(1)
			if n <= 20 {
				props, _ := e.EventProperties()
				imgName := ""
				pid := ""
				if props != nil {
					if v, ok := props["ImageName"]; ok {
						imgName = fmt.Sprintf("%v", v)
					}
					if v, ok := props["ProcessID"]; ok {
						pid = fmt.Sprintf("%v", v)
					}
				}
				fmt.Printf("  [%d] EventID=%d HeaderPID=%d TDH_PID=%s Image=%s\n",
					n, e.Header.ID, e.Header.ProcessID, pid, imgName)
			}
		})
		if err != nil {
			fmt.Printf("  Process error: %v\n", err)
		}
	}()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	select {
	case <-time.After(duration):
	case <-sigCh:
		fmt.Println("  Interrupted!")
	}

	n := count.Load()
	fmt.Printf("  Closing. Total events: %d\n", n)
	session.Close()
	wg.Wait()
}
