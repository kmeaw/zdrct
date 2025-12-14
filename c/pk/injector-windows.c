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

extern void** gscript;
extern void* cdocommand_ptr;
extern void* gc_ptr;
extern void rconserver_start(void) __attribute__((stdcall));
extern void* rconserver_tick(void*, const char*) __attribute__((stdcall));

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
//    HMODULE current = GetModuleHandle(NULL);
    while(VirtualQuery(pnext, &mbi, sizeof(mbi)) == sizeof(mbi)) {
        pnext = mbi.BaseAddress + mbi.RegionSize;
//        if (mbi.AllocationBase != current) continue;
        if (mbi.State == MEM_FREE) continue;
        if (mbi.Protect != perm_filter) continue;

        printf("base %p abase %p state %lx protect %lx uptr %p\n", mbi.BaseAddress, mbi.AllocationBase, mbi.State, mbi.Protect, uptr);
        ptr = cb((void *) mbi.BaseAddress, (size_t) mbi.RegionSize, uptr);
        if (ptr != NULL) break;
    }

    return ptr;
}

static void do_inject () {
    void *cfg_save = scan(PAGE_READONLY, search_string_cb, "Cfg:Save()");
    void *game_gc = scan(PAGE_READONLY, search_string_cb, "Game_GC");

    if (cfg_save == NULL || game_gc == NULL)
        return;

    printf("cfg_save = %p\n", cfg_save);
    printf("game_gc = %p\n", game_gc);

    void **gscript_ref = scan(PAGE_EXECUTE_READ, search_data_ref_ecx_push_ptr32, cfg_save);
    printf("GScript ref = %p\n", gscript_ref);
    printf("GScript = %p\n", *gscript_ref);
    gscript = *gscript_ref;

    cdocommand_ptr = scan(PAGE_EXECUTE_READ, search_data_ref_ecx, cfg_save);
    printf("C_DoCommand = %p\n", cdocommand_ptr);

    int32_t *call = scan(PAGE_EXECUTE_READ, search_data_ref_push_push_call, game_gc);
    if (call == NULL) {
	printf("Cannot find data push reference to Game_PostRender");
	return;
    }
    gc_ptr = ((char *)(call + 1)) + (*call);
    int32_t new_call = ((char *) ((void *) rconserver_tick)) - ((char *) (call + 1));
    printf("gc = %p\n", gc_ptr);
    printf("rconserver_tick = %p\n", rconserver_tick);
    printf("tick call site = %p\n", ((char *) call) - 1);
    printf("Going to overwrite 0x%08x with 0x%08x\n", *call, new_call);
    if (!WriteProcessMemory (GetCurrentProcess(), call, &new_call, sizeof(new_call), 0)) {
	printf("WriteProcessMemory has failed: error 0x%lx.\n", GetLastError());
	return;
    }

    rconserver_start();
}

BOOL WINAPI DllMain(__attribute__((unused)) HANDLE hDllHandle, __attribute__((unused)) DWORD dwReason, __attribute__ ((unused)) LPVOID lpreserved) {
    if (dwReason == DLL_PROCESS_ATTACH) {
        AllocConsole();
        freopen("CONOUT$", "w", stdout);
        puts("injector: in DllMain");
        do_inject();
    }

    return TRUE;
}

void WINAPI __declspec(dllexport) empty_function_dummy() {}
