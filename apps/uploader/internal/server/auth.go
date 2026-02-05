package server

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const clientIDKey contextKey = "clientID"

type AuthMiddleware struct {
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
		clientID, ok := a.clients[apiKey]
		if !ok {
			http.Error(w, "invalid api key", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), clientIDKey, clientID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetClientID(ctx context.Context) string {
	if id, ok := ctx.Value(clientIDKey).(string); ok {
		return id
	}
	return ""
}
