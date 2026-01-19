# Future Features

Features deferred from the initial implementation.

## Daily Reports

- Client sends daily upload statistics to server
- Server tracks client health based on report frequency
- New endpoint: `POST /report`
- New client table: `daily_stats`
- Admin endpoint: `GET /clients` (list clients with health status)

## Auto-Cleanup

- Automatically delete local files after they've been uploaded for N days
- Verify file exists on S3 before deleting locally
- New column: `locally_deleted` flag in files table
- Configurable: `cleanup.enabled`, `cleanup.after_days`, `cleanup.time`

## Periodic Re-scan

- Periodic timer to re-scan watched directories
- Catches files missed by inotify (e.g., during network issues)
- Configurable: `scan.interval_minutes`

## Download Command

- `s3up get <remote_path>` to download files from S3
- Outputs to stdout for piping flexibility

## File Extension Restrictions

- Per-client whitelist of allowed file extensions
- Server rejects uploads that don't match

## Concurrent Uploads

- Upload multiple files in parallel
- Configurable: `upload.concurrent`
