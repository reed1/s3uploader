.PHONY: build build-linux clean

build:
	go build -o dist/s3up-server ./cmd/server
	go build -o dist/s3up ./cmd/client

build-linux:
	GOOS=linux GOARCH=amd64 go build -o dist/s3up-server-linux ./cmd/server
	GOOS=linux GOARCH=amd64 go build -o dist/s3up-linux ./cmd/client

clean:
	rm -rf dist/
