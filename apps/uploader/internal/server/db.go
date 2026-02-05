package server

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

type UploadRecord struct {
	ID         int64
	ClientID   string
	RemotePath string
	FileSize   int64
	UploadedAt int64
}

func NewDB(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	if err := initServerSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db: db}, nil
}

func initServerSchema(db *sql.DB) error {
	schema := `
		CREATE TABLE IF NOT EXISTS uploads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			client_id TEXT NOT NULL,
			remote_path TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			uploaded_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_uploads_client_id ON uploads(client_id);
		CREATE INDEX IF NOT EXISTS idx_uploads_remote_path ON uploads(remote_path);
	`
	_, err := db.Exec(schema)
	return err
}

func (d *DB) InsertUpload(clientID, remotePath string, fileSize int64) error {
	_, err := d.db.Exec(`
		INSERT INTO uploads (client_id, remote_path, file_size, uploaded_at)
		VALUES (?, ?, ?, ?)
	`, clientID, remotePath, fileSize, time.Now().UTC().Unix())
	return err
}

func (d *DB) Close() error {
	return d.db.Close()
}
