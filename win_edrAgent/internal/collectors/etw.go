// Package collectors — ETW kernel process tracer.
//go:build windows
// +build windows

package collectors

/*
#cgo LDFLAGS: -ltdh -ladvapi32

#include "etw_cgo.h"
*/
import "C"

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/edr-platform/win-agent/internal/event"
	"github.com/edr-platform/win-agent/internal/logging"
)

// Unused GUID kept for session compat parameter.
var kernelProcessGUID = C.GUID{
	Data1: 0x22FB2CD6, Data2: 0x0FE7, Data3: 0x4212,
	Data4: [8]C.uchar{0xA2, 0x96, 0x1F, 0x7F, 0x7D, 0x3B, 0x40, 0x0C},
}

// =====================================================================
// Collector
// =====================================================================

type ETWCollector struct {
	logger    *logging.Logger
	eventChan chan<- *event.Event
	filter    *Filter
	session   string
	running   atomic.Bool
	collected atomic.Uint64
	dropped   atomic.Uint64
	errors    atomic.Uint64

	// Config toggles for event types handled by the same kernel session.
	fileEnabled      bool
	imageLoadEnabled bool

	// Per-type metrics
	fileEvents      atomic.Uint64
	imageLoadEvents atomic.Uint64
}

var globalCollector atomic.Pointer[ETWCollector]

func NewETWCollector(session string, ch chan<- *event.Event, l *logging.Logger, filter *Filter, fileEnabled, imageLoadEnabled bool) *ETWCollector {
	if session == "" {
		session = "EDRKernelTrace"
	}
	return &ETWCollector{
		logger:           l,
		eventChan:        ch,
		filter:           filter,
		session:          session,
		fileEnabled:      fileEnabled,
		imageLoadEnabled: imageLoadEnabled,
	}
}

func (c *ETWCollector) Start(ctx context.Context) error {
	if c.running.Load() {
		return fmt.Errorf("already running")
	}
	c.running.Store(true)
	globalCollector.Store(c)
	go c.run(ctx)
	return nil
}

func (c *ETWCollector) Stop() error {
	c.running.Store(false)
	c.logger.Infof("ETW stats: process=%d imageload=%d fileio=%d dropped=%d errors=%d",
		c.collected.Load(), c.imageLoadEvents.Load(), c.fileEvents.Load(),
		c.dropped.Load(), c.errors.Load())
	return nil
}

func (c *ETWCollector) run(ctx context.Context) {
	c.logger.Info("[BASELINE] Running initial process snapshot...")
	c.baseline()
	c.logger.Info("[BASELINE] Initial snapshot complete")

	c.logger.Infof("[ETW] Starting kernel tracer (Process=ON, ImageLoad=%v, FileIO=%v)",
		c.imageLoadEnabled, c.fileEnabled)
	for ctx.Err() == nil && c.running.Load() {
		if err := c.session_(ctx); err != nil && ctx.Err() == nil && c.running.Load() {
			c.logger.Errorf("[ETW] Session error: %v — restarting in 3s", err)
			time.Sleep(3 * time.Second)
		}
	}
	c.logger.Info("[ETW] Tracer stopped")
}

func (c *ETWCollector) session_(ctx context.Context) error {
	name16, err := windows.UTF16FromString(c.session)
	if err != nil {
		return err
	}
	np := (*C.wchar_t)(unsafe.Pointer(&name16[0]))
	C.KillNamedSession(np)
	time.Sleep(200 * time.Millisecond)

	ret := C.StartKernelProcessSession(np, &kernelProcessGUID, 0xFF, 0x10)
	if ret != 0 {
		c.errors.Add(1)
		return fmt.Errorf("StartKernelProcessSession: error %d", ret)
	}
	c.logger.Info("[ETW] Session ACTIVE — SYSTEM_LOGGER_MODE + EnableFlags=PROCESS|IMAGE_LOAD|FILE_IO_INIT")

	var diag atomic.Uint64
	globalDiag.Store(&diag)
	go func() {
		time.Sleep(5 * time.Second)
		c.logger.Infof("[ETW] Diagnostic: %d events in first 5s", diag.Load())
	}()

	go func() {
		<-ctx.Done()
		C.StopKernelSession(&kernelProcessGUID)
	}()

	ret = C.ProcessKernelEvents(np, nil)
	if ret != 0 && ctx.Err() != nil {
		return nil
	}
	if ret != 0 {
		return fmt.Errorf("ProcessKernelEvents: error %d", ret)
	}
	return nil
}

// =====================================================================
// C → Go callback (no goroutine, already parsed by C/TDH)
// =====================================================================

var globalDiag atomic.Pointer[atomic.Uint64]

var (
	dedupMu    sync.Mutex
	dedupCache = make(map[uint32]int64, 64)
)

func isDuplicate(pid uint32) bool {
	now := time.Now().UnixNano()
	dedupMu.Lock()
	defer dedupMu.Unlock()
	for k, ts := range dedupCache {
		if now-ts > 2_000_000_000 {
			delete(dedupCache, k)
		}
	}
	if t, ok := dedupCache[pid]; ok && now-t < 2_000_000_000 {
		return true
	}
	dedupCache[pid] = now
	return false
}

//export goProcessEvent
func goProcessEvent(evt *C.ParsedProcessEvent) {
	collector := globalCollector.Load()
	if collector == nil {
		return
	}

	pid := uint32(evt.processId)
	ppid := uint32(evt.parentId)
	opcode := uint8(evt.opcode)

	// Convert C strings to Go strings (copies — safe for goroutine)
	imageName := C.GoString(&evt.imageFileName[0])
	cmdLine := wcharToGo(&evt.commandLine[0], 4096)

	// Diagnostic
	if d := globalDiag.Load(); d != nil {
		n := d.Add(1)
		if n <= 20 {
			collector.logger.Infof("[ETW-DBG] #%d Op=%d PID=%d Img=%s Cmd=%s",
				n, opcode, pid, imageName, truncStr(cmdLine, 60))
		}
	}

	if opcode == 1 {
		go collector.processStart(pid, ppid, imageName, cmdLine)
	} else {
		go collector.processEnd(pid, ppid, imageName)
	}
}

// =====================================================================
// C → Go callbacks: Image Load and File I/O events
// =====================================================================

//export goImageLoadEvent
func goImageLoadEvent(evt *C.ParsedImageLoadEvent) {
	collector := globalCollector.Load()
	if collector == nil || !collector.imageLoadEnabled {
		return
	}

	pid := uint32(evt.processId)
	opcode := uint8(evt.opcode)
	imagePath := wcharToGo(&evt.imagePath[0], 1024)

	if imagePath == "" {
		return
	}

	// Only care about loads (opcode 10), not unloads.
	if opcode != 10 {
		return
	}

	go collector.handleImageLoad(pid, imagePath)
}

//export goFileIoEvent
func goFileIoEvent(evt *C.ParsedFileIoEvent) {
	collector := globalCollector.Load()
	if collector == nil || !collector.fileEnabled {
		return
	}

	pid := uint32(evt.processId)
	opcode := uint8(evt.opcode)
	filePath := wcharToGo(&evt.filePath[0], 1024)

	if filePath == "" {
		return
	}

	go collector.handleFileIo(pid, opcode, filePath)
}

func wcharToGo(p *C.WCHAR, max int) string {
	if p == nil {
		return ""
	}
	chars := make([]uint16, 0, 256)
	base := uintptr(unsafe.Pointer(p))
	for i := 0; i < max; i++ {
		ch := *(*uint16)(unsafe.Pointer(base + uintptr(i*2)))
		if ch == 0 {
			break
		}
		chars = append(chars, ch)
	}
	if len(chars) == 0 {
		return ""
	}
	return windows.UTF16ToString(chars)
}

// =====================================================================
// Sigma-Enriched Process Events
// =====================================================================

// trustedOSProcess is an O(1) lookup table of Windows kernel and shell
// infrastructure processes that fire continuously with zero security signal.
// These are hard-coded (not configurable) because they represent immutable
// OS internals — an attacker cannot create a process with these exact names
// from a non-system path and get past ETW's kernel-level PID attribution.
//
// The configurable ExcludeProcesses list in FilterConfig handles user-defined
// exclusions and is applied AFTER event creation in the filter pipeline.
var trustedOSProcess = map[string]bool{
	// Session managers / kernel (PID 0-4 already skipped)
	"conhost.exe": true,
	"wmiprvse.exe": true,

	// Shell / Desktop infrastructure (fire hundreds of times/minute)
	"backgroundtaskhost.exe":    true,
	"applicationframehost.exe":  true,
	"gamebarpresencewriter.exe": true,
	"textinputhost.exe":         true,
	"systemsettings.exe":        true,

	// Search indexing — extremely noisy, no security signal
	"searchprotocolhost.exe": true,
	"searchfilterhost.exe":   true,

	// Audio / Media
	"audiodg.exe":    true,
	"fontdrvhost.exe": true,

	// Peripheral / device infrastructure
	"dashost.exe": true, // Device Association Framework Provider Host
	"ctfmon.exe":  true, // CTF Loader (text input framework)
	"sihost.exe":  true, // Shell Infrastructure Host

	// Windows Update telemetry (periodic, no detection value)
	"compattelrunner.exe":      true,
	"musnotification.exe":      true,
	"microsoftedgeupdate.exe":  true,
	"wuauclt.exe":              true,
}

// isSelfOrChildProcess returns true if the process is the EDR agent itself
// or a child process spawned by it (e.g., PowerShell for WMI/Network queries).
// This prevents a telemetry feedback loop where the agent's own activity
// generates events that flood the pipeline.
func isSelfOrChildProcess(nameLow, cmdLine string) bool {
	// Skip the agent executable itself
	if nameLow == "edr-agent.exe" || nameLow == "agent.exe" {
		return true
	}

	// Skip PowerShell instances launched by the agent for WMI/Network collection.
	// These use -NoProfile and contain known query cmdlets.
	if nameLow == "powershell.exe" && strings.Contains(cmdLine, "-NoProfile") {
		if containsAny(cmdLine,
			"Get-NetTCPConnection", "Get-CimInstance",
			"Get-NetAdapter", "ConvertTo-Json", "ConvertTo-Csv",
			"Win32_Process", "Win32_ComputerSystem") {
			return true
		}
	}

	return false
}

func (c *ETWCollector) processStart(pid, ppid uint32, eventImg, eventCmd string) {
	if isDuplicate(pid) {
		return
	}

	// --- Enrich via Windows APIs (reliable for live processes) ---
	exePath := getImagePath(pid)
	cmdLine := getCmdLine(pid)

	// --- Fallback to event data (short-lived processes) ---
	if exePath == "" {
		exePath = eventImg
	}
	if cmdLine == "" {
		cmdLine = eventCmd
	}

	name := baseName(exePath)
	if name == "" {
		name = exePath
	}
	nameLow := strings.ToLower(name)

	// Hard-coded noise filter — these processes fire constantly with zero
	// security signal. They cannot be abused by attackers (kernel-managed).
	// The configurable FilterConfig.ExcludeProcesses handles additional
	// user-defined exclusions in the pipeline after event creation.
	if trustedOSProcess[nameLow] {
		return
	}

	// Self-exclusion: skip the agent's own child processes.
	// The agent spawns PowerShell for WMI/Network queries; these generate
	// noise and create a telemetry feedback loop.
	if isSelfOrChildProcess(nameLow, cmdLine) {
		return
	}

	if cmdLine == "" {
		cmdLine = exePath
	}
	if exePath == "" {
		exePath = name
	}

	// Sigma enrichment: ParentImage, User
	parentImage := getImagePath(ppid)
	if parentImage == "" {
		parentImage = fmt.Sprintf("pid:%d", ppid)
	}
	userSid, userName, isElevated, integrity := getPrivileges(pid)

	evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, map[string]interface{}{
		"action":           "process_creation",
		"pid":              pid,
		"ppid":             ppid,
		"name":             name,
		"executable":       exePath,
		"command_line":     cmdLine,
		"parent_executable": parentImage,
		"parent_name":      baseName(parentImage),
		"user_sid":         userSid,
		"user_name":        userName,
		"is_elevated":      isElevated,
		"integrity_level":  integrity,
	})
	c.send(evt)
	c.logger.Infof("[ETW] Process START: pid=%d ppid=%d name=%s cmd=%s",
		pid, ppid, name, truncStr(cmdLine, 80))
}

func (c *ETWCollector) processEnd(pid, ppid uint32, eventImg string) {
	name := baseName(getImagePath(pid))
	if name == "" {
		name = baseName(eventImg)
	}
	if name == "" || trustedOSProcess[strings.ToLower(name)] {
		return
	}
	evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, map[string]interface{}{
		"action": "process_termination",
		"pid":    pid,
		"ppid":   ppid,
		"name":   name,
	})
	c.send(evt)
}

// =====================================================================
// Baseline Snapshot (Toolhelp32, runs once)
// =====================================================================

func (c *ETWCollector) baseline() {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snap)

	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))

	table := map[uint32]struct{ n, x string }{}
	if windows.Process32First(snap, &e) == nil {
		for {
			n := windows.UTF16ToString(e.ExeFile[:])
			x := getImagePath(e.ProcessID)
			if x == "" {
				x = n
			}
			table[e.ProcessID] = struct{ n, x string }{n, x}
			if windows.Process32Next(snap, &e) != nil {
				break
			}
		}
	}

	if windows.Process32First(snap, &e) != nil {
		return
	}
	for {
		pid := e.ProcessID
		ppid := e.ParentProcessID
		info := table[pid]
		pinfo := table[ppid]
		cmd := getCmdLine(pid)
		if cmd == "" {
			cmd = info.x
		}
		sid, user, elev, integ := getPrivileges(pid)
		evt := event.NewEvent(event.EventTypeProcess, event.SeverityLow, map[string]interface{}{
			"action": "snapshot", "pid": pid, "ppid": ppid,
			"name": info.n, "executable": info.x, "command_line": cmd,
			"parent_name": pinfo.n, "parent_executable": pinfo.x,
			"user_sid": sid, "user_name": user,
			"is_elevated": elev, "integrity_level": integ,
		})
		c.send(evt)
		if windows.Process32Next(snap, &e) != nil {
			break
		}
	}
}

// =====================================================================
// Windows API Helpers
// =====================================================================

func getImagePath(pid uint32) string {
	if pid == 0 || pid == 4 {
		return ""
	}
	h, err := windows.OpenProcess(0x1000, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	var buf [windows.MAX_PATH]uint16
	sz := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &sz) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:sz])
}

var (
	ntdll    = windows.NewLazyDLL("ntdll.dll")
	ntqip    = ntdll.NewProc("NtQueryInformationProcess")
)

func getCmdLine(pid uint32) string {
	if pid == 0 || pid == 4 {
		return ""
	}
	h, err := windows.OpenProcess(0x1000, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)

	const infoCls = 60
	var retLen uint32
	buf := make([]byte, 1024)
	r, _, _ := ntqip.Call(uintptr(h), infoCls,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
		uintptr(unsafe.Pointer(&retLen)))

	if r == 0xC0000004 && retLen > 0 && retLen < 65536 {
		buf = make([]byte, retLen)
		r, _, _ = ntqip.Call(uintptr(h), infoCls,
			uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
			uintptr(unsafe.Pointer(&retLen)))
	}
	if r != 0 || retLen < 8 {
		return ""
	}

	length := *(*uint16)(unsafe.Pointer(&buf[0]))
	if length == 0 || int(length)+16 > len(buf) {
		return ""
	}
	ptr := *(*uintptr)(unsafe.Pointer(&buf[8]))
	base := uintptr(unsafe.Pointer(&buf[0]))
	off := int(ptr - base)
	if off < 0 || off+int(length) > len(buf) {
		return ""
	}
	s := make([]uint16, length/2)
	for i := range s {
		s[i] = *(*uint16)(unsafe.Pointer(&buf[off+i*2]))
	}
	return windows.UTF16ToString(s)
}

func getPrivileges(pid uint32) (sid, user string, elevated bool, integrity string) {
	if pid == 0 || pid == 4 {
		return
	}
	h, err := windows.OpenProcess(0x1000, false, pid)
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)

	var tok windows.Token
	if windows.OpenProcessToken(h, windows.TOKEN_QUERY, &tok) != nil {
		return
	}
	defer tok.Close()

	if u, err := tok.GetTokenUser(); err == nil {
		sid = u.User.Sid.String()
		if acct, dom, _, err := u.User.Sid.LookupAccount(""); err == nil {
			user = dom + `\` + acct
		}
	}
	elevated = tok.IsElevated()

	var isz uint32
	windows.GetTokenInformation(tok, windows.TokenIntegrityLevel, nil, 0, &isz)
	if isz > 0 {
		ib := make([]byte, isz)
		if windows.GetTokenInformation(tok, windows.TokenIntegrityLevel, &ib[0], isz, &isz) == nil {
			tml := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&ib[0]))
			switch tml.Label.Sid.String() {
			case "S-1-16-4096":
				integrity = "Low"
			case "S-1-16-8192":
				integrity = "Medium"
			case "S-1-16-12288":
				integrity = "High"
			case "S-1-16-16384":
				integrity = "System"
			default:
				integrity = tml.Label.Sid.String()
			}
		}
	}
	return
}

// =====================================================================
// Utility
// =====================================================================

func (c *ETWCollector) send(evt *event.Event) {
	select {
	case c.eventChan <- evt:
		c.collected.Add(1)
	default:
		c.dropped.Add(1)
	}
}

func baseName(p string) string {
	if i := strings.LastIndex(p, `\`); i >= 0 {
		return p[i+1:]
	}
	if i := strings.LastIndex(p, `/`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// Public API for agent
func (c *ETWCollector) IsRunning() bool          { return c.running.Load() }
func (c *ETWCollector) Stats() ETWStats {
	return ETWStats{c.running.Load(), c.collected.Load(), c.dropped.Load(), c.errors.Load()}
}

type ETWStats struct {
	Running         bool
	EventsCollected uint64
	EventsDropped   uint64
	Errors          uint64
}
