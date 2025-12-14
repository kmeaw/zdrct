#include <stdio.h>
#include <string.h>
#ifdef WIN32
# define WIN32_LEAN_AND_MEAN
# include <winsock2.h>
typedef int socklen_t;
#else
# include <sys/socket.h>
# include <netinet/in.h>
# include <errno.h>
#endif

void **gscript = NULL;

void
__attribute__((cdecl))
(*cdocommand_ptr) (void *scriptref, const char *);

void*
__attribute__((stdcall))
(*gc_ptr) (void *scriptref, const char *);

static void cons_perror(const char *prefix) {
    static char errmsg[512 + 12] = "echo ERROR: ";

    if (
        cdocommand_ptr == NULL
    ) {
        perror(prefix);
        return;
    }

    strcpy(errmsg + 12, prefix);
    char *ptr = errmsg + 12 + strlen(prefix);
    *ptr++ = ':';
    *ptr++ = ' ';
#ifdef WIN32
    sprintf(ptr, "errno %d", WSAGetLastError());
    if (cdocommand_ptr != NULL)
        (*cdocommand_ptr) (*gscript, errmsg);
#else
    strcpy(ptr, strerror(errno));
    (*cdocommand_ptr) (*gscript, errmsg);
#endif
}

#define CLRC_BEGINCONNECTION 52
#define CLRC_COMMAND 54

#define SVRC_LOGGEDIN 35

int s = -1;

static struct sockaddr_in console_receiver;

void
rconserver_start() {
#ifdef WIN32
    WSADATA wsaData;
    int iResult;
    struct sockaddr_in lcl = {
        .sin_family = AF_INET,
        .sin_port = htons(10666),
        .sin_addr.s_addr = htonl(0x7f000001),
    };

    // Initialize Winsock
    iResult = WSAStartup(MAKEWORD(2,2), &wsaData);
    if (iResult != 0) {
        printf("WSAStartup failed: %d\n", iResult);
	return;
    }
#endif

    s = socket(AF_INET, SOCK_DGRAM, 0);
    if (s < 0) {
        cons_perror("socket");
	return;
    }

    unsigned long on = 1;
    if (
#ifdef WIN32
		    ioctlsocket 
#else
		    ioctl
#endif
		    (s, FIONBIO, &on)) {
        cons_perror("ioctlsocket");
	return;
    }

    if (bind(s, (struct sockaddr *) &lcl, sizeof(lcl)) == -1) {
        cons_perror("bind");
	return;
    }

    puts("rconserver is ready.");
}

__attribute__((naked))
void rconserver_tick() {
    __asm__ __volatile__ (
	    "pusha\n\t"
	    "call _rconserver_tick_c\n\t"
	    "popa\n\t"
	    "jmp *gc_ptr\n\t"
    );
}
void
__attribute__((cdecl))
rconserver_tick_c() {
    static char buf[4096];
    static struct sockaddr_in rmt;
    static socklen_t rmt_sz = sizeof(rmt);

    int sz = recvfrom(s, buf, sizeof(buf) - 1, 0, (struct sockaddr *) &rmt, &rmt_sz);
    if (sz < 2 || ((unsigned char) buf[0]) != 0xFF) {
	// I am too lazy to implement huffman decoding
	return;
    }

    buf[sz] = 0;

    switch (buf[1]) {
    case CLRC_BEGINCONNECTION:
	buf[0] = 0xFF;
	buf[1] = SVRC_LOGGEDIN;
	sz = sendto(s, buf, 2, 0, (struct sockaddr *) &rmt, rmt_sz);
	if (sz != 2) {
	    cons_perror("send");
	}
	memcpy(&console_receiver, &rmt, rmt_sz);
	break;

    case CLRC_COMMAND:
	if (
	    cdocommand_ptr == NULL
	) {
	    fprintf(stderr, "console is not initialized, dropping message: %s\n", buf + 2);
	} else {
	    (*cdocommand_ptr) (*gscript, buf + 2);
	}
	printf("C_DoCommand(%s);\n", buf + 2);
	break;
    }
}
