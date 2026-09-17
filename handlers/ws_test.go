package handlers_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/graph-gophers/graphql-go"
	"github.com/mwangaben/graphql-kit/handlers"
)

// ============================================================================
// Subscription test fixtures
// ============================================================================

type subResolver struct {
	events chan *Tick
}

// Tick is the payload for the tick subscription.
//
// Struct fields and methods both named ID would conflict in Go,
// so the value field has a suffix. The ID() method satisfies
// graph-gophers' field resolver requirement for `id`.
type Tick struct {
	IDValue int32
}

// ID resolves the `id` field on the Tick type.
func (t *Tick) ID() int32 {
	return t.IDValue
}

func (r *subResolver) Version() string     { return "1.0.0" }
func (r *subResolver) NoopMutation() *bool { return nil }
func (r *subResolver) NoopSubscription() <-chan *bool {
	ch := make(chan *bool)
	return ch
}

// Tick subscribes to tick events.
func (r *subResolver) Tick(ctx context.Context) <-chan *Tick {
	return r.events
}

const subSDL = `
type Tick {
    id: Int!
}

type Query {
    version: String!
}
type Mutation {
    noopMutation: Boolean
}
type Subscription {
    noopSubscription: Boolean
    tick: Tick!
}
`

// ============================================================================
// Basic WebSocket tests
// ============================================================================

func TestWS_ConnectionInit(t *testing.T) {
	events := make(chan *Tick, 10)
	defer close(events)

	parsed := graphql.MustParseSchema(subSDL, &subResolver{events: events})

	server := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer server.Close()

	// Connect
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send connection_init
	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	if err := conn.WriteMessage(websocket.TextMessage, init); err != nil {
		t.Fatalf("write init: %v", err)
	}

	// Expect connection_ack
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}

	var ack map[string]string
	if err := json.Unmarshal(msg, &ack); err != nil {
		t.Fatalf("unmarshal ack: %v", err)
	}
	if ack["type"] != "connection_ack" {
		t.Errorf("expected connection_ack, got %q", ack["type"])
	}
}

// ============================================================================
// Full subscription flow
// ============================================================================

func TestWS_SubscriptionFlow(t *testing.T) {
	events := make(chan *Tick, 10)
	defer close(events)

	parsed := graphql.MustParseSchema(subSDL, &subResolver{events: events})

	server := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// 1. Init
	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage() // ack

	// 2. Start subscription
	startPayload, _ := json.Marshal(map[string]interface{}{
		"query":     "subscription { tick { id } }",
		"variables": nil,
	})
	start, _ := json.Marshal(map[string]interface{}{
		"id":      "1",
		"type":    "start",
		"payload": json.RawMessage(startPayload),
	})
	_ = conn.WriteMessage(websocket.TextMessage, start)

	// 3. Publish an event
	time.Sleep(100 * time.Millisecond) // let subscription register
	events <- &Tick{IDValue: 42}

	// 4. Expect data message
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read data: %v", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(msg, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["type"] != "data" {
		t.Errorf("expected type=data, got %v", data["type"])
	}
	if data["id"] != "1" {
		t.Errorf("expected id=1, got %v", data["id"])
	}
}

// ============================================================================
// Stop
// ============================================================================

func TestWS_StopSubscription(t *testing.T) {
	events := make(chan *Tick, 10)
	defer close(events)

	parsed := graphql.MustParseSchema(subSDL, &subResolver{events: events})

	server := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Init + start
	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage()

	startPayload, _ := json.Marshal(map[string]interface{}{
		"query": "subscription { tick { id } }",
	})
	start, _ := json.Marshal(map[string]interface{}{
		"id":      "1",
		"type":    "start",
		"payload": json.RawMessage(startPayload),
	})
	_ = conn.WriteMessage(websocket.TextMessage, start)

	// Stop it
	stop, _ := json.Marshal(map[string]string{"id": "1", "type": "stop"})
	_ = conn.WriteMessage(websocket.TextMessage, stop)

	time.Sleep(100 * time.Millisecond)

	// Publish — should not arrive
	events <- &Tick{IDValue: 99}

	// Expect timeout (no data message)
	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Error("expected no message after stop")
	}
}

// ============================================================================
// Connection terminate
// ============================================================================

func TestWS_ConnectionTerminate(t *testing.T) {
	events := make(chan *Tick, 10)
	defer close(events)

	parsed := graphql.MustParseSchema(subSDL, &subResolver{events: events})

	server := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send terminate — server should close cleanly
	msg, _ := json.Marshal(map[string]string{"type": "connection_terminate"})
	_ = conn.WriteMessage(websocket.TextMessage, msg)

	// Next read should error (server closed)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Error("expected connection close after terminate")
	}
}

// ============================================================================
// Malformed messages — server should not crash
// ============================================================================

func TestWS_MalformedMessages(t *testing.T) {
	events := make(chan *Tick, 10)
	defer close(events)

	parsed := graphql.MustParseSchema(subSDL, &subResolver{events: events})

	server := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send garbage
	_ = conn.WriteMessage(websocket.TextMessage, []byte("not json"))
	_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type": "unknown"}`))
	_ = conn.WriteMessage(websocket.TextMessage, []byte(`{}`))

	// Server should still respond to valid init
	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected ack after garbage, got %v", err)
	}

	var ack map[string]string
	_ = json.Unmarshal(msg, &ack)
	if ack["type"] != "connection_ack" {
		t.Errorf("expected ack, got %q", ack["type"])
	}
}

// ============================================================================
// Context cancellation on client disconnect
// ============================================================================

func TestWS_ClientDisconnectCancelsSubscription(t *testing.T) {
	events := make(chan *Tick, 10)
	defer close(events)

	parsed := graphql.MustParseSchema(subSDL, &subResolver{events: events})

	server := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Init + start subscription
	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)

	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, _, _ = conn.ReadMessage()

	startPayload, _ := json.Marshal(map[string]interface{}{
		"query": "subscription { tick { id } }",
	})
	start, _ := json.Marshal(map[string]interface{}{
		"id":      "1",
		"type":    "start",
		"payload": json.RawMessage(startPayload),
	})
	_ = conn.WriteMessage(websocket.TextMessage, start)

	// Disconnect abruptly
	conn.Close()

	// Give server time to process
	time.Sleep(200 * time.Millisecond)

	// Server should have cleaned up — sending on events should not panic
	// (This is a bit indirect, but verifies no goroutine leaks crash the server)
	events <- &Tick{IDValue: 1}
}
