package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/errors"
)

// wsMessage is the wire format for both protocols.
// `payload` is json.RawMessage so the caller can decode it per-message-type.
type wsMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// connState holds per-connection state shared by both protocol loops.
//
// Two mutexes, two distinct jobs:
//   - mu guards subscriptions and user (fast, in-memory).
//   - writeMu serializes all writes to conn. Gorilla allows exactly one
//     concurrent writer; every write path must go through it.
type connState struct {
	conn          *websocket.Conn
	mu            sync.Mutex // guards subscriptions and user
	writeMu       sync.Mutex // serializes all writes to conn
	subscriptions map[string]context.CancelFunc
	user          any // set after successful connection_init auth
}

func newConnState(conn *websocket.Conn) *connState {
	return &connState{
		conn:          conn,
		subscriptions: make(map[string]context.CancelFunc),
	}
}

func (s *connState) setUser(u any) { s.mu.Lock(); s.user = u; s.mu.Unlock() }
func (s *connState) getUser() any  { s.mu.Lock(); defer s.mu.Unlock(); return s.user }

func (s *connState) hasSubscription(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.subscriptions[id]
	return ok
}

func (s *connState) trackSubscription(id string, cancel context.CancelFunc) {
	s.mu.Lock()
	s.subscriptions[id] = cancel
	s.mu.Unlock()
}

func (s *connState) cancelSubscription(id string) {
	s.mu.Lock()
	if cancel, ok := s.subscriptions[id]; ok {
		cancel()
		delete(s.subscriptions, id)
	}
	s.mu.Unlock()
}

func (s *connState) cancelAll() {
	s.mu.Lock()
	for _, cancel := range s.subscriptions {
		cancel()
	}
	s.subscriptions = make(map[string]context.CancelFunc)
	s.mu.Unlock()
}

// writeMessage serializes writes to the underlying connection.
// All data-frame writes must go through this method.
func (s *connState) writeMessage(messageType int, data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(messageType, data)
}

// writeControl sends a WebSocket control frame. Gorilla permits one
// concurrent WriteMessage alongside WriteControl, but we take writeMu
// so we don't close the connection mid-frame from another goroutine.
func (s *connState) writeControl(messageType int, data []byte, deadline time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteControl(messageType, data, deadline)
}

// authenticateConnection extracts a token from the connection_init payload
// (preferred) or the ?token= query string (fallback), and validates it via
// the configured Authenticator.
//
// Returns the authenticated user, or nil if auth is not configured or fails.
// Failures are non-fatal — public subscriptions work without auth.
func authenticateConnection(
	r *http.Request,
	initPayload json.RawMessage,
	cfg *WSOptions,
) any {
	if cfg.Authenticator == nil {
		return nil
	}

	var payload struct {
		Authorization string `json:"Authorization"`
	}
	_ = json.Unmarshal(initPayload, &payload)

	token := strings.TrimSpace(payload.Authorization)
	if token == "" {
		token = r.URL.Query().Get("token")
	}
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
//
// dataType is the wire message type used for data frames:
// "data" for the legacy protocol, "next" for graphql-transport-ws.
func runSubscription(
	state *connState,
	id string,
	ctx context.Context,
	schema *graphql.Schema,
	query string,
	variables map[string]interface{},
	dataType string,
) {
	ch, err := schema.Subscribe(ctx, query, "", variables)
	if err != nil {
		_ = writeError(state, id, err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-ch:
			if !ok {
				_ = writeJSON(state, wsMessage{ID: id, Type: "complete"})
				state.cancelSubscription(id)
				return
			}

			resp, ok := result.(*graphql.Response)
			if !ok {
				continue
			}

			if len(resp.Errors) > 0 {
				_ = writeErrors(state, id, resp.Errors)
				continue
			}

			payload, _ := json.Marshal(map[string]interface{}{
				"data": resp.Data,
			})
			_ = writeJSON(state, wsMessage{
				ID:      id,
				Type:    dataType,
				Payload: payload,
			})
		}
	}
}

// writeJSON sends a protocol message over the WebSocket.
func writeJSON(state *connState, msg wsMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return state.writeMessage(websocket.TextMessage, data)
}

// writeError sends a single error message for a subscription.
func writeError(state *connState, id string, err error) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"errors": []map[string]string{{"message": err.Error()}},
	})
	return writeJSON(state, wsMessage{
		ID:      id,
		Type:    "error",
		Payload: payload,
	})
}

// writeErrors sends multiple errors for a subscription.
func writeErrors(state *connState, id string, errs []*errors.QueryError) error {
	payload, _ := json.Marshal(map[string]interface{}{"errors": errs})
	return writeJSON(state, wsMessage{
		ID:      id,
		Type:    "error",
		Payload: payload,
	})
}
