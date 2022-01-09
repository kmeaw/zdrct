#ifdef WIN32
static void *memmem(const void *haystack, size_t haystack_len,
                    const void * const needle, const size_t needle_len);
#endif

static void* search_string_cb(void *ptr, size_t sz, void *uptr) {
    return memmem(ptr, sz, uptr, strlen((const char *) uptr));
}

static void* search_data_ref(void *ptr, size_t sz, void *uptr) {
#if defined(__x86_64__)
#if defined(WIN32)
    static char prefix[] = { 0xcc, 0xcc, 0xcc };
    static char pattern[] = { 0x48, 0x8d, 0x0d };
#else
    static char prefix[] = { 0x00 };
    static char pattern[] = { 0x48, 0x8d, 0x3d };
#endif
    char *scan = (char *) ptr;
    char *end = scan + sz;
    char *call;
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
        }
    }
    return NULL;
#elif defined(__i386__)
    static char pattern[] = { 0x68, 0x00, 0x00, 0x00, 0x00, 0xe8 };
    memcpy(pattern + 1, &uptr, 4);
    printf("win32: looking for %02x %02x %02x %02x %02x %02x\n", pattern[0], pattern[1], pattern[2], pattern[3], pattern[4], pattern[5]);
    void *m = memmem(ptr, sz, pattern, sizeof(pattern));
    if (m) return m;
    pattern[0] = 0xb9;
    m = memmem(ptr, sz, pattern, sizeof(pattern));
    if (m) return m;
    return NULL;
#else
#error architecture is not supported.
#endif
}
