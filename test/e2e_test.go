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

	"github.com/reed/s3uploader/internal/client"
	"github.com/reed/s3uploader/internal/server"
)

func TestE2E_UploadAndVerify(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "s3uploader-e2e-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
		{Name: "test-client", APIKey: "test-api-key"},
	})
	handler := server.NewHandler(storage)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, auth)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	db, err := client.NewDB(dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	queue := client.NewQueue()
	uploader := client.NewUploader(ts.URL, "test-api-key")

	watches := []client.WatchConfig{
		{LocalPath: watchDir, RemotePrefix: "uploads/", Recursive: true},
	}

	watcher, err := client.NewWatcher(watches, queue)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	if err := watcher.Start(); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	cfg := &client.Config{
		Stability: client.StabilityConfig{DebounceSeconds: 1, MaxAttempts: 10},
		Upload:    client.UploadConfig{RetryAttempts: 3, RetryDelaySeconds: 1, MaxFileSizeMB: 100},
	}

	stopProcessor := make(chan struct{})
	go processQueueForTest(queue, db, uploader, cfg, stopProcessor)
	defer close(stopProcessor)

	testFiles := generateRandomFiles(t, watchDir, 5)

	waitForUploads(t, db, testFiles, 30*time.Second)

	for localPath, expectedHash := range testFiles {
		relPath, _ := filepath.Rel(watchDir, localPath)
		remotePath := filepath.Join("uploads", relPath)
		storagePath := storage.GetFilePath("test-client", remotePath)

		actualHash := hashFile(t, storagePath)
		if actualHash != expectedHash {
			t.Errorf("hash mismatch for %s: expected %s, got %s", localPath, expectedHash, actualHash)
		}
	}

	t.Logf("all %d files uploaded and verified successfully", len(testFiles))
}

func generateRandomFiles(t *testing.T, dir string, count int) map[string]string {
	t.Helper()
	files := make(map[string]string)

	for i := 0; i < count; i++ {
		filename := fmt.Sprintf("file_%d.bin", i)
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

func processQueueForTest(queue *client.Queue, db *client.DB, uploader *client.Uploader, cfg *client.Config, stop <-chan struct{}) {
	maxSizeBytes := int64(cfg.Upload.MaxFileSizeMB) * 1024 * 1024
	debounce := time.Duration(cfg.Stability.DebounceSeconds) * time.Second

	for {
		select {
		case <-stop:
			return
		default:
		}

		entry, ok := queue.Dequeue()
		if !ok {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		processEntryForTest(entry, queue, db, uploader, maxSizeBytes, debounce, cfg)
	}
}

func processEntryForTest(entry client.QueueEntry, queue *client.Queue, db *client.DB, uploader *client.Uploader, maxSizeBytes int64, debounce time.Duration, cfg *client.Config) {
	info, err := os.Stat(entry.LocalPath)
	if err != nil {
		return
	}

	rec, err := db.GetFile(entry.LocalPath)
	if err != nil {
		return
	}

	currentMtime := info.ModTime().UTC().Unix()
	if rec != nil && rec.Mtime == currentMtime {
		return
	}

	if info.Size() > maxSizeBytes {
		reason := "file_too_large"
		if rec == nil {
			db.InsertFile(entry.LocalPath, entry.RemotePath, info.Size(), currentMtime, &reason)
		} else {
			db.UpdateFile(entry.LocalPath, entry.RemotePath, info.Size(), currentMtime, &reason)
		}
		return
	}

	time.Sleep(debounce)

	info2, err := os.Stat(entry.LocalPath)
	if err != nil {
		return
	}

	mtime2 := info2.ModTime().UTC().Unix()
	if mtime2 != currentMtime || info2.Size() != info.Size() {
		entry.AttemptCount++
		if entry.AttemptCount >= cfg.Stability.MaxAttempts {
			return
		}
		queue.EnqueueWithAttempts(entry.LocalPath, entry.RemotePath, entry.AttemptCount)
		return
	}

	var lastErr error
	for attempt := 0; attempt < cfg.Upload.RetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(cfg.Upload.RetryDelaySeconds) * time.Second)
		}

		_, lastErr = uploader.Upload(entry.LocalPath, entry.RemotePath)
		if lastErr == nil {
			break
		}
	}

	if lastErr != nil {
		return
	}

	info3, err := os.Stat(entry.LocalPath)
	if err != nil {
		return
	}

	mtime3 := info3.ModTime().UTC().Unix()
	if mtime3 != currentMtime {
		queue.Enqueue(entry.LocalPath, entry.RemotePath)
		return
	}

	if rec == nil {
		db.InsertFile(entry.LocalPath, entry.RemotePath, info.Size(), currentMtime, nil)
	} else {
		db.UpdateFile(entry.LocalPath, entry.RemotePath, info.Size(), currentMtime, nil)
	}
}
