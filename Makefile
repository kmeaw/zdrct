all: zdrct

youtube.api/%.pb.go: %.proto
	protoc --go_out=. --go-grpc_out=. $^

zdrct.exe: $(wildcard *.go) youtube.api/stream_list.pb.go
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o zdrct.exe -trimpath

dist.exe: zdrct.exe assets templates
	makensis zdrct.nsi

zdrct: $(wildcard *.go)
	rm -f zdrct
	CGO_ENABLED=1 go build

clean:
	rm -f zdrct zdrct.exe dist.exe

run: zdrct
	./zdrct

.PHONY: dist clean run
