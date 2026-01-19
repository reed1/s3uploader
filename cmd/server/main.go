package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/reed/s3uploader/internal/server"
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

	s3Client := server.NewS3Client(cfg.S3)
	auth := server.NewAuthMiddleware(cfg.Clients)
	handler := server.NewHandler(s3Client)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, auth)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("starting server on %s", addr)

	if cfg.Server.TLS.Enabled {
		log.Fatal(http.ListenAndServeTLS(addr, cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile, mux))
	} else {
		log.Fatal(http.ListenAndServe(addr, mux))
	}
}
