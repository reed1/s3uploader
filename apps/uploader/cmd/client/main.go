package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"s3uploader/internal/client"
)

func durationUntilNext(hour int) time.Duration {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}

func retryRestartLoop(processor *client.Processor, processorDone <-chan struct{}) {
	for {
		time.Sleep(durationUntilNext(1))

		if !processor.HasFailures() {
			continue
		}

		files := processor.FailedFiles()
		log.Printf("exiting due to failed uploads, sample failed files: [%s]. service will restart and rescan.",
			strings.Join(files, ", "))

		processor.Stop()
		<-processorDone
		os.Exit(1)
	}
}

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

	watcher, err := client.NewWatcher(cfg.Watches, queue, cfg)
	if err != nil {
		log.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	if err := watcher.Start(); err != nil {
		log.Fatalf("failed to start watcher: %v", err)
	}

	scanner := client.NewScanner(cfg.Watches, queue, cfg.Scan.UploadExisting, cfg)
	if err := scanner.Scan(); err != nil {
		log.Fatalf("failed to scan directories: %v", err)
	}

	log.Printf("client started, watching %d directories", len(cfg.Watches))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	processor := client.NewProcessor(queue, db, uploader, cfg)
	processorDone := make(chan struct{})
	go func() {
		processor.Run(nil)
		close(processorDone)
	}()

	go retryRestartLoop(processor, processorDone)

	<-stop
	log.Println("shutting down")
}
