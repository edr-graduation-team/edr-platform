// TDH-based ETW process event parser.
// All parsing happens SYNCHRONOUSLY in the C callback so data is
// captured before short-lived processes exit.

#include "etw_cgo.h"
#include <string.h>

// Go callback (defined in etw.go via //export)
extern void goProcessEvent(ParsedProcessEvent* evt);

// =====================================================================
// TDH Property Helpers
// =====================================================================

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

// =====================================================================
// Manual C Fallback (if TDH unavailable for classic events)
// Handles SID variable length + WCHAR alignment correctly.
// =====================================================================

static void manualParseV4(PEVENT_RECORD rec, ParsedProcessEvent* out) {
    BYTE* ud  = (BYTE*)rec->UserData;
    int   len = (int)rec->UserDataLength;
    if (!ud || len < 36) return;

    out->processId = *(ULONG*)(ud + 8);
    out->parentId  = *(ULONG*)(ud + 12);

    // SID at offset 36
    if (len <= 44) return;        // need at least SID header + some data
    BYTE rev = ud[36];
    int  sac = (int)ud[37];      // SubAuthorityCount
    if (rev != 1 || sac > 15) return;
    int sidLen   = 8 + sac * 4;
    int imgStart = 36 + sidLen;
    if (imgStart >= len) return;

    // ImageFileName: ANSI null-terminated
    int n = 0;
    for (int i = imgStart; i < len && ud[i] != 0 && n < 259; i++)
        out->imageFileName[n++] = (char)ud[i];
    out->imageFileName[n] = 0;

    // CommandLine: WCHAR-aligned, null-terminated
    int cmdOff = imgStart + n + 1;          // past ANSI null
    if (cmdOff % 2 != 0) cmdOff++;          // align to 2-byte
    if (cmdOff + 2 > len) return;

    WCHAR* src  = (WCHAR*)(ud + cmdOff);
    int maxCh   = (len - cmdOff) / 2;
    int w = 0;
    for (int i = 0; i < maxCh && src[i] != 0 && w < 4095; i++)
        out->commandLine[w++] = src[i];
    out->commandLine[w] = 0;
}

// =====================================================================
// Parse a Process Event (TDH primary, manual fallback)
// =====================================================================

static void parseProcessEvent(PEVENT_RECORD rec, ParsedProcessEvent* out) {
    // --- TDH extraction (works for MOF+manifest events) ---
    tdhGetAnsi  (rec, L"ImageFileName", out->imageFileName, sizeof(out->imageFileName));
    tdhGetUnicode(rec, L"CommandLine",  out->commandLine,   sizeof(out->commandLine));

    // PID/PPID: always read from fixed offsets (reliable, avoids TDH overhead)
    BYTE* ud = (BYTE*)rec->UserData;
    if (ud && rec->UserDataLength >= 16) {
        out->processId = *(ULONG*)(ud + 8);
        out->parentId  = *(ULONG*)(ud + 12);
    }

    // --- Fallback: if TDH didn't extract fields, use manual C parser ---
    if (out->imageFileName[0] == 0) {
        manualParseV4(rec, out);
    }
}

// =====================================================================
// Event Callback (stdcall wrapper)
// =====================================================================

static void WINAPI stdcallEventCallback(PEVENT_RECORD eventRecord) {
    // STRICT: only Process Start (1) and Process End (2)
    BYTE opcode = eventRecord->EventHeader.EventDescriptor.Opcode;
    if (opcode != 1 && opcode != 2) return;

    ParsedProcessEvent evt;
    memset(&evt, 0, sizeof(evt));
    evt.opcode = opcode;

    parseProcessEvent(eventRecord, &evt);

    if (evt.processId <= 4) return;   // Skip Idle/System

    goProcessEvent(&evt);
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
    p->EnableFlags         = EVENT_TRACE_FLAG_PROCESS;
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
        p->EnableFlags = EVENT_TRACE_FLAG_PROCESS;
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
