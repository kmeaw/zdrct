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

#ifdef WIN32
extern void* cdocommand_ptr_std;
extern void* cdocommand_ptr_fast;
#else
extern void* cdocommand_ptr;
#endif
extern unsigned long rconserver(void*) __attribute__((stdcall));
extern void printf_callback(void*) __attribute((stdcall));

extern void* console_player;
extern int (*P_GiveArtifact)(void *player, int artifact, void *mo);

#define STACK_SIZE (8 * 1024 * 1024)
typedef char bool;
#ifndef TRUE
# define TRUE 1
# define FALSE 0
#endif

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

static bool patch_zdoom(void *script_error, void *toggle_idmypos) {
    printf("script_error = %p\n", script_error);
    printf("toggle_idmypos = %p\n", toggle_idmypos);

    void* printf_ptr = scan(PAGE_EXECUTE_READ, search_data_ref, script_error);
    printf("Printf = %p\n", printf_ptr);

#ifdef WIN32
    cdocommand_ptr_std = scan(PAGE_EXECUTE_READ, search_data_ref, toggle_idmypos);
    if (cdocommand_ptr_std != NULL) {
        printf("C_DoCommand = stdcall %p\n", cdocommand_ptr_std);
    } else {
        cdocommand_ptr_fast = scan(PAGE_EXECUTE_READ, search_data_ref_fast, toggle_idmypos);
        printf("C_DoCommand = fastcall %p\n", cdocommand_ptr_fast);
    }
#else
    cdocommand_ptr = scan(PAGE_EXECUTE_READ, search_data_ref, toggle_idmypos);
    printf("C_DoCommand = %p\n", cdocommand_ptr);
#endif

    unsigned char *printf_call;
    int32_t *call_offset = NULL;
    printf("Patching Printf...\n");
    for (printf_call = printf_ptr; *printf_call != 0xCC; printf_call++) {
	    if ((printf_call[0] == 0xB9) && (printf_call[-1] & 0xF0) == 0x50) {
		    unsigned char *trampoline = VirtualAlloc(NULL, 54, MEM_COMMIT|MEM_RESERVE, PAGE_EXECUTE_READWRITE);
		    memset(trampoline, 0x90, 42);
		    uint32_t imm32 = 0;
		    trampoline[0] = 0x60; // PUSHA
		    trampoline[1] = 0x68; memcpy(trampoline + 2, &imm32, sizeof(imm32)); // PUSH 0
		    trampoline[6] = 0x68; memcpy(trampoline + 7, &imm32, sizeof(imm32)); // PUSH 0
		    /*
		    imm32 = 0x12345678;
		    trampoline[11] = 0x68; memcpy(trampoline + 12, &imm32, sizeof(imm32)); // PUSH 0x12345678
		    */
		    trampoline[11] = 0x54;
		    imm32 = (uint32_t) (long) printf_callback;
		    trampoline[16] = 0x68; memcpy(trampoline + 17, &imm32, sizeof(imm32)); // PUSH printf_callback
		    imm32 = STACK_SIZE;
		    trampoline[21] = 0x68; memcpy(trampoline + 22, &imm32, sizeof(imm32)); // PUSH STACK_SIZE
		    imm32 = 0;
		    trampoline[26] = 0x68; memcpy(trampoline + 27, &imm32, sizeof(imm32)); // PUSH 0
		    imm32 = ((long) &CreateThread) - ((long) &trampoline[36]);
		    trampoline[31] = 0xE8; memcpy(trampoline + 32, &imm32, sizeof(imm32)); // CALL CreateThread
		    printf("CreateThread = %p\n", &CreateThread);
		    imm32 = INFINITE;
		    trampoline[36] = 0x68; memcpy(trampoline + 37, &imm32, sizeof(imm32)); // PUSH INFINITE
		    trampoline[41] = 0x50; // PUSH handle
		    imm32 = ((long) &WaitForSingleObject) - ((long) &trampoline[47]);
		    trampoline[42] = 0xE8; memcpy(trampoline + 43, &imm32, sizeof(imm32)); // CALL WaitForSingleObject
		    trampoline[47] = 0x61; // POPA
		    memcpy(trampoline + 48, printf_call, 5);
		    trampoline[53] = 0xC3; // RET

		    call_offset = (int32_t *) ((long)trampoline) - ((long)&printf_call[5]);
		    unsigned char call_rel32 = 0xE8;
		    if (!WriteProcessMemory(GetCurrentProcess(), printf_call, &call_rel32, sizeof(call_rel32), NULL)) {
			    printf("WriteProcessMemory has failed: %d\n", GetLastError());
		    }
		    if (!WriteProcessMemory(GetCurrentProcess(), printf_call + 1, &call_offset, sizeof(call_offset), NULL)) {
			    printf("WriteProcessMemory has failed: %d\n", GetLastError());
		    }
	    }
    }
    if (call_offset == NULL) {
	    printf("Could not find CALL inside Printf.\n");
    } else {
	    printf("Call target of Printf has been changed to %p.\n", printf_callback);
    }

    return TRUE;
}

static bool patch_russian_doom(void *you_got_it, void *a_secret_is_revealed) {
	/* Find the language branch
	   MOV DWORD PTR DS:[CCC458],russian-.00B64>;  ASCII "CHOOSE AN ARTIFACT ( A - J )"
	 */

	void* load_english = scan(PAGE_EXECUTE_READ, search_data_store, you_got_it);
	printf("load_english = %p\n", load_english);

	if (load_english == NULL)
		return FALSE;

	void* cheat_func3 = scan(PAGE_EXECUTE_READ, search_data_load_func, load_english);
	printf ("cheat_func3 = %p\n", cheat_func3);

	if (cheat_func3 == NULL)
		return FALSE;

	struct ArgValue av = {
		.func = cheat_func3,
		.arg = 2,
		.value = 0,
	};

	P_GiveArtifact = scan(PAGE_EXECUTE_READ, search_load_arg, &av);
	printf ("P_GiveArtifact = %p\n", P_GiveArtifact);

	void *load_english2 = scan(PAGE_EXECUTE_READ, search_data_store, a_secret_is_revealed);
	printf ("load_english2 = %p\n", load_english2);

	if (load_english2 == NULL)
		return FALSE;

	void* sector9_handler = scan(PAGE_EXECUTE_READ, search_data_load, load_english2);
	printf ("sector9_handler = %p\n", sector9_handler);

	if (sector9_handler == NULL)
		return FALSE;

	console_player = scan(PAGE_EXECUTE_READ, search_mul_add, sector9_handler);
	printf ("console_player = %p\n", console_player);

	return TRUE;
}

static void do_inject () {
    void *script_error = scan(PAGE_READONLY, search_string_cb, "\034GScript error, \"%s\" line %d:");
    void *toggle_idmypos = scan(PAGE_READONLY, search_string_cb, "toggle idmypos");
    void *you_got_it = scan(PAGE_READONLY, search_string_cb, "YOU GOT IT");
    void *a_secret_is_revealed = scan(PAGE_READONLY, search_string_cb, "A SECRET IS REVEALED");
    bool success = FALSE;

    if (script_error != NULL && toggle_idmypos != NULL) {
	    success = patch_zdoom(script_error, toggle_idmypos);
    } else if (you_got_it != NULL && a_secret_is_revealed != NULL) {
	    success = patch_russian_doom(you_got_it, a_secret_is_revealed);
    }

    if (success) {
	    if (CreateThread(NULL, STACK_SIZE, rconserver, NULL, 0, NULL) == INVALID_HANDLE_VALUE) {
		perror("clone");
	    }
    }
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

void WINAPI StartServer(__attribute__((unused)) void *ptr) {
	puts("injector: Hello, World!");
}

void WINAPI __declspec(dllexport) empty_function_dummy() {}
