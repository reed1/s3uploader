package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Handler struct {
	s3 *S3Client
}

func NewHandler(s3 *S3Client) *Handler {
	return &Handler{s3: s3}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, auth *AuthMiddleware) {
	mux.HandleFunc("GET /health", h.handleHealth)
	mux.Handle("POST /upload", auth.Wrap(http.HandlerFunc(h.handleUpload)))
	mux.Handle("GET /exists", auth.Wrap(http.HandlerFunc(h.handleExists)))
	mux.Handle("GET /download", auth.Wrap(http.HandlerFunc(h.handleDownload)))
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
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

	s3Key, err := h.s3.Upload(r.Context(), clientName, remotePath, file, header.Size)
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

	exists, err := h.s3.Exists(r.Context(), clientName, remotePath)
	if err != nil {
		http.Error(w, "check failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"exists": exists})
}

func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request) {
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

	body, contentType, err := h.s3.Download(r.Context(), clientName, remotePath)
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
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
