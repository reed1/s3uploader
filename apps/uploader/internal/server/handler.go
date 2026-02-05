package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
)

type Handler struct {
	storage Storage
}

func NewHandler(storage Storage) *Handler {
	return &Handler{storage: storage}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth *AuthMiddleware) {
	mux.HandleFunc("/health", h.handleHealth)
	mux.Handle("/upload", auth.Wrap(http.HandlerFunc(h.handleUpload)))
	mux.Handle("/exists", auth.Wrap(http.HandlerFunc(h.handleExists)))
	mux.Handle("/download", auth.Wrap(http.HandlerFunc(h.handleDownload)))
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clientName := GetClientName(r.Context())

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, "failed to parse multipart form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	remotePath := r.FormValue("path")
	if remotePath == "" {
		http.Error(w, "missing path field", http.StatusBadRequest)
		return
	}

	if !isValidPath(remotePath) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	s3Key, err := h.storage.Upload(r.Context(), clientName, remotePath, file, header.Size)
	if err != nil {
		http.Error(w, "upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"s3_key":  s3Key,
		"size":    header.Size,
	})
}

func (h *Handler) handleExists(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clientName := GetClientName(r.Context())

	remotePath := r.URL.Query().Get("path")
	if remotePath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}

	if !isValidPath(remotePath) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	exists, err := h.storage.Exists(r.Context(), clientName, remotePath)
	if err != nil {
		http.Error(w, "check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"exists": exists})
}

func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clientName := GetClientName(r.Context())

	remotePath := r.URL.Query().Get("path")
	if remotePath == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}

	if !isValidPath(remotePath) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	body, contentType, err := h.storage.Download(r.Context(), clientName, remotePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		http.Error(w, "download failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", contentType)
	io.Copy(w, body)
}

func isValidPath(p string) bool {
	if strings.Contains(p, "..") {
		return false
	}
	cleaned := path.Clean(p)
	if strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "..") {
		return false
	}
	return true
}
