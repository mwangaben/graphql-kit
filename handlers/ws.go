package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/graph-gophers/graphql-go"
)

// Subprotocol identifiers as negotiated in Sec-WebSocket-Protocol.
const (
	protoLegacy = "graphql-ws"           // subscriptions-transport-ws
	protoModern = "graphql-transport-ws" // graphql-ws (enisdenjo)
)

// upgraderDefault is used when no origin restrictions are configured.
// It advertises both subprotocols; gorilla echoes whichever the client offered.
var upgraderDefault = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
	Subprotocols:    []string{protoModern, protoLegacy},
}

// NewWebSocket returns an http.HandlerFunc that implements GraphQL
// subscriptions over WebSocket.
//
// Two subprotocols are supported, selected by the client's
// Sec-WebSocket-Protocol header:
//
//   - "graphql-transport-ws" (modern, recommended; JS library: graphql-ws)
//   - "graphql-ws"           (legacy; JS library: subscriptions-transport-ws)
//
// If the client sends no subprotocol, the legacy protocol is used to
// preserve backward compatibility with existing deployments.
//
// Authentication is optional and identical across both protocols:
//   - If an Authenticator is provided, the handler extracts the token from
//     the connection_init payload's "Authorization" field, falling back to
//     the ?token= query parameter.
//   - Successful auth injects the user via WithUserFunc before each
//     subscription; failure is non-fatal so public subscriptions still work.
//
// Example:
//
//	handler := handlers.NewWebSocket(schema,
//	    handlers.WithAuthenticator(myAuthenticator),
//	    handlers.WithUserFunc(appctx.WithUser),
//	)
//	http.HandleFunc("/subscriptions", handler)
func NewWebSocket(
	schema *graphql.Schema,
	opts ...func(*WSOptions),
) http.HandlerFunc {
	cfg := &WSOptions{}
	for _, opt := range opts {
		opt(cfg)
	}

	upgrader := upgraderDefault
	if len(cfg.AllowedOrigins) > 0 {
		allowed := make(map[string]bool, len(cfg.AllowedOrigins))
		for _, o := range cfg.AllowedOrigins {
			allowed[o] = true
		}
		upgrader.CheckOrigin = func(r *http.Request) bool {
			return allowed[r.Header.Get("Origin")]
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		requested := negotiateSubprotocol(r)

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("graphql-kit: websocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		// gorilla echoes the negotiated subprotocol (or leaves it empty
		// when the client offered none/unsupported). Fall back sensibly.
		chosen := conn.Subprotocol()
		if chosen == "" {
			chosen = requested
		}
		if chosen == "" {
			chosen = protoLegacy
		}

		state := newConnState(conn)

		switch chosen {
		case protoModern:
			serveModern(r, state, schema, cfg)
		default:
			serveLegacy(r, state, schema, cfg)
		}
	}
}

// negotiateSubprotocol reads the client's requested subprotocols,
// preferring the modern one when both are offered.
func negotiateSubprotocol(r *http.Request) string {
	h := r.Header.Get("Sec-WebSocket-Protocol")
	for _, p := range strings.Split(h, ",") {
		switch strings.TrimSpace(p) {
		case protoModern:
			return protoModern
		case protoLegacy:
			return protoLegacy
		}
	}
	return ""
}

// Silence unused-import warnings if this file stops using these directly.
var _ = context.Background
var _ sync.Mutex
