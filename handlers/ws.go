package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/errors"
)

// upgrader handles the HTTP → WebSocket upgrade.
//
// CheckOrigin allows all origins. In production, restrict this to
// your known frontend origins.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	Subprotocols: []string{"graphql-ws"},
}

// wsMessage is the protocol message format for graphql-ws.
type wsMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// NewWebSocket creates an http.HandlerFunc for GraphQL subscriptions.
//
// It implements the graphql-ws protocol:
//
//   - connection_init → connection_ack
//   - start → data / error / complete
//   - stop
//   - connection_terminate
//
// Example:
//
//	schema := graphql.MustParseSchema(sdl, resolver)
//	http.HandleFunc("/subscriptions", handlers.NewWebSocket(schema))
func NewWebSocket(schema *graphql.Schema) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("graphql-kit: websocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		var (
			mu            sync.Mutex
			subscriptions = make(map[string]context.CancelFunc)
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
				_ = writeJSON(conn, wsMessage{Type: "connection_ack"})

			case "start":
				var payload struct {
					Query     string                 `json:"query"`
					Variables map[string]interface{} `json:"variables"`
				}
				_ = json.Unmarshal(msg.Payload, &payload)

				ctx, cancel := context.WithCancel(r.Context())
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
