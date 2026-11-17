package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/graph-gophers/graphql-go"
)

// serveLegacy implements the subscriptions-transport-ws ("graphql-ws")
// protocol. Its wire vocabulary is:
//
//	C→S: connection_init, start, stop, connection_terminate
//	S→C: connection_ack, data, error, complete
func serveLegacy(
	r *http.Request,
	state *connState,
	schema *graphql.Schema,
	cfg *WSOptions,
) {
	for {
		_, message, err := state.conn.ReadMessage()
		if err != nil {
			state.cancelAll()
			return
		}

		var msg wsMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "connection_init":
			user := authenticateConnection(r, msg.Payload, cfg)
			state.setUser(user)
			_ = writeJSON(state, wsMessage{Type: "connection_ack"})

		case "start":
			var payload struct {
				Query     string                 `json:"query"`
				Variables map[string]interface{} `json:"variables"`
			}
			_ = json.Unmarshal(msg.Payload, &payload)

			ctx, cancel := context.WithCancel(r.Context())
			if u := state.getUser(); u != nil && cfg.WithUserFunc != nil {
				ctx = cfg.WithUserFunc(ctx, u)
			}

			state.trackSubscription(msg.ID, cancel)

			go runSubscription(
				state, msg.ID, ctx, schema,
				payload.Query, payload.Variables, "data",
			)

		case "stop":
			state.cancelSubscription(msg.ID)

		case "connection_terminate":
			state.cancelAll()
			return
		}
	}
}
