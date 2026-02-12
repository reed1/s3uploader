package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"s3uploader/internal/client"
)

func TestE2E_ExcludePatterns(t *testing.T) {
	env := newTestEnvWithExcludes(t, []string{"/thumbnails/", `\.tmp$`})
	defer env.cleanup()

	thumbDir := filepath.Join(env.watchDir, "thumbnails")
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		t.Fatalf("failed to create thumbnails dir: %v", err)
	}

	excludedFiles := generateRandomFilesWithPrefix(t, thumbDir, 2, "thumb_")

	tmpFiles := generateFilesWithSuffix(t, env.watchDir, 2, ".tmp")
	for k, v := range tmpFiles {
		excludedFiles[k] = v
	}

	normalFiles := generateRandomFiles(t, env.watchDir, 3)

	scanner := client.NewScanner(env.watches, env.queue, true, env.cfg)
	if err := scanner.Scan(); err != nil {
		t.Fatalf("failed to scan: %v", err)
	}

	stopProcessor := make(chan struct{})
	go env.processor.Run(stopProcessor)
	defer close(stopProcessor)

	waitForUploads(t, env.db, normalFiles, 30*time.Second)

	for localPath, expectedHash := range normalFiles {
		relPath, _ := filepath.Rel(env.watchDir, localPath)
		remotePath := filepath.Join("uploads", relPath)
		storagePath := env.storage.GetFilePath("test-client", remotePath)

		actualHash := hashFile(t, storagePath)
		if actualHash != expectedHash {
			t.Errorf("hash mismatch for %s: expected %s, got %s", localPath, expectedHash, actualHash)
		}
	}

	time.Sleep(2 * time.Second)

	for localPath := range excludedFiles {
		relPath, _ := filepath.Rel(env.watchDir, localPath)
		remotePath := filepath.Join("uploads", relPath)
		storagePath := env.storage.GetFilePath("test-client", remotePath)

		if _, err := os.Stat(storagePath); err == nil {
			t.Errorf("excluded file %s should NOT have been uploaded but was (remote: %s)", localPath, remotePath)
		}
	}

	t.Logf("exclude_patterns: %d normal files uploaded, %d excluded files correctly skipped",
		len(normalFiles), len(excludedFiles))
}
