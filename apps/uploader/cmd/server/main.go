package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"s3uploader/internal/server"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: s3up-server --config <path>")
		os.Exit(1)
	}

	cfg, err := server.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	clients, err := server.LoadClientsConfig(cfg.ClientsConfig)
	if err != nil {
		log.Fatalf("failed to load clients config: %v", err)
	}

	var db *server.DB
	if cfg.Database.Path != "" {
		var err error
		db, err = server.NewDB(cfg.Database.Path)
		if err != nil {
			log.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()
	}

	s3Client := server.NewS3Client(cfg.S3)
	auth := server.NewAuthMiddleware(clients)

	watcher, err := auth.WatchClientsFile(cfg.ClientsConfig)
	if err != nil {
		log.Fatalf("failed to start clients file watcher: %v", err)
	}
	defer watcher.Close()

	handler := server.NewHandler(s3Client, db)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, auth)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("starting server on %s", addr)

	log.Fatal(http.ListenAndServe(addr, mux))
}
