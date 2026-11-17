package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/graph-gophers/graphql-go"
)

// Close codes defined by the graphql-transport-ws spec.
const (
	wsCloseBadRequest       = 4400
	wsCloseUnauthorized     = 4401
	wsCloseSubscriberExists = 4409
	wsCloseTooManyInit      = 4429
	wsCloseWriteWait        = 5 * time.Second
)

// serveModern implements the graphql-transport-ws protocol. Its wire
// vocabulary is:
//
//	C→S: connection_init, subscribe, complete, ping, pong
//	S→C: connection_ack, next, error, complete, ping, pong
func serveModern(
	r *http.Request,
	state *connState,
	schema *graphql.Schema,
	cfg *WSOptions,
) {
	initDone := false

	for {
		_, message, err := state.conn.ReadMessage()
		if err != nil {
			state.cancelAll()
			return
		}

		var msg wsMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			state.closeWith(wsCloseBadRequest, "invalid JSON")
			return
		}

		if msg.Type == "connection_init" {
			if initDone {
				state.closeWith(wsCloseTooManyInit, "too many connection_init requests")
				return
			}
			user := authenticateConnection(r, msg.Payload, cfg)
			state.setUser(user)
			_ = writeJSON(state, wsMessage{Type: "connection_ack"})
			initDone = true
			continue
		}

		if !initDone {
			state.closeWith(wsCloseBadRequest, "connection_init must be first")
			return
		}

		switch msg.Type {
		case "subscribe":
			var payload struct {
				Query     string                 `json:"query"`
				Variables map[string]interface{} `json:"variables"`
			}
			_ = json.Unmarshal(msg.Payload, &payload)

			if payload.Query == "" {
				_ = writeError(state, msg.ID, errMissingQuery)
				continue
			}

			if state.hasSubscription(msg.ID) {
				state.closeWith(wsCloseSubscriberExists, "subscriber for this id already exists")
				return
			}

			ctx, cancel := context.WithCancel(r.Context())
			if u := state.getUser(); u != nil && cfg.WithUserFunc != nil {
				ctx = cfg.WithUserFunc(ctx, u)
			}
			state.trackSubscription(msg.ID, cancel)

			go runSubscription(
				state, msg.ID, ctx, schema,
				payload.Query, payload.Variables, "next",
			)

		case "complete": // client-initiated unsubscribe
			state.cancelSubscription(msg.ID)

		case "ping":
			_ = writeJSON(state, wsMessage{Type: "pong", Payload: msg.Payload})

		case "pong":
			// No-op; optional liveness check can live here.

		default:
			state.closeWith(wsCloseBadRequest, "unknown message type: "+msg.Type)
			return
		}
	}
}

// closeWith sends a WebSocket close frame with the given code and reason,
// then closes the underlying connection. Serialized with other writes so
// we don't close mid-frame.
func (s *connState) closeWith(code int, reason string) {
	_ = s.writeControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason),
		time.Now().Add(wsCloseWriteWait),
	)
	_ = s.conn.Close()
}

// errMissingQuery is returned when a subscribe message has no query string.
var errMissingQuery = wsError("missing query")

// wsError is a minimal error type used for protocol-level errors.
type wsError string

func (e wsError) Error() string { return string(e) }
