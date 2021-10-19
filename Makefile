all: zdrct

dist:
	rm -f zdrct.exe
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o zdrct.exe -trimpath
	rice append --exec zdrct.exe

zdrct: $(wildcard *.go) assets templates
	rm -f zdrct
	CGO_ENABLED=0 go build
	rice append --exec zdrct

clean:
	rm -f zdrct zdrct.exe

run: zdrct
	./zdrct

.PHONY: dist clean rnu
