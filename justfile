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

test-system:
    just apps/uploader/test-system

clean:
    just apps/uploader/clean

setup-server:
    just ansible/setup-server

setup-client client_id:
    just ansible/setup-client {{client_id}}
