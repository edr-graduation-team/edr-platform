#ifndef ETW_CGO_H
#define ETW_CGO_H

#include <windows.h>
#include <evntrace.h>
#include <evntcons.h>
#include <tdh.h>
#include <wchar.h>

#ifndef INVALID_PROCESSTRACE_HANDLE
#define INVALID_PROCESSTRACE_HANDLE ((TRACEHANDLE)INVALID_HANDLE_VALUE)
#endif

#ifndef EVENT_TRACE_FLAG_PROCESS
#define EVENT_TRACE_FLAG_PROCESS 0x00000001
#endif

#ifndef EVENT_TRACE_SYSTEM_LOGGER_MODE
#define EVENT_TRACE_SYSTEM_LOGGER_MODE 0x02000000
#endif

// Pre-parsed process event struct passed from C to Go.
// All field extraction happens synchronously in the C callback
// so that data is captured before the process exits.
typedef struct {
    DWORD processId;
    DWORD parentId;
    BYTE  opcode;       // 1=Start, 2=End
    BYTE  _pad[3];
    char  imageFileName[260];   // ANSI short name from kernel event
    WCHAR commandLine[4096];    // Unicode full command line
} ParsedProcessEvent;

// Session management
int StartKernelProcessSession(
    const wchar_t* sessionName,
    const GUID*    providerGuid,
    UCHAR          level,
    ULONGLONG      matchAnyKeyword);

int ProcessKernelEvents(const wchar_t* sessionName, void* goContextKey);
void StopKernelSession(const GUID* providerGuid);
int KillNamedSession(const wchar_t* sessionName);

#endif // ETW_CGO_H
