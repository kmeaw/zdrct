all: zdrct

IMPORT_PATH := github.com/kmeaw/zdrct
VERSION := $(shell git describe --tags --always --match "v[0-9]*")
DATE := $(shell date -u '+%Y-%m-%d-%H%M UTC')
VERSION_FLAGS := -X "main.Version=$(VERSION)" -X "main.BuildTime=$(DATE)"

youtube.api/%.pb.go: %.proto
	protoc --go_out=. --go-grpc_out=. $^

zdrct.exe: $(wildcard *.go) youtube.api/stream_list.pb.go
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o zdrct.exe -trimpath -ldflags='$(VERSION_FLAGS)'

dist.exe: zdrct.exe assets templates
	makensis zdrct.nsi

zdrct: $(wildcard *.go)
	rm -f zdrct
	CGO_ENABLED=1 go build -o $@ -ldflags='$(VERSION_FLAGS)'

clean:
	rm -f zdrct zdrct.exe dist.exe

run: zdrct
	./zdrct

.PHONY: dist clean run
