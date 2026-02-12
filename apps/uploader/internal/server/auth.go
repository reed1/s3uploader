package server

import (
	"context"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type contextKey string

const clientIDKey contextKey = "clientID"

type AuthMiddleware struct {
	mu      sync.RWMutex
	clients map[string]string // apiKey -> clientID
}

func NewAuthMiddleware(clients []ClientEntry) *AuthMiddleware {
	m := &AuthMiddleware{
		clients: make(map[string]string),
	}
	for _, c := range clients {
		m.clients[c.APIKey] = c.ID
	}
	return m
}

func (a *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "invalid authorization format", http.StatusUnauthorized)
			return
		}

		apiKey := strings.TrimPrefix(auth, "Bearer ")

		a.mu.RLock()
		clientID, ok := a.clients[apiKey]
		a.mu.RUnlock()

		if !ok {
			http.Error(w, "invalid api key", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), clientIDKey, clientID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *AuthMiddleware) UpdateClients(clients []ClientEntry) {
	m := make(map[string]string, len(clients))
	for _, c := range clients {
		m[c.APIKey] = c.ID
	}
	a.mu.Lock()
	a.clients = m
	a.mu.Unlock()
}

func (a *AuthMiddleware) WatchClientsFile(path string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Name != path {
					continue
				}
				if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
					continue
				}
				clients, err := LoadClientsConfig(path)
				if err != nil {
					log.Fatalf("failed to reload clients config: %v", err)
				}
				a.UpdateClients(clients)
				log.Printf("reloaded clients config: %d clients", len(clients))

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("clients file watcher error: %v", err)
			}
		}
	}()

	if err := watcher.Add(filepath.Dir(path)); err != nil {
		watcher.Close()
		return nil, err
	}

	return watcher, nil
}

func GetClientID(ctx context.Context) string {
	if id, ok := ctx.Value(clientIDKey).(string); ok {
		return id
	}
	return ""
}
