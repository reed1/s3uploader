package client

import (
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const maxTrackedFailures = 10

type Processor struct {
	queue        *Queue
	db           *DB
	uploader     *Uploader
	cfg          *Config
	maxSizeBytes int64
	debounce     time.Duration

	failedMu    sync.Mutex
	failedFiles []string
	stopping    atomic.Bool
}

func NewProcessor(queue *Queue, db *DB, uploader *Uploader, cfg *Config) *Processor {
	return &Processor{
		queue:        queue,
		db:           db,
		uploader:     uploader,
		cfg:          cfg,
		maxSizeBytes: int64(cfg.Upload.MaxFileSizeMB) * 1024 * 1024,
		debounce:     time.Duration(cfg.Stability.DebounceSeconds) * time.Second,
	}
}

func (p *Processor) Run(stop <-chan struct{}) {
	for {
		if p.stopping.Load() {
			return
		}

		if stop != nil {
			select {
			case <-stop:
				return
			default:
			}
		}

		entry, ok := p.queue.Dequeue()
		if !ok {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		p.ProcessEntry(entry)
	}
}

func (p *Processor) Stop() {
	p.stopping.Store(true)
}

func (p *Processor) HasFailures() bool {
	p.failedMu.Lock()
	defer p.failedMu.Unlock()
	return len(p.failedFiles) > 0
}

func (p *Processor) FailedFiles() []string {
	p.failedMu.Lock()
	defer p.failedMu.Unlock()
	result := make([]string, len(p.failedFiles))
	copy(result, p.failedFiles)
	return result
}

func (p *Processor) recordFailure(path string) {
	p.failedMu.Lock()
	defer p.failedMu.Unlock()
	if len(p.failedFiles) < maxTrackedFailures {
		p.failedFiles = append(p.failedFiles, path)
	}
}

func (p *Processor) ProcessEntry(entry QueueEntry) {
	info, err := os.Stat(entry.LocalPath)
	if err != nil {
		return
	}

	rec, err := p.db.GetFile(entry.LocalPath)
	if err != nil {
		log.Printf("db error for %s: %v", entry.LocalPath, err)
		return
	}

	currentMtime := info.ModTime().UTC().Unix()
	if rec != nil && rec.Mtime == currentMtime {
		return
	}

	if info.Size() > p.maxSizeBytes {
		reason := "file_too_large"
		if rec == nil {
			p.db.InsertFile(entry.LocalPath, entry.RemotePath, info.Size(), currentMtime, &reason)
		} else {
			p.db.UpdateFile(entry.LocalPath, entry.RemotePath, info.Size(), currentMtime, &reason)
		}
		log.Printf("skipped %s: file too large (%d bytes)", entry.LocalPath, info.Size())
		return
	}

	time.Sleep(p.debounce)

	info2, err := os.Stat(entry.LocalPath)
	if err != nil {
		return
	}

	mtime2 := info2.ModTime().UTC().Unix()
	if mtime2 != currentMtime || info2.Size() != info.Size() {
		entry.AttemptCount++
		if entry.AttemptCount >= p.cfg.Stability.MaxAttempts {
			log.Printf("giving up on %s after %d stability attempts", entry.LocalPath, entry.AttemptCount)
			return
		}
		p.queue.EnqueueWithAttempts(entry.LocalPath, entry.RemotePath, entry.AttemptCount)
		return
	}

	var lastErr error
	for attempt := 0; attempt < p.cfg.Upload.RetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(p.cfg.Upload.RetryDelaySeconds) * time.Second)
		}

		_, lastErr = p.uploader.Upload(entry.LocalPath, entry.RemotePath)
		if lastErr == nil {
			break
		}
		log.Printf("upload attempt %d failed for %s: %v", attempt+1, entry.LocalPath, lastErr)
	}

	if lastErr != nil {
		log.Printf("upload failed for %s after %d attempts: %v", entry.LocalPath, p.cfg.Upload.RetryAttempts, lastErr)
		p.recordFailure(entry.LocalPath)
		return
	}

	info3, err := os.Stat(entry.LocalPath)
	if err != nil {
		return
	}

	mtime3 := info3.ModTime().UTC().Unix()
	if mtime3 != currentMtime {
		p.queue.Enqueue(entry.LocalPath, entry.RemotePath)
		return
	}

	if rec == nil {
		p.db.InsertFile(entry.LocalPath, entry.RemotePath, info.Size(), currentMtime, nil)
	} else {
		p.db.UpdateFile(entry.LocalPath, entry.RemotePath, info.Size(), currentMtime, nil)
	}

	log.Printf("uploaded %s -> %s", entry.LocalPath, entry.RemotePath)
}
