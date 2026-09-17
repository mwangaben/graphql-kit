// Package handlers provides HTTP and WebSocket handlers for GraphQL.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
)

// HTTPHandler wraps graph-gophers' relay handler with sensible
// defaults for GraphQL-over-HTTP.
//
// Features:
//   - POST for standard queries/mutations
//   - GET for queries (with optional Playground UI)
//   - CORS headers
//   - Method validation
//
// Example:
//
//	schema := graphql.MustParseSchema(sdl, resolver)
//	handler := handlers.NewHTTP(schema)
//	http.Handle("/graphql", handler)
type HTTPHandler struct {
	schema     *graphql.Schema
	relay      http.Handler
	playground bool
	corsOrigin string
}

// HTTPOptions configures the HTTP handler.
type HTTPOptions struct {
	// Playground enables the GraphQL Playground UI at GET.
	// Defaults to true.
	Playground bool

	// CORSOrigin sets the Access-Control-Allow-Origin header.
	// Defaults to "*" (allow all). Set to specific origins in production.
	CORSOrigin string
}

// NewHTTP creates a new HTTP handler.
func NewHTTP(schema *graphql.Schema, opts ...func(*HTTPOptions)) *HTTPHandler {
	cfg := &HTTPOptions{
		Playground: true,
		CORSOrigin: "*",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return &HTTPHandler{
		schema:     schema,
		relay:      &relay.Handler{Schema: schema},
		playground: cfg.Playground,
		corsOrigin: cfg.CORSOrigin,
	}
}

// ServeHTTP implements http.Handler.
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", h.corsOrigin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case http.MethodPost:
		h.relay.ServeHTTP(w, r)
	case http.MethodGet:
		h.serveGet(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// serveGet handles GET requests — either a query or the Playground UI.
func (h *HTTPHandler) serveGet(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		if h.playground {
			h.servePlayground(w)
			return
		}
		http.Error(w, "Missing query", http.StatusBadRequest)
		return
	}

	// Execute the query
	variables := make(map[string]interface{})
	if v := r.URL.Query().Get("variables"); v != "" {
		_ = json.Unmarshal([]byte(v), &variables)
	}

	resp := h.schema.Exec(
		r.Context(),
		query,
		r.URL.Query().Get("operationName"),
		variables,
	)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// servePlayground serves the GraphQL Playground UI.
func (h *HTTPHandler) servePlayground(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(playgroundHTML))
}
