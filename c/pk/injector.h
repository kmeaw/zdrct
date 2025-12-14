#ifdef WIN32
static void *memmem(const void *haystack, size_t haystack_len,
                    const void * const needle, const size_t needle_len);
#endif

static void* search_string_cb(void *ptr, size_t sz, void *uptr) {
    return memmem(ptr, sz, uptr, strlen((const char *) uptr));
}

static void* search_data_ref_ecx(void *ptr, size_t sz, void *uptr) {
    unsigned char *call;
    static unsigned char pattern[] = { 0x68, 0x00, 0x00, 0x00, 0x00, 0x51, 0xff, 0x15 }; /* push imm32; push ecx; call [ptr32]... */
    memcpy(pattern + 1, &uptr, 4);
    void *m = memmem(ptr, sz, pattern, sizeof(pattern));
    if (m == NULL)
        return NULL;
    call = (unsigned char *) m + sizeof(pattern);
    void** *ptr32 = 0;
    memcpy(&ptr32, call, sizeof(int32_t));
    return *ptr32;
}

static void* search_data_ref_ecx_push_ptr32(void *ptr, size_t sz, void *uptr) {
    static unsigned char pattern[] = { 0x68, 0x00, 0x00, 0x00, 0x00, 0x51, 0xff, 0x15 }; /* push imm32; push ecx; call [ptr32]... */
    memcpy(pattern + 1, &uptr, 4);
    void *m = memmem(ptr, sz, pattern, sizeof(pattern));
    if (m == NULL)
        return NULL;
    return ((unsigned char *) m) - 4;
}

static void* search_data_ref_push_push_call(void *ptr, size_t sz, void *uptr) {
    static unsigned char pattern[] = { 0x68, 0x00, 0x00, 0x00, 0x00, 0x68 }; /* push imm32; push imm32... */
    memcpy(pattern + 1, &uptr, 4);
    void *m = memmem(ptr, sz, pattern, sizeof(pattern));
    if (m == NULL)
        return NULL;
    if (((unsigned char *) m)[10] != 0xE8) /* call */
        return NULL;
    return ((unsigned char *) m) + 11;
}
