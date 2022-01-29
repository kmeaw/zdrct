all: zdrct

zdrct.exe: $(wildcard *.go)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o zdrct.exe -trimpath

dist.exe: zdrct.exe assets templates
	makensis zdrct.nsi

zdrct: $(wildcard *.go)
	rm -f zdrct
	CGO_ENABLED=0 go build

clean:
	rm -f zdrct zdrct.exe dist.exe

run: zdrct
	./zdrct

.PHONY: dist clean run
