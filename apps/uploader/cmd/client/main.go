package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"s3uploader/internal/client"
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

	processor := client.NewProcessor(queue, db, uploader, cfg)
	go processor.Run(nil)

	<-stop
	log.Println("shutting down")
}
