// TDH-only ETW kernel event parser.
// Handles PROCESS, IMAGE LOAD, and FILE I/O events.
//
// CRITICAL: ALL field extraction uses Microsoft's TDH (Trace Data Helper)
// API to dynamically resolve property offsets by name. NO manual byte
// offsets or pointer arithmetic is used anywhere in this file.
// This ensures correctness across all Windows versions and architectures.
//
// All parsing happens SYNCHRONOUSLY in the C callback so data is
// captured before short-lived processes exit or DLLs unload.

#include "etw_cgo.h"
#include <string.h>

// Go callbacks (defined in etw.go via //export)
extern void goProcessEvent(ParsedProcessEvent* evt);
extern void goImageLoadEvent(ParsedImageLoadEvent* evt);
extern void goFileIoEvent(ParsedFileIoEvent* evt);

// =====================================================================
// Well-known Kernel Trace Provider GUIDs
// =====================================================================

// {3D6FA8D0-FE05-11D0-9DDA-00C04FD7BA7C} — Process events
static const GUID ProcessProviderGuid =
    {0x3D6FA8D0, 0xFE05, 0x11D0, {0x9D,0xDA,0x00,0xC0,0x4F,0xD7,0xBA,0x7C}};

// {2CB15D1D-5FC1-11D2-ABE1-00A0C911F518} — Image Load events
static const GUID ImageLoadProviderGuid =
    {0x2CB15D1D, 0x5FC1, 0x11D2, {0xAB,0xE1,0x00,0xA0,0xC9,0x11,0xF5,0x18}};

// {90CBDC39-4A3E-11D1-84F4-0000F80464E3} — File I/O events
static const GUID FileIoProviderGuid =
    {0x90CBDC39, 0x4A3E, 0x11D1, {0x84,0xF4,0x00,0x00,0xF8,0x04,0x64,0xE3}};

// =====================================================================
// TDH Property Helpers — all field extraction goes through these
// =====================================================================

// Extract an ANSI string property by name.
static int tdhGetAnsi(PEVENT_RECORD rec, LPCWSTR name, char* out, ULONG sz) {
    PROPERTY_DATA_DESCRIPTOR d;
    memset(&d, 0, sizeof(d));
    d.PropertyName = (ULONGLONG)name;
    d.ArrayIndex   = ULONG_MAX;
    ULONG ps = 0;
    if (TdhGetPropertySize(rec,0,NULL,1,&d,&ps) != ERROR_SUCCESS || ps==0 || ps>=sz)
        return 1;
    return TdhGetProperty(rec,0,NULL,1,&d,ps,(PBYTE)out)==ERROR_SUCCESS ? 0:1;
}

// Extract a Unicode string property by name.
static int tdhGetUnicode(PEVENT_RECORD rec, LPCWSTR name, WCHAR* out, ULONG bytes) {
    PROPERTY_DATA_DESCRIPTOR d;
    memset(&d, 0, sizeof(d));
    d.PropertyName = (ULONGLONG)name;
    d.ArrayIndex   = ULONG_MAX;
    ULONG ps = 0;
    if (TdhGetPropertySize(rec,0,NULL,1,&d,&ps) != ERROR_SUCCESS || ps==0 || ps>=bytes)
        return 1;
    return TdhGetProperty(rec,0,NULL,1,&d,ps,(PBYTE)out)==ERROR_SUCCESS ? 0:1;
}

// Extract a ULONG (32-bit) property by name.
static int tdhGetULONG(PEVENT_RECORD rec, LPCWSTR name, ULONG* out) {
    PROPERTY_DATA_DESCRIPTOR d;
    memset(&d, 0, sizeof(d));
    d.PropertyName = (ULONGLONG)name;
    d.ArrayIndex   = ULONG_MAX;
    ULONG ps = 0;
    if (TdhGetPropertySize(rec,0,NULL,1,&d,&ps) != ERROR_SUCCESS || ps==0)
        return 1;
    // Property may be 4 or 8 bytes depending on architecture.
    // Always read into a temporary buffer and extract safely.
    BYTE buf[8];
    memset(buf, 0, sizeof(buf));
    ULONG readSz = ps > 8 ? 8 : ps;
    if (TdhGetProperty(rec,0,NULL,1,&d,readSz,buf) != ERROR_SUCCESS)
        return 1;
    *out = *(ULONG*)buf;
    return 0;
}

// Extract a ULONGLONG (64-bit) / pointer-sized property by name.
// Handles both 32-bit and 64-bit event payloads transparently —
// TDH reports the actual property size, so we read exactly what it gives us.
static int tdhGetPointer(PEVENT_RECORD rec, LPCWSTR name, ULONGLONG* out) {
    PROPERTY_DATA_DESCRIPTOR d;
    memset(&d, 0, sizeof(d));
    d.PropertyName = (ULONGLONG)name;
    d.ArrayIndex   = ULONG_MAX;
    ULONG ps = 0;
    if (TdhGetPropertySize(rec,0,NULL,1,&d,&ps) != ERROR_SUCCESS || ps==0)
        return 1;
    BYTE buf[8];
    memset(buf, 0, sizeof(buf));
    ULONG readSz = ps > 8 ? 8 : ps;
    if (TdhGetProperty(rec,0,NULL,1,&d,readSz,buf) != ERROR_SUCCESS)
        return 1;
    if (ps <= 4) {
        *out = (ULONGLONG)(*(ULONG*)buf);
    } else {
        *out = *(ULONGLONG*)buf;
    }
    return 0;
}

// =====================================================================
// Parse Process Event — TDH ONLY, no manual offsets
//
// TDH properties for kernel Process events:
//   ProcessId      (ULONG)
//   ParentId       (ULONG)
//   ImageFileName  (ANSI String)
//   CommandLine    (Unicode String)
// =====================================================================

static void parseProcessEvent(PEVENT_RECORD rec, ParsedProcessEvent* out) {
    // PID and PPID via TDH — NOT from hardcoded byte offsets.
    tdhGetULONG(rec, L"ProcessId",  &out->processId);
    tdhGetULONG(rec, L"ParentId",   &out->parentId);

    // Image name and command line via TDH.
    tdhGetAnsi  (rec, L"ImageFileName", out->imageFileName, sizeof(out->imageFileName));
    tdhGetUnicode(rec, L"CommandLine",  out->commandLine,   sizeof(out->commandLine));
}

// =====================================================================
// Parse Image Load Event — TDH ONLY, no manual offsets
//
// TDH properties for kernel Image Load events:
//   FileName    (Unicode String)
//   ImageBase   (Pointer — 4 or 8 bytes depending on architecture)
//   ImageSize   (Pointer — 4 or 8 bytes depending on architecture)
// PID/TID come from EventHeader (set by the caller before this function).
// =====================================================================

static void parseImageLoadEvent(PEVENT_RECORD rec, ParsedImageLoadEvent* out) {
    // Image path via TDH.
    tdhGetUnicode(rec, L"FileName", out->imagePath, sizeof(out->imagePath));

    // ImageBase and ImageSize via TDH — handles 32/64-bit transparently.
    tdhGetPointer(rec, L"ImageBase", &out->imageBase);
    ULONGLONG imgSz = 0;
    if (tdhGetPointer(rec, L"ImageSize", &imgSz) == 0) {
        out->imageSize = (ULONG)imgSz;
    }
}

// =====================================================================
// Parse File I/O Event — TDH ONLY
//
// TDH properties for kernel File I/O events:
//   OpenPath  (Unicode String — used by Create opcode 64)
//   FileName  (Unicode String — used by other opcodes)
// PID/TID come from EventHeader (set by the caller before this function).
// =====================================================================

static void parseFileIoEvent(PEVENT_RECORD rec, ParsedFileIoEvent* out) {
    // Try OpenPath first (Create events use this field name)
    if (tdhGetUnicode(rec, L"OpenPath", out->filePath, sizeof(out->filePath)) != 0) {
        // Fallback to FileName (used by Write, Delete, Rename)
        tdhGetUnicode(rec, L"FileName", out->filePath, sizeof(out->filePath));
    }
}

// =====================================================================
// GUID comparison helper
// =====================================================================

static int guidsEqual(const GUID* a, const GUID* b) {
    return memcmp(a, b, sizeof(GUID)) == 0;
}

// =====================================================================
// Event Callback — routes events by provider GUID
// =====================================================================

static void WINAPI stdcallEventCallback(PEVENT_RECORD eventRecord) {
    const GUID* provider = &eventRecord->EventHeader.ProviderId;
    BYTE opcode = eventRecord->EventHeader.EventDescriptor.Opcode;

    // ---- PROCESS events (Start=1, End=2) ----
    if (guidsEqual(provider, &ProcessProviderGuid)) {
        if (opcode != 1 && opcode != 2) return;

        ParsedProcessEvent evt;
        memset(&evt, 0, sizeof(evt));
        evt.opcode = opcode;

        parseProcessEvent(eventRecord, &evt);

        if (evt.processId <= 4) return;   // Skip Idle/System

        goProcessEvent(&evt);
        return;
    }

    // ---- IMAGE LOAD events (Load=10, Unload=2/3) ----
    if (guidsEqual(provider, &ImageLoadProviderGuid)) {
        if (opcode != 10 && opcode != 2 && opcode != 3) return;

        ParsedImageLoadEvent evt;
        memset(&evt, 0, sizeof(evt));
        evt.opcode    = opcode;
        evt.processId = eventRecord->EventHeader.ProcessId;
        evt.threadId  = eventRecord->EventHeader.ThreadId;

        parseImageLoadEvent(eventRecord, &evt);

        if (evt.processId <= 4) return;

        goImageLoadEvent(&evt);
        return;
    }

    // ---- FILE I/O events (Create=64, Write=68, Delete=70, Rename=71) ----
    if (guidsEqual(provider, &FileIoProviderGuid)) {
        if (opcode != 64 && opcode != 68 && opcode != 70 && opcode != 71) return;

        ParsedFileIoEvent evt;
        memset(&evt, 0, sizeof(evt));
        evt.opcode    = opcode;
        evt.processId = eventRecord->EventHeader.ProcessId;
        evt.threadId  = eventRecord->EventHeader.ThreadId;

        parseFileIoEvent(eventRecord, &evt);

        if (evt.processId <= 4) return;
        if (evt.filePath[0] == 0) return;  // No path — skip

        goFileIoEvent(&evt);
        return;
    }
}

// =====================================================================
// Session Management
// =====================================================================

static TRACEHANDLE g_sessionHandle = 0;
static BYTE        g_propBuf[4096];

int StartKernelProcessSession(
    const wchar_t* sessionName,
    const GUID*    providerGuid,
    UCHAR          level,
    ULONGLONG      matchAnyKeyword)
{
    int nameLen = (int)(wcslen(sessionName)+1) * (int)sizeof(wchar_t);
    int bufSize = (int)sizeof(EVENT_TRACE_PROPERTIES) + nameLen + 1024;
    if (bufSize > (int)sizeof(g_propBuf)) bufSize = (int)sizeof(g_propBuf);

    // Kill orphan first
    KillNamedSession(sessionName);
    Sleep(200);

    memset(g_propBuf, 0, sizeof(g_propBuf));
    PEVENT_TRACE_PROPERTIES p = (PEVENT_TRACE_PROPERTIES)g_propBuf;
    p->Wnode.BufferSize    = (ULONG)bufSize;
    p->Wnode.ClientContext = 1;
    p->Wnode.Flags         = WNODE_FLAG_TRACED_GUID;
    p->LogFileMode         = EVENT_TRACE_REAL_TIME_MODE | EVENT_TRACE_SYSTEM_LOGGER_MODE;
    // Enable PROCESS + IMAGE LOAD + FILE I/O events in a single kernel session
    p->EnableFlags         = EVENT_TRACE_FLAG_PROCESS
                           | EVENT_TRACE_FLAG_IMAGE_LOAD
                           | EVENT_TRACE_FLAG_FILE_IO_INIT;
    p->BufferSize          = 64;
    p->LoggerNameOffset    = (ULONG)sizeof(EVENT_TRACE_PROPERTIES);
    p->LogFileNameOffset   = 0;

    ULONG st = StartTraceW(&g_sessionHandle, sessionName, p);
    if (st == ERROR_ALREADY_EXISTS) {
        KillNamedSession(sessionName);
        Sleep(200);
        memset(g_propBuf,0,sizeof(g_propBuf));
        p->Wnode.BufferSize = (ULONG)bufSize;
        p->Wnode.ClientContext=1; p->Wnode.Flags=WNODE_FLAG_TRACED_GUID;
        p->LogFileMode = EVENT_TRACE_REAL_TIME_MODE|EVENT_TRACE_SYSTEM_LOGGER_MODE;
        p->EnableFlags = EVENT_TRACE_FLAG_PROCESS
                       | EVENT_TRACE_FLAG_IMAGE_LOAD
                       | EVENT_TRACE_FLAG_FILE_IO_INIT;
        p->BufferSize=64; p->LoggerNameOffset=(ULONG)sizeof(EVENT_TRACE_PROPERTIES);
        st = StartTraceW(&g_sessionHandle, sessionName, p);
    }
    return st == ERROR_SUCCESS ? 0 : (int)st;
}

int ProcessKernelEvents(const wchar_t* sessionName, void* ctx) {
    EVENT_TRACE_LOGFILEW t;
    memset(&t,0,sizeof(t));
    t.LoggerName          = (LPWSTR)sessionName;
    t.Context             = ctx;
    t.ProcessTraceMode    = PROCESS_TRACE_MODE_REAL_TIME | PROCESS_TRACE_MODE_EVENT_RECORD;
    t.EventRecordCallback = stdcallEventCallback;

    TRACEHANDLE h = OpenTraceW(&t);
    if (h == INVALID_PROCESSTRACE_HANDLE) return (int)GetLastError();
    ULONG st = ProcessTrace(&h,1,NULL,NULL);
    CloseTrace(h);
    return (st==ERROR_SUCCESS||st==ERROR_CANCELLED) ? 0 : (int)st;
}

void StopKernelSession(const GUID* g) {
    BYTE b[4096]; memset(b,0,sizeof(b));
    PEVENT_TRACE_PROPERTIES p = (PEVENT_TRACE_PROPERTIES)b;
    p->Wnode.BufferSize = sizeof(b);
    ControlTraceW(g_sessionHandle, NULL, p, EVENT_TRACE_CONTROL_STOP);
    g_sessionHandle = 0;
}

int KillNamedSession(const wchar_t* name) {
    int nl = (int)(wcslen(name)+1)*2;
    int bs = (int)sizeof(EVENT_TRACE_PROPERTIES)+nl+1024;
    BYTE b[4096]; if(bs>(int)sizeof(b)) bs=(int)sizeof(b);
    memset(b,0,sizeof(b));
    ((PEVENT_TRACE_PROPERTIES)b)->Wnode.BufferSize = (ULONG)bs;
    ULONG st = ControlTraceW(0,name,(PEVENT_TRACE_PROPERTIES)b,EVENT_TRACE_CONTROL_STOP);
    return (st==ERROR_SUCCESS||st==ERROR_MORE_DATA)?0:(int)st;
}
