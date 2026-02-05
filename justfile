default:
    @just --list

build:
    just apps/uploader/build

build-linux:
    just apps/uploader/build-linux

test:
    just apps/uploader/test

test-cover:
    just apps/uploader/test-cover

clean:
    just apps/uploader/clean
