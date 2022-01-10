CFLAGS=-fPIC -W -Wall -O2
CC=zig cc
LIBS=

all: libinjector.so

libinjector.so: injector-linux.o rconserver.o
	$(CC) $(LIBS) -shared -o $@ $^

rconserver64.o: rconserver.c
	$(CC) $(CFLAGS) -target x86_64-windows-gnu -o $@ -c $<

rconserver32.o: rconserver.c
	$(CC) $(CFLAGS) -target i386-windows-gnu -o $@ -c $<

libinjector64.dll: injector-windows64.o rconserver64.o
	$(CC) -target x86_64-windows-gnu -shared -lws2_32 -o $@ $^

libinjector32.dll: injector-windows32.o rconserver32.o
	$(CC) -target i386-windows-gnu -shared -lws2_32 -o $@ $^

.PHONY: clean all

injector-linux.o: injector-linux.c injector.h

injector-windows64.o: injector-windows.c injector.h
	$(CC) $(CFLAGS) -target x86_64-windows-gnu -o $@ -c $<

injector-windows32.o: injector-windows.c injector.h
	$(CC) $(CFLAGS) -target i386-windows-gnu -o $@ -c $<

clean:
	rm -f *.o *.so *.dll

test: all
	env LD_PRELOAD=$(shell pwd)/libinjector.so zandronum
