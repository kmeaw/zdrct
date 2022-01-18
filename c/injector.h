#ifdef WIN32
static void *memmem(const void *haystack, size_t haystack_len,
                    const void * const needle, const size_t needle_len);
#endif

static void* search_string_cb(void *ptr, size_t sz, void *uptr) {
    return memmem(ptr, sz, uptr, strlen((const char *) uptr));
}

static void* search_data_ref(void *ptr, size_t sz, void *uptr) {
    char *call;
#if defined(__x86_64__)
    static char prologue[] = { 0x55, 0x48, 0x89, 0xe5 }; /* push rbp; mov rbp, rsp */
# if defined(WIN32)
    static char prefix[] = { 0xcc, 0xcc, 0xcc }; /* nop; nop; nop */
    static char pattern[] = { 0x48, 0x8d, 0x0d }; /* lea rcx, [rip+off32] */
# else
    static char prefix[] = { 0x00 };
    static char pattern[] = { 0x48, 0x8d, 0x3d }; /* lea rdi, [rip+off32] */
# endif
    char *scan = (char *) ptr;
    char *end = scan + sz;
    while (scan < end - 64) {
        scan = memmem(scan, end - scan, pattern, sizeof(pattern));
        if (scan == NULL) break;
        scan = scan + sizeof(pattern);
        if (scan + (sizeof(int32_t) + * (int32_t *) scan) == uptr) {
            scan += sizeof(uint32_t);
            call = memchr(scan, 0xe8, 64);
            if (call == NULL) continue;
            call++;
            int32_t offset = * (int32_t *) call;
            call += sizeof(int32_t) + offset;
            if (!memcmp(call - sizeof(prefix), prefix, sizeof(prefix)))
                return call;
            if (!memcmp(call, prologue, sizeof(prologue)))
                return call;
        }
    }
    return NULL;
#elif defined(__i386__)
    static char prologue[] = { 0x55, 0x89, 0xe5 }; /* push ebp; mov ebp, esp */
    static char pattern[] = { 0x68, 0x00, 0x00, 0x00, 0x00, 0xe8 }; /* push imm32; call... */
    memcpy(pattern + 1, &uptr, 4);
    void *m = memmem(ptr, sz, pattern, sizeof(pattern));
    if (m == NULL)
        return NULL;
    call = (char *) m + sizeof(pattern);
    int32_t offset = 0;
    memcpy(&offset, call, sizeof(int32_t));
    call += sizeof(int32_t) + offset;
    return call;
#else
# error architecture is not supported.
#endif
}

#if defined(WIN32)
static void* search_data_ref_fast(void *ptr, size_t sz, void *uptr) {
    char *call;
    static char pattern[] = { 0xb9, 0x00, 0x00, 0x00, 0x00, 0xe8 }; /* mov ecx, imm32 */
    memcpy(pattern + 1, &uptr, 4);
    void *m = memmem(ptr, sz, pattern, sizeof(pattern));
    if (m == NULL)
        return NULL;
    call = (char *) m + sizeof(pattern);
    int32_t offset = 0;
    memcpy(&offset, call, sizeof(int32_t));
    call += sizeof(int32_t) + offset;
    return call;
}
#endif
