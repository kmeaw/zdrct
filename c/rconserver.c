#include <stdio.h>
#include <string.h>
#ifdef WIN32
#define WIN32_LEAN_AND_MEAN
#include <winsock2.h>
typedef int socklen_t;
#else
#include <sys/socket.h>
#include <netinet/in.h>
#include <errno.h>
#endif

#ifdef WIN32
void
__attribute__((stdcall))
(*cdocommand_ptr_std) (const char *, int);
void
__attribute__((fastcall))
(*cdocommand_ptr_fast) (const char *, int);
#else
void (*cdocommand_ptr) (const char *, int);
#endif


static void cons_perror(const char *prefix) {
    static char errmsg[512 + 12] = "echo ERROR: ";

    if (
#ifdef WIN32
        cdocommand_ptr_std == NULL
        && cdocommand_ptr_fast == NULL
#else
        cdocommand_ptr == NULL
#endif
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
    if (cdocommand_ptr_std != NULL)
        (*cdocommand_ptr_std) (errmsg, 0);
    else if (cdocommand_ptr_fast != NULL)
        (*cdocommand_ptr_fast) (errmsg, 0);
#else
    strcpy(ptr, strerror(errno));
    (*cdocommand_ptr) (errmsg, 0);
#endif
}

#define CLRC_BEGINCONNECTION 52
#define CLRC_COMMAND 54

#define SVRC_LOGGEDIN 35

int s = -1;

static struct sockaddr_in console_receiver;

#ifdef WIN32
unsigned long __attribute__((stdcall))
#else
void
#endif
rconserver(__attribute__((unused)) void* _unused0) {
    static char buf[4096];

#ifdef WIN32
    WSADATA wsaData;
    int iResult;

    // Initialize Winsock
    iResult = WSAStartup(MAKEWORD(2,2), &wsaData);
    if (iResult != 0) {
        printf("WSAStartup failed: %d\n", iResult);
        goto rconend;
    }
#endif

    s = socket(AF_INET, SOCK_DGRAM, 0);
    if (s < 0) {
        cons_perror("socket");
        goto rconend;
    }

    struct sockaddr_in lcl = {
        .sin_family = AF_INET,
        .sin_port = htons(10666),
        .sin_addr.s_addr = htonl(0x7f000001),
    }, rmt;
    socklen_t rmt_sz = sizeof(rmt);

    if (bind(s, (struct sockaddr *) &lcl, sizeof(lcl)) == -1) {
        cons_perror("bind");
        goto rconend;
    }

    puts("rconserver is ready.");

    while (1) {
        int sz = recvfrom(s, buf, sizeof(buf) - 1, 0, (struct sockaddr *) &rmt, &rmt_sz);
        if (sz < 0) {
            cons_perror("recv");
            goto rconend;
        }

        if (sz < 2 || ((unsigned char) buf[0]) != 0xFF) {
            // I am too lazy to implement huffman decoding
            continue;
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
#ifdef WIN32
                cdocommand_ptr_std == NULL
                && cdocommand_ptr_fast == NULL
#else
                cdocommand_ptr == NULL
#endif
            ) {
                fprintf(stderr, "console is not initialized, dropping message: %s\n", buf + 2);
            } else {
#ifdef WIN32
                if (cdocommand_ptr_std != NULL)
                    (*cdocommand_ptr_std) (buf + 2, 0);
                else if (cdocommand_ptr_fast != NULL)
                    (*cdocommand_ptr_fast) (buf + 2, 0);
#else
                (*cdocommand_ptr) (buf + 2, 0);
#endif
            }
            printf("C_DoCommand(%s);\n", buf + 2);
            break;
        }
    }
rconend:
#ifdef WIN32
    return 0;
#else
    ;
#endif
}
