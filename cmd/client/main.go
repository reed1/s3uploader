package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reed/s3uploader/internal/client"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: s3up --config <path>")
		os.Exit(1)
	}

	cfg, err := client.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	db, err := client.NewDB(cfg.Database.Path)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	queue := client.NewQueue()
	uploader := client.NewUploader(cfg.Server.URL, cfg.Server.APIKey)

	watcher, err := client.NewWatcher(cfg.Watches, queue)
	if err != nil {
		log.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	if err := watcher.Start(); err != nil {
		log.Fatalf("failed to start watcher: %v", err)
	}

	scanner := client.NewScanner(cfg.Watches, queue, cfg.Scan.UploadExisting)
	if err := scanner.Scan(); err != nil {
		log.Fatalf("failed to scan directories: %v", err)
	}

	log.Printf("client started, watching %d directories", len(cfg.Watches))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go processQueue(queue, db, uploader, cfg)

	<-stop
	log.Println("shutting down")
}

func processQueue(queue *client.Queue, db *client.DB, uploader *client.Uploader, cfg *client.Config) {
	maxSizeBytes := int64(cfg.Upload.MaxFileSizeMB) * 1024 * 1024
	debounce := time.Duration(cfg.Stability.DebounceSeconds) * time.Second

	for {
		entry, ok := queue.Dequeue()
		if !ok {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		processEntry(entry, queue, db, uploader, maxSizeBytes, debounce, cfg)
	}
}

func processEntry(entry client.QueueEntry, queue *client.Queue, db *client.DB, uploader *client.Uploader, maxSizeBytes int64, debounce time.Duration, cfg *client.Config) {
	info, err := os.Stat(entry.LocalPath)
	if err != nil {
		return
	}

	rec, err := db.GetFile(entry.LocalPath)
	if err != nil {
		log.Printf("db error for %s: %v", entry.LocalPath, err)
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
		log.Printf("skipped %s: file too large (%d bytes)", entry.LocalPath, info.Size())
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
			log.Printf("giving up on %s after %d stability attempts", entry.LocalPath, entry.AttemptCount)
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
		log.Printf("upload attempt %d failed for %s: %v", attempt+1, entry.LocalPath, lastErr)
	}

	if lastErr != nil {
		log.Printf("upload failed for %s after %d attempts: %v", entry.LocalPath, cfg.Upload.RetryAttempts, lastErr)
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

	log.Printf("uploaded %s -> %s", entry.LocalPath, entry.RemotePath)
}
