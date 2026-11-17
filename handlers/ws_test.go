package handlers_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/graph-gophers/graphql-go"
	"github.com/mwangaben/graphql-kit/handlers"
)

// ============================================================================
// Subscription test fixtures
// ============================================================================

// subResolver delivers each published Tick to every active subscription.
//
// Each Tick() call returns a fresh channel, buffered large enough that a
// batch of publishes never blocks an active subscriber. publish() blocks
// until every subscriber has accepted the event (or the test deadline
// expires), so tests never silently drop events due to timing.
type subResolver struct {
	mu   sync.Mutex
	subs []chan *Tick
}

// publish delivers t to every currently active subscriber.
//
// It blocks until each subscriber has received the event, or the test
// deadline (10s) expires — in which case it calls t.Fatalf. This mirrors
// a real broker that backs-pressure the publisher rather than dropping
// silently, and it removes the race between "subscriber registered in
// subs" and "graph-gophers is actually reading from its channel".
func (r *subResolver) publish(t *testing.T, ev *Tick) {
	t.Helper()
	r.mu.Lock()
	subs := append([]chan *Tick(nil), r.subs...)
	r.mu.Unlock()

	deadline := time.After(10 * time.Second)
	for _, ch := range subs {
		select {
		case ch <- ev:
		case <-deadline:
			t.Fatalf("publish timed out: a subscriber is not draining its channel")
		}
	}
}

// subscriberCount returns the number of registered subscriptions.
func (r *subResolver) subscriberCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.subs)
}

// Tick is the payload for the tick subscription.
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

// Tick subscribes to tick events. Each call gets its own channel, buffered
// to comfortably hold the largest batch any test publishes (n = 100).
func (r *subResolver) Tick(ctx context.Context) <-chan *Tick {
	ch := make(chan *Tick, 256)
	r.mu.Lock()
	r.subs = append(r.subs, ch)
	r.mu.Unlock()
	return ch
}

// ============================================================================
// Helpers
// ============================================================================

// waitForSubscribers polls res until at least n subscriptions are registered,
// or fails the test after a short deadline. Replaces fragile fixed sleeps.
func waitForSubscribers(t *testing.T, res *subResolver, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if res.subscriberCount() >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d subscriber(s), have %d",
				n, res.subscriberCount())
		}
		time.Sleep(5 * time.Millisecond)
	}
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
	parsed := graphql.MustParseSchema(subSDL, &subResolver{})

	server := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	if err := conn.WriteMessage(websocket.TextMessage, init); err != nil {
		t.Fatalf("write init: %v", err)
	}

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
	res := &subResolver{}
	parsed := graphql.MustParseSchema(subSDL, res)

	server := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage() // ack

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

	// Wait until the resolver has registered the subscription, then publish.
	waitForSubscribers(t, res, 1)
	res.publish(t, &Tick{IDValue: 42})

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
	res := &subResolver{}
	parsed := graphql.MustParseSchema(subSDL, res)

	server := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

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

	waitForSubscribers(t, res, 1)

	stop, _ := json.Marshal(map[string]string{"id": "1", "type": "stop"})
	_ = conn.WriteMessage(websocket.TextMessage, stop)

	time.Sleep(100 * time.Millisecond)

	// Publish — should not arrive because the subscription was stopped.
	res.publish(t, &Tick{IDValue: 99})

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
	res := &subResolver{}
	parsed := graphql.MustParseSchema(subSDL, res)

	server := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	msg, _ := json.Marshal(map[string]string{"type": "connection_terminate"})
	_ = conn.WriteMessage(websocket.TextMessage, msg)

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
	res := &subResolver{}
	parsed := graphql.MustParseSchema(subSDL, res)

	server := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_ = conn.WriteMessage(websocket.TextMessage, []byte("not json"))
	_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type": "unknown"}`))
	_ = conn.WriteMessage(websocket.TextMessage, []byte(`{}`))

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
	res := &subResolver{}
	parsed := graphql.MustParseSchema(subSDL, res)

	server := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

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

	waitForSubscribers(t, res, 1)

	conn.Close()

	time.Sleep(200 * time.Millisecond)

	// Server should have cleaned up; publishing must not panic.
	res.publish(t, &Tick{IDValue: 1})
}
