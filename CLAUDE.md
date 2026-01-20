# CLAUDE.md

S3 Uploader: centralized server with S3 credentials, distributed clients watch directories and upload files.

- **Server** (`cmd/server`): HTTP API, authenticates clients, uploads to S3
- **Client** (`cmd/client`): Watches directories via inotify, queues uploads with stability checks

Go project using `just` for tasks. Pure Go, no CGO.

## Commands

just build && just test
