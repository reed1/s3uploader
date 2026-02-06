package client

import (
	"os"
	"path/filepath"
)

type Scanner struct {
	watches        []WatchConfig
	queue          *Queue
	uploadExisting bool
}

func NewScanner(watches []WatchConfig, queue *Queue, uploadExisting bool) *Scanner {
	return &Scanner{
		watches:        watches,
		queue:          queue,
		uploadExisting: uploadExisting,
	}
}

func (s *Scanner) Scan() error {
	if !s.uploadExisting {
		return nil
	}

	for _, watch := range s.watches {
		if err := s.scanWatch(watch); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scanner) scanWatch(watch WatchConfig) error {
	return filepath.WalkDir(watch.LocalPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		return s.enqueueFile(path, watch)
	})
}

func (s *Scanner) enqueueFile(localPath string, watch WatchConfig) error {
	relPath, err := filepath.Rel(watch.LocalPath, localPath)
	if err != nil {
		return err
	}
	remotePath := filepath.Join(watch.RemotePrefix, relPath)
	s.queue.Enqueue(localPath, remotePath)
	return nil
}
