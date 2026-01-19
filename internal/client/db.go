package client

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

type FileRecord struct {
	ID         int64
	LocalPath  string
	RemotePath string
	FileSize   int64
	Mtime      int64
	UploadedAt *int64
	SkipReason *string
}

func NewDB(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db: db}, nil
}

func initSchema(db *sql.DB) error {
	schema := `
		CREATE TABLE IF NOT EXISTS files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			local_path TEXT UNIQUE NOT NULL,
			remote_path TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			mtime INTEGER NOT NULL,
			uploaded_at INTEGER,
			skip_reason TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_files_local_path ON files(local_path);
	`
	_, err := db.Exec(schema)
	return err
}

func (d *DB) GetFile(localPath string) (*FileRecord, error) {
	row := d.db.QueryRow(`
		SELECT id, local_path, remote_path, file_size, mtime, uploaded_at, skip_reason
		FROM files WHERE local_path = ?
	`, localPath)

	var rec FileRecord
	err := row.Scan(&rec.ID, &rec.LocalPath, &rec.RemotePath, &rec.FileSize, &rec.Mtime, &rec.UploadedAt, &rec.SkipReason)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (d *DB) InsertFile(localPath, remotePath string, fileSize, mtime int64, skipReason *string) error {
	var uploadedAt *int64
	if skipReason == nil {
		now := time.Now().UTC().Unix()
		uploadedAt = &now
	}

	_, err := d.db.Exec(`
		INSERT INTO files (local_path, remote_path, file_size, mtime, uploaded_at, skip_reason)
		VALUES (?, ?, ?, ?, ?, ?)
	`, localPath, remotePath, fileSize, mtime, uploadedAt, skipReason)
	return err
}

func (d *DB) UpdateFile(localPath, remotePath string, fileSize, mtime int64, skipReason *string) error {
	var uploadedAt *int64
	if skipReason == nil {
		now := time.Now().UTC().Unix()
		uploadedAt = &now
	}

	_, err := d.db.Exec(`
		UPDATE files SET remote_path = ?, file_size = ?, mtime = ?, uploaded_at = ?, skip_reason = ?
		WHERE local_path = ?
	`, remotePath, fileSize, mtime, uploadedAt, skipReason, localPath)
	return err
}

func (d *DB) Close() error {
	return d.db.Close()
}
