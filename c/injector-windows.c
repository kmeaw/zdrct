#define _GNU_SOURCE
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <fcntl.h>
#include <string.h>
#include <unistd.h>

#include "injector.h"

extern void* cdocommand_ptr;
extern unsigned long rconserver(void*) __attribute__((stdcall));

#define STACK_SIZE (8 * 1024 * 1024)

static void *memmem(const void *haystack, size_t haystack_len,
                    const void * const needle, const size_t needle_len)
{
    if (haystack == NULL) return NULL; // or assert(haystack != NULL);
    if (haystack_len == 0) return NULL;
    if (needle == NULL) return NULL; // or assert(needle != NULL);
    if (needle_len == 0) return NULL;

    for (const char *h = haystack;
            haystack_len >= needle_len;
            ++h, --haystack_len) {
        if (!memcmp(h, needle, needle_len)) {
            return (void *) h;
        }
    }
    return NULL;
}

static void* scan(const DWORD perm_filter, void*(*cb)(void *, size_t, void *), void *uptr) {
    PBYTE ptr = NULL, pnext = NULL;
    MEMORY_BASIC_INFORMATION mbi;
    HMODULE current = GetModuleHandle(NULL);
    while(VirtualQuery(pnext, &mbi, sizeof(mbi)) == sizeof(mbi)) {
        pnext = mbi.BaseAddress + mbi.RegionSize;
        if (mbi.AllocationBase != current) continue;
        if (mbi.State == MEM_FREE) continue;
        if (mbi.Protect != perm_filter) continue;

        printf("base %p abase %p state %x protect %x uptr %p\n", mbi.BaseAddress, mbi.AllocationBase, mbi.State, mbi.Protect, uptr);
        ptr = cb((void *) mbi.BaseAddress, (size_t) mbi.RegionSize, uptr);
        if (ptr != NULL) break;
    }

    return ptr;
}

static void do_inject () {
    void *script_error = scan(PAGE_READONLY, search_string_cb, "\034GScript error, \"%s\" line %d:");
    void *toggle_idmypos = scan(PAGE_READONLY, search_string_cb, "toggle idmypos");

    if (script_error == NULL || toggle_idmypos == NULL)
        return;

    printf("script_error = %p\n", script_error);
    printf("toggle_idmypos = %p\n", toggle_idmypos);

    void* printf_ptr = scan(PAGE_EXECUTE_READ, search_data_ref, script_error);
    cdocommand_ptr = scan(PAGE_EXECUTE_READ, search_data_ref, toggle_idmypos);
    printf("Printf = %p\n", printf_ptr);
    printf("C_DoCommand = %p\n", cdocommand_ptr);

    if (CreateThread(NULL, STACK_SIZE, rconserver, NULL, 0, NULL) == INVALID_HANDLE_VALUE) {
        perror("clone");
    }
}

BOOL WINAPI DllMain(HANDLE hDllHandle, DWORD dwReason, LPVOID lpreserved) {
    if (dwReason == DLL_PROCESS_ATTACH) {
        puts("injector: in DllMain");
        do_inject();
    }

    return TRUE;
}

void WINAPI __declspec(dllexport) empty_function_dummy() {}
