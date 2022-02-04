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

typedef struct ArgValue {
	void *func;
	int arg;
	long value;
} ArgValue;

#if defined(WIN32)
static void* search_data_ref_fast(void *ptr, size_t sz, void *uptr) {
    unsigned char *call;
    static unsigned char pattern[] = { 0xb9, 0x00, 0x00, 0x00, 0x00, 0xe8 }; /* mov ecx, imm32 */
    memcpy(pattern + 1, &uptr, 4);
    void *m = memmem(ptr, sz, pattern, sizeof(pattern));
    if (m == NULL)
        return NULL;
    call = (unsigned char *) m + sizeof(pattern);
    int32_t offset = 0;
    memcpy(&offset, call, sizeof(int32_t));
    call += sizeof(int32_t) + offset;
    return call;
}

static void* search_data_store(void *ptr, size_t sz, void *uptr) {
    static unsigned char pattern[] = { 0xc7, 0x05 }; /* mov ds:[imm32], imm32 */
    unsigned char *scan = (unsigned char *) ptr;
    while (scan < ((unsigned char *) ptr) + sz) {
	    scan = (unsigned char *) memmem(scan, sz - (scan - (unsigned char *) ptr), pattern, sizeof(pattern));
	    if (scan == NULL)
		return NULL;
	    if (!memcmp (scan + 6, &uptr, sizeof(ptr)))
		    break;
	    scan++;
    }

    return * (void **) (scan + 2);
}

static void* search_data_load_func(void *ptr, size_t sz, void *uptr) {
    static unsigned char pattern[] = { 0xA1, 0x00, 0x00, 0x00, 0x00 }; /* mov eax, ds:[imm32] */
    memcpy (pattern + 1, &uptr, sizeof(uptr));
    void *m = memmem(ptr, sz, pattern, sizeof(pattern));
    if (m == NULL)
	    return NULL;

    unsigned char *mfunc = (unsigned char *) (((long) m) & ~0xF);
    int n = 16;
    while (n > 0) {
	    if (mfunc[0] != 0x55 && (mfunc[-1] != 0x90 && mfunc[-1] != 0xC3)) { /* find the prologue */
		    n--;
		    mfunc -= 0x10;
	    } else {
		    return mfunc;
	    }
    }

    return NULL;
}

static void* search_data_load(void *ptr, size_t sz, void *uptr) {
    static unsigned char pattern[] = { 0xA1, 0x00, 0x00, 0x00, 0x00 }; /* mov eax, ds:[imm32] */
    memcpy (pattern + 1, &uptr, sizeof(uptr));
    void *m = memmem(ptr, sz, pattern, sizeof(pattern));
    if (m == NULL)
	    return NULL;

    unsigned char *mfunc = (unsigned char *) (((long) m) & ~0xF);
    int n = 32;
    while (n > 0) {
	    if (mfunc[0] != 0x55 && (mfunc[-1] != 0x90 && mfunc[-1] != 0xC3)) { /* find the prologue */
		    n--;
		    mfunc -= 0x10;
	    } else {
		    return m;
	    }
    }

    return NULL;
}

static void* search_load_arg(void *ptr, size_t sz, void *uptr) {
	static unsigned char pattern[] = { 0xC7, 0x44, 0x24, 0x00, 0x00, 0x00, 0x00, 0x00 }; /* mov ss:[esp+disp8], imm32 */
	ArgValue *av = (ArgValue *) uptr;
	if ((unsigned long) av->func < (unsigned long) ptr || (unsigned long) av->func > ((unsigned long) ptr) + sz)
		return NULL;

	pattern[3] = av->arg * sizeof(long);
	memcpy (pattern + 4, &av->value, sizeof(av->value));
	unsigned char *m = (unsigned char *) memmem(av->func, sz - (((unsigned long) av->func) - ((unsigned long) ptr)), pattern, sizeof(pattern));
	if (m == NULL)
		return NULL;
	unsigned char *call = memchr(m, 0xe8, 64);
	if (call == NULL)
		return NULL;
	call++;
	int32_t offset = * (int32_t *) call;
	call += sizeof(int32_t) + offset;
	if ((((long) call) & 0xF) == 0)
		return (void *) call;

	return NULL;
}

static void* search_mul_add(void *ptr, size_t sz, void *uptr) {
	static unsigned char pattern[] = {
		/* 00 */ 0x89, 0x44, 0x24, 0x04, /* mov ss:[esp+4], eax */
		/* 04 */ 0x69, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, /* imul eax, ds:[imm32], imm32 */
		/* 0e */ 0x05, 0x00, 0x00, 0x00, 0x00, /* add eax, imm32 */
		/* 13 */ 0x89, 0x04, 0x24, /* mov ss:[esp], eax */
		/* 16 */ 0xE8, 0x00, 0x00, 0x00, 0x00, /* call rel32 */
	};
	if ((unsigned long) uptr < (unsigned long) ptr || (unsigned long) uptr > ((unsigned long) ptr) + sz)
		return NULL;
	unsigned char *m = (unsigned char *) memmem(uptr, 64, pattern, 6);
	printf ("DEBUG: found mul+add pattern at %p (uptr = %p).\n", m, uptr);
	if (m == NULL)
		return NULL;
	if (m[0x0e] != 0x05)
		return NULL;
	if (memcmp (m + 0x13, pattern + 0x13, 4))
		return NULL;

	return * (void **) (m + 0x0F);
}
#endif
