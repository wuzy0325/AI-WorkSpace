#include <windows.h>

typedef int (*veh_callback_t)(
    DWORD code,
    ULONG_PTR addr,
    DWORD flags,
    ULONG_PTR accessAddr,
    int accessType);

extern int goVEHCallback(
    DWORD code,
    ULONG_PTR addr,
    DWORD flags,
    ULONG_PTR accessAddr,
    int accessType);

static veh_callback_t g_callback = NULL;
static PVOID g_veh_handle = NULL;
static PVOID g_veh_cont_handle = NULL;
static __thread DWORD g_handled_code = 0;

static LONG CALLBACK veh_handler(PEXCEPTION_POINTERS ep)
{
    if (g_callback != NULL) {
        PEXCEPTION_RECORD rec = ep->ExceptionRecord;
        ULONG_PTR accessAddr = 0;
        int accessType = -1;

        if (rec->ExceptionCode == EXCEPTION_ACCESS_VIOLATION
            && rec->NumberParameters >= 2) {
            accessAddr = rec->ExceptionInformation[1];
            accessType = (int)rec->ExceptionInformation[0];
        }

        int action = g_callback(
            rec->ExceptionCode,
            (ULONG_PTR)rec->ExceptionAddress,
            rec->ExceptionFlags,
            accessAddr,
            accessType);

        if (action != 0) {
            g_handled_code = rec->ExceptionCode;
            return EXCEPTION_CONTINUE_EXECUTION;
        }
    }
    return EXCEPTION_CONTINUE_SEARCH;
}

static LONG CALLBACK veh_continue_handler(PEXCEPTION_POINTERS ep)
{
    if (ep->ExceptionRecord->ExceptionCode == g_handled_code
        && g_handled_code != 0) {
        g_handled_code = 0;
        return EXCEPTION_CONTINUE_EXECUTION;
    }
    return EXCEPTION_CONTINUE_SEARCH;
}

void veh_start(void)
{
    g_veh_handle = AddVectoredExceptionHandler(1, veh_handler);
    g_veh_cont_handle = AddVectoredContinueHandler(1, veh_continue_handler);
}

void veh_stop(void)
{
    if (g_veh_handle != NULL) {
        RemoveVectoredExceptionHandler(g_veh_handle);
        g_veh_handle = NULL;
    }
    if (g_veh_cont_handle != NULL) {
        RemoveVectoredContinueHandler(g_veh_cont_handle);
        g_veh_cont_handle = NULL;
    }
    g_callback = NULL;
}

void veh_init_callback(void)
{
    g_callback = goVEHCallback;
}

static __thread int g_test_handled = 0;
static __thread DWORD g_test_code = 0;

static LONG CALLBACK test_safety_handler(PEXCEPTION_POINTERS ep)
{
    DWORD code = ep->ExceptionRecord->ExceptionCode;
    if (code >= 0xE0000000 && code < 0xE0001000) {
        g_test_handled = 0;
        g_test_code = code;
        return EXCEPTION_CONTINUE_EXECUTION;
    }
    return EXCEPTION_CONTINUE_SEARCH;
}

static LONG CALLBACK test_safety_continue_handler(PEXCEPTION_POINTERS ep)
{
    if (ep->ExceptionRecord->ExceptionCode == g_test_code
        && g_test_code != 0) {
        g_test_code = 0;
        return EXCEPTION_CONTINUE_EXECUTION;
    }
    return EXCEPTION_CONTINUE_SEARCH;
}

int veh_raise_exception(DWORD code, int *handled_by_veh)
{
    PVOID safety;
    PVOID safety_cont;
    g_test_handled = 1;
    safety = AddVectoredExceptionHandler(0, test_safety_handler);
    safety_cont = AddVectoredContinueHandler(1, test_safety_continue_handler);
    RaiseException(code, 0, 0, NULL);
    *handled_by_veh = g_test_handled;
    RemoveVectoredContinueHandler(safety_cont);
    RemoveVectoredExceptionHandler(safety);
    return 0;
}

static PVOID g_simple_handle = NULL;
static PVOID g_simple_cont_handle = NULL;
static __thread DWORD g_last_handled_code = 0;
static int g_simple_called = 0;
static DWORD g_simple_code = 0;
static ULONG_PTR g_simple_addr = 0;

static LONG CALLBACK simple_handler(PEXCEPTION_POINTERS ep)
{
    g_simple_called = 1;
    g_simple_code = ep->ExceptionRecord->ExceptionCode;
    g_simple_addr = (ULONG_PTR)ep->ExceptionRecord->ExceptionAddress;
    g_last_handled_code = ep->ExceptionRecord->ExceptionCode;
    return EXCEPTION_CONTINUE_EXECUTION;
}

static LONG CALLBACK simple_continue_handler(PEXCEPTION_POINTERS ep)
{
    if (ep->ExceptionRecord->ExceptionCode == g_last_handled_code
        && g_last_handled_code != 0) {
        g_last_handled_code = 0;
        return EXCEPTION_CONTINUE_EXECUTION;
    }
    return EXCEPTION_CONTINUE_SEARCH;
}

void veh_simple_start(void)
{
    g_simple_handle = AddVectoredExceptionHandler(1, simple_handler);
    g_simple_cont_handle = AddVectoredContinueHandler(1, simple_continue_handler);
}

void veh_simple_stop(void)
{
    if (g_simple_handle != NULL) {
        RemoveVectoredExceptionHandler(g_simple_handle);
        g_simple_handle = NULL;
    }
    if (g_simple_cont_handle != NULL) {
        RemoveVectoredContinueHandler(g_simple_cont_handle);
        g_simple_cont_handle = NULL;
    }
}

int veh_simple_start_ok(void)
{
    return g_simple_handle != NULL ? 1 : 0;
}

int veh_simple_raise(DWORD code)
{
    PVOID safety;
    PVOID safety_cont;
    g_simple_called = 0;
    safety = AddVectoredExceptionHandler(0, test_safety_handler);
    safety_cont = AddVectoredContinueHandler(1, test_safety_continue_handler);
    RaiseException(code, 0, 0, NULL);
    RemoveVectoredContinueHandler(safety_cont);
    RemoveVectoredExceptionHandler(safety);
    return g_simple_called;
}

void veh_trigger_read(void *addr)
{
    volatile char c = *(volatile char *)addr;
    (void)c;
}
