# CLAUDE.md

S3 Uploader: centralized server with S3 credentials, distributed clients watch directories and upload files.

Monorepo structure:
- **apps/uploader** — Go: server (`cmd/server`), client (`cmd/client`), shared code (`internal/`)
- **apps/dashboard** — Next.js dashboard (planned)
- **docs/** — documentation
- **ansible/** — deployment (planned)

Go project using `just` for tasks. Pure Go, no CGO.

## Commands

just build && just test
