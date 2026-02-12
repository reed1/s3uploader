package test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"s3uploader/internal/client"
	"s3uploader/internal/server"
)

type testEnv struct {
	tmpDir     string
	watchDir   string
	storageDir string
	dbPath     string
	storage    *server.FakeStorage
	ts         *httptest.Server
	db         *client.DB
	queue      *client.Queue
	uploader   *client.Uploader
	cfg        *client.Config
	processor  *client.Processor
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "s3uploader-e2e-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	watchDir := filepath.Join(tmpDir, "watch")
	storageDir := filepath.Join(tmpDir, "storage")
	dbPath := filepath.Join(tmpDir, "client.db")

	if err := os.MkdirAll(watchDir, 0755); err != nil {
		t.Fatalf("failed to create watch dir: %v", err)
	}
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("failed to create storage dir: %v", err)
	}

	storage := server.NewFakeStorage(storageDir, "backups")
	auth := server.NewAuthMiddleware([]server.ClientEntry{
		{ID: "test-client", APIKey: "test-api-key"},
	})
	handler := server.NewHandler(storage, nil)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, auth)
	ts := httptest.NewServer(mux)

	db, err := client.NewDB(dbPath)
	if err != nil {
		ts.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create db: %v", err)
	}

	queue := client.NewQueue()

	cfg := &client.Config{
		Server:    client.ServerConfig{URL: ts.URL, APIKey: "test-api-key"},
		Watches:   []client.WatchConfig{{LocalPath: watchDir, RemotePrefix: "uploads/"}},
		Stability: client.StabilityConfig{DebounceSeconds: 1, MaxAttempts: 10},
		Upload:    client.UploadConfig{RetryAttempts: 3, RetryDelaySeconds: 1, MaxFileSizeMB: 100},
	}

	return buildTestEnv(t, tmpDir, watchDir, storageDir, dbPath, storage, ts, db, queue, cfg)
}

func newTestEnvWithExcludes(t *testing.T, patterns []string) *testEnv {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "s3uploader-e2e-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	watchDir := filepath.Join(tmpDir, "watch")
	storageDir := filepath.Join(tmpDir, "storage")
	dbPath := filepath.Join(tmpDir, "client.db")

	if err := os.MkdirAll(watchDir, 0755); err != nil {
		t.Fatalf("failed to create watch dir: %v", err)
	}
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("failed to create storage dir: %v", err)
	}

	storage := server.NewFakeStorage(storageDir, "backups")
	auth := server.NewAuthMiddleware([]server.ClientEntry{
		{ID: "test-client", APIKey: "test-api-key"},
	})
	handler := server.NewHandler(storage, nil)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, auth)
	ts := httptest.NewServer(mux)

	db, err := client.NewDB(dbPath)
	if err != nil {
		ts.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create db: %v", err)
	}

	queue := client.NewQueue()

	cfg := &client.Config{
		Server:          client.ServerConfig{URL: ts.URL, APIKey: "test-api-key"},
		Watches:         []client.WatchConfig{{LocalPath: watchDir, RemotePrefix: "uploads/"}},
		Stability:       client.StabilityConfig{DebounceSeconds: 1, MaxAttempts: 10},
		Upload:          client.UploadConfig{RetryAttempts: 3, RetryDelaySeconds: 1, MaxFileSizeMB: 100},
		ExcludePatterns: patterns,
	}
	if err := cfg.CompileExcludePatterns(); err != nil {
		t.Fatalf("failed to compile exclude patterns: %v", err)
	}

	return buildTestEnv(t, tmpDir, watchDir, storageDir, dbPath, storage, ts, db, queue, cfg)
}

func buildTestEnv(t *testing.T, tmpDir, watchDir, storageDir, dbPath string, storage *server.FakeStorage, ts *httptest.Server, db *client.DB, queue *client.Queue, cfg *client.Config) *testEnv {
	t.Helper()

	uploader := client.NewUploader(cfg)
	processor := client.NewProcessor(queue, db, uploader, cfg)

	return &testEnv{
		tmpDir:     tmpDir,
		watchDir:   watchDir,
		storageDir: storageDir,
		dbPath:     dbPath,
		storage:    storage,
		ts:         ts,
		db:         db,
		queue:      queue,
		uploader:   uploader,
		cfg:        cfg,
		processor:  processor,
	}
}

func (e *testEnv) cleanup() {
	e.db.Close()
	e.ts.Close()
	os.RemoveAll(e.tmpDir)
}

func generateRandomFiles(t *testing.T, dir string, count int) map[string]string {
	return generateRandomFilesWithPrefix(t, dir, count, "file_")
}

func generateRandomFilesWithPrefix(t *testing.T, dir string, count int, prefix string) map[string]string {
	t.Helper()
	files := make(map[string]string)

	for i := 0; i < count; i++ {
		filename := fmt.Sprintf("%s%d.bin", prefix, i)
		path := filepath.Join(dir, filename)

		size := 1024 + (i * 512)
		data := make([]byte, size)
		if _, err := rand.Read(data); err != nil {
			t.Fatalf("failed to generate random data: %v", err)
		}

		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", path, err)
		}

		hash := sha256.Sum256(data)
		files[path] = hex.EncodeToString(hash[:])

		time.Sleep(100 * time.Millisecond)
	}

	return files
}

func generateFilesWithSuffix(t *testing.T, dir string, count int, suffix string) map[string]string {
	t.Helper()
	files := make(map[string]string)

	for i := 0; i < count; i++ {
		filename := fmt.Sprintf("file_%d%s", i, suffix)
		path := filepath.Join(dir, filename)

		size := 1024 + (i * 512)
		data := make([]byte, size)
		if _, err := rand.Read(data); err != nil {
			t.Fatalf("failed to generate random data: %v", err)
		}

		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", path, err)
		}

		hash := sha256.Sum256(data)
		files[path] = hex.EncodeToString(hash[:])

		time.Sleep(100 * time.Millisecond)
	}

	return files
}

func hashFile(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open file %s: %v", path, err)
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		t.Fatalf("failed to hash file %s: %v", path, err)
	}

	return hex.EncodeToString(h.Sum(nil))
}

func waitForUploads(t *testing.T, db *client.DB, files map[string]string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		allDone := true
		for path := range files {
			rec, err := db.GetFile(path)
			if err != nil {
				t.Fatalf("db error: %v", err)
			}
			if rec == nil || rec.UploadedAt == nil {
				allDone = false
				break
			}
		}
		if allDone {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for uploads")
}
