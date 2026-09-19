package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/errors"
)

// upgraderDefault is used when no origin restrictions are configured.
var upgraderDefault = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	Subprotocols: []string{"graphql-ws"},
}

// NewWebSocket returns an http.HandlerFunc that implements the
// graphql-ws protocol for GraphQL subscriptions.
//
// Authentication is optional:
//
//   - If an Authenticator is provided, the handler tries to
//     authenticate the connection from:
//     1. The connection_init payload's "Authorization" field
//     2. The ?token= query parameter (fallback)
//   - If authentication succeeds, the user is injected into the
//     context via WithUserFunc before running each subscription.
//   - If authentication fails or is absent, the connection proceeds
//     without a user. Private subscriptions return Unauthenticated;
//     public subscriptions work normally.
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
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("graphql-kit: websocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		var (
			mu             sync.Mutex
			subscriptions  = make(map[string]context.CancelFunc)
			connectionUser any // set after successful auth
		)

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				// Client disconnected — cancel all subscriptions
				mu.Lock()
				for _, cancel := range subscriptions {
					cancel()
				}
				mu.Unlock()
				return
			}

			var msg wsMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}

			switch msg.Type {
			case "connection_init":
				user := authenticateConnection(r, msg.Payload, cfg)
				mu.Lock()
				connectionUser = user
				mu.Unlock()

				_ = writeJSON(conn, wsMessage{Type: "connection_ack"})

			case "start":
				var payload struct {
					Query     string                 `json:"query"`
					Variables map[string]interface{} `json:"variables"`
				}
				_ = json.Unmarshal(msg.Payload, &payload)

				ctx, cancel := context.WithCancel(r.Context())

				// Inject the connection's user into the subscription context
				mu.Lock()
				user := connectionUser
				mu.Unlock()

				if user != nil && cfg.WithUserFunc != nil {
					ctx = cfg.WithUserFunc(ctx, user)
				}

				mu.Lock()
				subscriptions[msg.ID] = cancel
				mu.Unlock()

				go runSubscription(conn, &mu, msg.ID, ctx, schema, payload.Query, payload.Variables)

			case "stop":
				mu.Lock()
				if cancel, ok := subscriptions[msg.ID]; ok {
					cancel()
					delete(subscriptions, msg.ID)
				}
				mu.Unlock()

			case "connection_terminate":
				return
			}
		}
	}
}

// authenticateConnection extracts a token from the connection_init
// payload or the query string, and validates it via the Authenticator.
//
// Returns the authenticated user, or nil if auth is not configured or
// fails. Failures are non-fatal — public subscriptions work without auth.
func authenticateConnection(
	r *http.Request,
	initPayload json.RawMessage,
	cfg *WSOptions,
) any {
	if cfg.Authenticator == nil {
		return nil
	}

	// Try to extract token from connection_init payload
	var payload struct {
		Authorization string `json:"Authorization"`
	}
	_ = json.Unmarshal(initPayload, &payload)

	token := strings.TrimSpace(payload.Authorization)

	// Fall back to query string
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	// Strip "Bearer " prefix if present
	token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
	if token == "" {
		return nil
	}

	user, err := cfg.Authenticator.Authenticate(r.Context(), token)
	if err != nil {
		log.Printf("graphql-kit: ws auth failed: %v", err)
		return nil
	}
	return user
}

// runSubscription executes a subscription and streams results.
func runSubscription(
	conn *websocket.Conn,
	mu *sync.Mutex,
	id string,
	ctx context.Context,
	schema *graphql.Schema,
	query string,
	variables map[string]interface{},
) {
	ch, err := schema.Subscribe(ctx, query, "", variables)
	if err != nil {
		_ = writeError(conn, id, err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-ch:
			if !ok {
				_ = writeJSON(conn, wsMessage{ID: id, Type: "complete"})
				return
			}

			resp, ok := result.(*graphql.Response)
			if !ok {
				continue
			}

			if len(resp.Errors) > 0 {
				_ = writeErrors(conn, id, resp.Errors)
				continue
			}

			payload, _ := json.Marshal(map[string]interface{}{
				"data": resp.Data,
			})
			_ = writeJSON(conn, wsMessage{
				ID:      id,
				Type:    "data",
				Payload: payload,
			})
		}
	}
}

// writeJSON sends a protocol message over the WebSocket.
func writeJSON(conn *websocket.Conn, msg wsMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// writeError sends an error message for a subscription.
func writeError(conn *websocket.Conn, id string, err error) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"errors": []map[string]string{{"message": err.Error()}},
	})
	return writeJSON(conn, wsMessage{
		ID:      id,
		Type:    "error",
		Payload: payload,
	})
}

// writeErrors sends multiple errors for a subscription.
func writeErrors(conn *websocket.Conn, id string, errs []*errors.QueryError) error {
	payload, _ := json.Marshal(map[string]interface{}{"errors": errs})
	return writeJSON(conn, wsMessage{
		ID:      id,
		Type:    "error",
		Payload: payload,
	})
}

// wsMessage is the protocol message format for graphql-ws.
type wsMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}
