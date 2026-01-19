# S3 Uploader - Design Document

A secure file backup system with a centralized server holding S3 credentials and distributed clients on web application deployments.

**All timestamps are UTC.**

## Architecture Overview

```
┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐
│   Web App 1      │     │   Web App 2      │     │   Web App 3      │
│   + Client       │     │   + Client       │     │   + Client       │
│   (API Key A)    │     │   (API Key B)    │     │   (API Key C)    │
└────────┬─────────┘     └────────┬─────────┘     └────────┬─────────┘
         │                        │                        │
         │      uploads + daily reports                    │
         └────────────────────────┼────────────────────────┘
                                  │
                                  ▼
                    ┌─────────────────────────────┐
                    │          Server             │
                    │   (S3 credentials here)     │
                    │                             │
                    │  - Authenticates clients    │
                    │  - Enforces limits          │
                    │  - Routes to S3 buckets     │
                    │  - Tracks client activity   │
                    │  - SQLite for reports       │
                    └──────────────┬──────────────┘
                                   │
                                   ▼
                    ┌─────────────────────────────┐
                    │            S3               │
                    │  {path_prefix}/{client}/... │
                    └─────────────────────────────┘
```

## S3 Path Structure

Server has ONE global `path_prefix`. Each client's files are stored under:

```
{path_prefix}/{client_name}/{remote_path}
```

Example:
- Server `path_prefix`: `backups/`
- Client name: `webapp-prod`
- File remote path: `uploads/users/123/avatar.png`
- Final S3 key: `backups/webapp-prod/uploads/users/123/avatar.png`

---

## Authentication

Simple API Key:
- Server generates a random API key per client (32+ chars)
- Client sends key in `Authorization: Bearer <key>` header
- Server validates key against its config
- Secure with TLS (HTTPS)

---

## Server

### Configuration (`server.yaml`)

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  tls:
    enabled: true
    cert_file: "/path/to/cert.pem"
    key_file: "/path/to/key.pem"

s3:
  endpoint: ""  # Leave empty for AWS, set for MinIO/etc
  region: "us-east-1"
  bucket: "mycompany-backups"
  path_prefix: "backups/"  # Global prefix, client name appended automatically
  access_key_id: "${S3_ACCESS_KEY_ID}"
  secret_access_key: "${S3_SECRET_ACCESS_KEY}"

clients:
  - name: "webapp-prod"
    api_key: "sk_live_abc123..."
    max_file_size_mb: 100

  - name: "webapp-staging"
    api_key: "sk_test_xyz789..."
    max_file_size_mb: 50
```

### API Endpoints

#### `POST /upload`
Upload a single file.

**Headers:**
```
Authorization: Bearer <api_key>
Content-Type: multipart/form-data
```

**Form fields:**
- `file`: The file content
- `path`: Relative path (e.g., `uploads/users/123/avatar.png`)

**Response:**
```json
{
  "success": true,
  "s3_key": "backups/webapp-prod/uploads/users/123/avatar.png",
  "size": 102400
}
```

#### `GET /exists`
Check if a file exists on S3 (used by client before auto-delete).

**Headers:**
```
Authorization: Bearer <api_key>
```

**Query params:**
- `path`: Relative path (e.g., `uploads/users/123/avatar.png`)

**Response:**
```json
{
  "exists": true
}
```

#### `GET /download`
Download a file from S3.

**Headers:**
```
Authorization: Bearer <api_key>
```

**Query params:**
- `path`: Relative path (e.g., `uploads/users/123/avatar.png`)

**Response:**
- File content with appropriate Content-Type
- Or 404 if not found

#### `GET /health`
Health check endpoint (no auth required).

---

## Client

### Configuration (`client.yaml`)

```yaml
server:
  url: "https://backup.mycompany.com:8080"
  api_key: "sk_live_abc123..."

database:
  path: "/var/lib/s3uploader/client.db"

watches:
  - local_path: "/var/www/webapp/uploads"
    remote_prefix: "uploads/"
    recursive: true

  - local_path: "/var/www/webapp/documents"
    remote_prefix: "documents/"
    recursive: true

scan:
  upload_existing: false       # If false, skip existing files on first run

stability:
  debounce_seconds: 3          # Wait time between mtime/size checks
  max_attempts: 100            # Max stability check attempts before giving up on a file

upload:
  retry_attempts: 3
  retry_delay_seconds: 5
```

### Client SQLite Schema

```sql
CREATE TABLE files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    local_path TEXT UNIQUE NOT NULL,
    remote_path TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    mtime INTEGER NOT NULL,          -- File's mtime when uploaded (Unix seconds)
    uploaded_at INTEGER NOT NULL     -- When last uploaded (Unix seconds)
);

CREATE INDEX idx_files_local_path ON files(local_path);
```

**File tracking logic:**
- Row exists → file has been uploaded to S3
- Row exists but file's current mtime differs → needs re-upload

**Upload decision (on scan or inotify):**
1. Query DB by `local_path`
2. If not found → upload, insert new row
3. If found → compare file's current mtime with DB `mtime`
   - If different → re-upload, update `mtime` and `uploaded_at`
   - If same → skip (already uploaded)

### Client Commands

```bash
# Run the daemon
s3up --config /etc/s3uploader/client.yaml
```

### Daemon Behavior

#### Race Condition Mitigation

When `upload_existing: true`, there's a potential race condition where:
- Initial scan finds a file and queues it for upload
- Inotify watcher detects the same file (or file being written) and queues it again
- File gets uploaded twice

**Solution: Upload Queue with Stability Check**

The client uses an in-memory upload queue with the following rules:

1. **Inotify events are always queued** - never processed immediately during startup
2. **Initial scan phase** - queues existing files for upload
3. **Queue draining phase** - after initial scan, process the queue sequentially
4. **Deduplication** - before uploading, check if file already exists in DB with same mtime (skip if unchanged)

**Stability Check (detects files being written):**

Linux file locks are advisory and most applications don't use them. Instead, detect active writes by checking if mtime/size are stable:

1. Stat the file (get mtime and size)
2. Wait `debounce_seconds` (default: 3 seconds)
3. Stat the file again
4. If mtime or size changed → file is unstable, push to back of queue
5. If mtime and size are the same → file is stable, proceed with upload
6. Track attempt count per file; if `max_attempts` exceeded, log warning and skip file

**Post-Upload Verification:**

After uploading, verify the file wasn't modified during the upload:

1. Stat the file again after upload completes
2. If mtime changed since the stability check → file was modified during upload, re-queue it
3. If mtime is the same → upload is valid, record in DB

**On Startup:**
1. Load config
2. Initialize SQLite database
3. Initialize in-memory upload queue
4. Start inotify watcher (events pushed to queue, not processed yet)
5. Scan all watched directories:
   - If `upload_existing: true`: push all files to queue
   - If `upload_existing: false`: skip all existing files (don't upload, don't record)
6. Drain upload queue (process one-by-one with stability check)
7. Switch to normal mode (continue processing queue as events arrive)

**On New File (inotify):**
1. Push to upload queue (no debounce here; stability check handles it)

**Queue Processing (runs continuously after startup):**
1. Pop entry from queue
2. Query DB by `local_path`
   - If in DB and mtime matches: skip (already uploaded)
3. Run stability check:
   - Stat file, wait `debounce_seconds`, stat again
   - If unstable: increment attempt count, push to back of queue, continue
   - If max_attempts exceeded: log warning, remove from queue, continue
4. Upload to server
5. Post-upload verification:
   - Stat file again
   - If mtime changed: re-queue for another upload
6. On success:
   - If new file: insert row with `uploaded_at = now()`, store mtime and file_size
   - If re-upload (mtime changed): update `mtime`, `file_size`, `uploaded_at = now()`
7. On failure:
   - Log error
   - Retry based on config (re-queue with retry count)

---

## Security Considerations

1. **TLS Required**: All client-server communication over HTTPS
2. **Path Validation**: Server validates file paths (no `../` traversal)
3. **Size Limits**: Enforced per-client on server side

---

## Tech Stack

- **Language**: Go 1.21+
- **Client**: Single binary
- **Server**: Single binary
- **Database**: SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- **S3 SDK**: `aws-sdk-go-v2`
- **Config**: YAML (`gopkg.in/yaml.v3`)
- **Watcher**: `fsnotify` (cross-platform, uses inotify on Linux)

---

## Project Structure

```
s3uploader/
├── cmd/
│   ├── server/
│   │   └── main.go
│   └── client/
│       └── main.go
├── internal/
│   ├── server/
│   │   ├── config.go
│   │   ├── handler.go
│   │   ├── auth.go
│   │   └── s3.go
│   └── client/
│       ├── config.go
│       ├── db.go
│       ├── scanner.go
│       ├── watcher.go
│       ├── uploader.go
│       └── queue.go
├── docs/
│   └── future-features.md
├── go.mod
├── go.sum
├── Makefile
└── DESIGN.md
```

---

## Build & Distribution

```makefile
build:
	go build -o dist/s3up-server ./cmd/server
	go build -o dist/s3up ./cmd/client

# Cross-compile for Linux (from any platform)
build-linux:
	GOOS=linux GOARCH=amd64 go build -o dist/s3up-server-linux ./cmd/server
	GOOS=linux GOARCH=amd64 go build -o dist/s3up-linux ./cmd/client
```

No CGO required - pure Go builds work on any platform.
