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

#ifndef EVENT_TRACE_FLAG_IMAGE_LOAD
#define EVENT_TRACE_FLAG_IMAGE_LOAD 0x00000004
#endif

#ifndef EVENT_TRACE_FLAG_FILE_IO_INIT
#define EVENT_TRACE_FLAG_FILE_IO_INIT 0x00000400
#endif

#ifndef EVENT_TRACE_SYSTEM_LOGGER_MODE
#define EVENT_TRACE_SYSTEM_LOGGER_MODE 0x02000000
#endif

// =======================================================================
// Pre-parsed event structs passed from C to Go.
// All field extraction happens synchronously in the C callback
// so that data is captured before short-lived processes exit.
// =======================================================================

// Process create / terminate event.
typedef struct {
    DWORD processId;
    DWORD parentId;
    BYTE  opcode;       // 1=Start, 2=End
    BYTE  _pad[3];
    char  imageFileName[260];   // ANSI short name from kernel event
    WCHAR commandLine[4096];    // Unicode full command line
} ParsedProcessEvent;

// Image (DLL/EXE) load / unload event.
typedef struct {
    DWORD     processId;
    DWORD     threadId;
    BYTE      opcode;       // 10=Load, 2/3=Unload
    BYTE      _pad[3];
    ULONGLONG imageBase;
    ULONG     imageSize;
    WCHAR     imagePath[1024];  // Full Unicode path to the loaded image
} ParsedImageLoadEvent;

// File I/O event (create, write, delete, rename).
typedef struct {
    DWORD processId;
    DWORD threadId;
    BYTE  opcode;       // 64=Create, 68=Write, 70=Delete, 71=Rename
    BYTE  _pad[3];
    WCHAR filePath[1024];       // Full Unicode path of the target file
} ParsedFileIoEvent;

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
