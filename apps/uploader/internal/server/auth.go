package server

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const clientNameKey contextKey = "clientName"

type AuthMiddleware struct {
	clients map[string]string // apiKey -> clientName
}

func NewAuthMiddleware(clients []ClientEntry) *AuthMiddleware {
	m := &AuthMiddleware{
		clients: make(map[string]string),
	}
	for _, c := range clients {
		m.clients[c.APIKey] = c.Name
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
		clientName, ok := a.clients[apiKey]
		if !ok {
			http.Error(w, "invalid api key", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), clientNameKey, clientName)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetClientName(ctx context.Context) string {
	if name, ok := ctx.Value(clientNameKey).(string); ok {
		return name
	}
	return ""
}
