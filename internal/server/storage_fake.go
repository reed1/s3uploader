package server

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type FakeStorage struct {
	mu         sync.RWMutex
	baseDir    string
	pathPrefix string
}

func NewFakeStorage(baseDir, pathPrefix string) *FakeStorage {
	return &FakeStorage{
		baseDir:    baseDir,
		pathPrefix: pathPrefix,
	}
}

func (f *FakeStorage) buildPath(clientName, remotePath string) string {
	return filepath.Join(f.baseDir, f.pathPrefix, clientName, remotePath)
}

func (f *FakeStorage) Upload(ctx context.Context, clientName, remotePath string, body io.Reader, size int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	fullPath := f.buildPath(clientName, remotePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", err
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, body); err != nil {
		return "", err
	}

	key := filepath.Join(f.pathPrefix, clientName, remotePath)
	return key, nil
}

func (f *FakeStorage) Exists(ctx context.Context, clientName, remotePath string) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	fullPath := f.buildPath(clientName, remotePath)
	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (f *FakeStorage) Download(ctx context.Context, clientName, remotePath string) (io.ReadCloser, string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	fullPath := f.buildPath(clientName, remotePath)
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", os.ErrNotExist
		}
		return nil, "", err
	}

	return file, "application/octet-stream", nil
}

func (f *FakeStorage) GetFilePath(clientName, remotePath string) string {
	return f.buildPath(clientName, remotePath)
}
