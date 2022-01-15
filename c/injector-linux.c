#define _GNU_SOURCE
#include <sys/mman.h>
#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <fcntl.h>
#include <string.h>
#include <unistd.h>
#include <sched.h>

#include "injector.h"

static int memfd;

extern void* cdocommand_ptr;
extern int rconserver(void*);

#define STACK_SIZE (1024 * 1024)

static void* scan(const char *perm_filter, void*(*cb)(void *, size_t, void *), void *uptr) {
    void *ptr = NULL;
    FILE *file = fopen("/proc/self/maps", "r");
    if (file == NULL) {
        perror("fopen");
        return NULL;
    }

    static char line[4096];
    while (fgets(line, sizeof(line), file) != NULL) {
        char *perm = strchr(line, ' ');
        if (!perm) break;
        *perm++ = 0;
        char *offs = strchr(perm, ' ');
        if (!offs) break;
        *offs++ = 0;
        char *dev = strchr(offs, ' ');
        if (!dev) break;
        *dev++ = 0;
        char *rest = strchr(dev, ' ');
        if (!rest) break;
        *rest++ = 0;

        if (!strcmp(dev, "00:00")) break;

        if (strcmp(perm, perm_filter)) continue;
        off_t map_from, map_to;
        if (sscanf(line, "%lx-%lx", &map_from, &map_to) != 2) continue;
        ptr = cb((void *) map_from, (size_t) (map_to - map_from), uptr);
        if (ptr != NULL) break;
    }
    fclose(file);

    return ptr;
}

void __attribute__((constructor)) do_inject () {
    memfd = open("/proc/self/mem", O_RDWR);
    if (!memfd) {
        perror("open");
        return;
    }

    void *script_error = scan("r--p", search_string_cb, "\034GScript error, \"%s\" line %d:");
    void *toggle_idmypos = scan("r--p", search_string_cb, "toggle idmypos");

    if (script_error == NULL || toggle_idmypos == NULL)
        return;

    printf("script_error = %p\n", script_error);
    printf("toggle_idmypos = %p\n", toggle_idmypos);

    void* printf_ptr = scan("r-xp", search_data_ref, script_error);
    cdocommand_ptr = scan("r-xp", search_data_ref, toggle_idmypos);
    printf("Printf = %p\n", printf_ptr);
    printf("C_DoCommand = %p\n", cdocommand_ptr);

    if (cdocommand_ptr == NULL)
        goto fail;

    char *stack = mmap(NULL, STACK_SIZE, PROT_READ | PROT_WRITE,
                       MAP_PRIVATE | MAP_ANONYMOUS | MAP_STACK, -1, 0);
    if (!stack) {
        perror("mmap");
        return;
    }

    char *stackTop = stack + STACK_SIZE;
    pid_t pid = clone(rconserver, stackTop, CLONE_THREAD | CLONE_SIGHAND | CLONE_VM, NULL);
    if (pid == -1) {
        perror("clone");
    }

    printf("Running thread %d.\n", pid);
    unsetenv("LD_PRELOAD");

fail:
    close(memfd);
}
