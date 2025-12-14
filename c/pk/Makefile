CFLAGS=-fPIC -W -Wall -O2
CC=zig cc
LIBS=

all: libinjectorpk32.dll

rconserver32.o: rconserver.c
	$(CC) $(CFLAGS) -target x86-windows-gnu -o $@ -c $<

libinjectorpk32.dll: injector-windows32.o rconserver32.o
	$(CC) -target x86-windows-gnu -shared -lws2_32 -o $@ $^

injector-windows32.o: injector-windows.c injector.h
	$(CC) $(CFLAGS) -target x86-windows-gnu -o $@ -c $<

.PHONY: clean all

clean:
	rm -f *.dll *.pdb *.lib *.o
