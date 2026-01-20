package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/reed/s3uploader/internal/client"
)

func TestE2E_UploadExisting_False(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	preExistingFiles := generateRandomFiles(t, env.watchDir, 3)

	scanner := client.NewScanner(env.watches, env.queue, false)
	if err := scanner.Scan(); err != nil {
		t.Fatalf("failed to scan: %v", err)
	}

	watcher, err := client.NewWatcher(env.watches, env.queue)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	if err := watcher.Start(); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	stopProcessor := make(chan struct{})
	go processQueueForTest(env.queue, env.db, env.uploader, env.cfg, stopProcessor)
	defer close(stopProcessor)

	time.Sleep(500 * time.Millisecond)
	newFiles := generateRandomFilesWithPrefix(t, env.watchDir, 2, "new_")

	waitForUploads(t, env.db, newFiles, 30*time.Second)

	for localPath := range newFiles {
		relPath, _ := filepath.Rel(env.watchDir, localPath)
		remotePath := filepath.Join("uploads", relPath)
		storagePath := env.storage.GetFilePath("test-client", remotePath)

		if _, err := os.Stat(storagePath); err != nil {
			t.Errorf("new file %s should have been uploaded but wasn't", localPath)
		}
	}

	for localPath := range preExistingFiles {
		relPath, _ := filepath.Rel(env.watchDir, localPath)
		remotePath := filepath.Join("uploads", relPath)
		storagePath := env.storage.GetFilePath("test-client", remotePath)

		if _, err := os.Stat(storagePath); err == nil {
			t.Errorf("pre-existing file %s should NOT have been uploaded but was", localPath)
		}
	}

	t.Logf("upload_existing=false: %d new files uploaded, %d pre-existing files correctly skipped",
		len(newFiles), len(preExistingFiles))
}

func TestE2E_UploadExisting_True(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	preExistingFiles := generateRandomFiles(t, env.watchDir, 3)

	scanner := client.NewScanner(env.watches, env.queue, true)
	if err := scanner.Scan(); err != nil {
		t.Fatalf("failed to scan: %v", err)
	}

	watcher, err := client.NewWatcher(env.watches, env.queue)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	if err := watcher.Start(); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	stopProcessor := make(chan struct{})
	go processQueueForTest(env.queue, env.db, env.uploader, env.cfg, stopProcessor)
	defer close(stopProcessor)

	time.Sleep(500 * time.Millisecond)
	newFiles := generateRandomFilesWithPrefix(t, env.watchDir, 2, "new_")

	allFiles := make(map[string]string)
	for k, v := range preExistingFiles {
		allFiles[k] = v
	}
	for k, v := range newFiles {
		allFiles[k] = v
	}

	waitForUploads(t, env.db, allFiles, 30*time.Second)

	for localPath, expectedHash := range allFiles {
		relPath, _ := filepath.Rel(env.watchDir, localPath)
		remotePath := filepath.Join("uploads", relPath)
		storagePath := env.storage.GetFilePath("test-client", remotePath)

		actualHash := hashFile(t, storagePath)
		if actualHash != expectedHash {
			t.Errorf("hash mismatch for %s: expected %s, got %s", localPath, expectedHash, actualHash)
		}
	}

	t.Logf("upload_existing=true: all %d files (pre-existing + new) uploaded and verified",
		len(allFiles))
}
