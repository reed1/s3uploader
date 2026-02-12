package test

import (
	"path/filepath"
	"testing"
	"time"

	"s3uploader/internal/client"
)

func TestE2E_UploadAndVerify(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()

	watcher, err := client.NewWatcher(env.watches, env.queue, env.cfg)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	if err := watcher.Start(); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	stopProcessor := make(chan struct{})
	go env.processor.Run(stopProcessor)
	defer close(stopProcessor)

	testFiles := generateRandomFiles(t, env.watchDir, 5)

	waitForUploads(t, env.db, testFiles, 30*time.Second)

	for localPath, expectedHash := range testFiles {
		relPath, _ := filepath.Rel(env.watchDir, localPath)
		remotePath := filepath.Join("uploads", relPath)
		storagePath := env.storage.GetFilePath("test-client", remotePath)

		actualHash := hashFile(t, storagePath)
		if actualHash != expectedHash {
			t.Errorf("hash mismatch for %s: expected %s, got %s", localPath, expectedHash, actualHash)
		}
	}

	t.Logf("all %d files uploaded and verified successfully", len(testFiles))
}
