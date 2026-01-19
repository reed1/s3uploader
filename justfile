default:
    @just --list

build:
    go build -o dist/s3up-server ./cmd/server
    go build -o dist/s3up ./cmd/client

build-linux:
    GOOS=linux GOARCH=amd64 go build -o dist/s3up-server-linux ./cmd/server
    GOOS=linux GOARCH=amd64 go build -o dist/s3up-linux ./cmd/client

test:
    go test -v ./test/...

clean:
    rm -rf dist/
