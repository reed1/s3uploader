package client

import (
	"os"
	"path/filepath"
)

type Scanner struct {
	queue *Queue
	cfg   *Config
}

func NewScanner(queue *Queue, cfg *Config) *Scanner {
	return &Scanner{
		queue: queue,
		cfg:   cfg,
	}
}

func (s *Scanner) Scan() error {
	if !s.cfg.Scan.UploadExisting {
		return nil
	}

	for _, watch := range s.cfg.Watches {
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
	if s.cfg.IsExcluded(remotePath) {
		return nil
	}
	s.queue.Enqueue(localPath, remotePath)
	return nil
}
