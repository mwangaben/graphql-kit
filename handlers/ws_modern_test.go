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

// Modern-protocol dialer: advertise the graphql-transport-ws subprotocol.
func dialModern(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()
	u := "ws" + strings.TrimPrefix(serverURL, "http")
	d := websocket.Dialer{
		Subprotocols: []string{"graphql-transport-ws"},
	}
	conn, resp, err := d.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if got := resp.Header.Get("Sec-WebSocket-Protocol"); got != "graphql-transport-ws" {
		t.Fatalf("expected negotiated subprotocol graphql-transport-ws, got %q", got)
	}
	return conn
}

func readMsg(t *testing.T, conn *websocket.Conn) map[string]interface{} {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(msg, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

// ============================================================================
// Handshake
// ============================================================================

func TestWSModern_ConnectionInit(t *testing.T) {
	parsed := graphql.MustParseSchema(subSDL, &subResolver{})
	srv := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer srv.Close()

	conn := dialModern(t, srv.URL)
	defer conn.Close()

	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)

	ack := readMsg(t, conn)
	if ack["type"] != "connection_ack" {
		t.Fatalf("expected connection_ack, got %v", ack["type"])
	}
}

func TestWSModern_InitRequiredFirst(t *testing.T) {
	parsed := graphql.MustParseSchema(subSDL, &subResolver{})
	srv := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer srv.Close()

	conn := dialModern(t, srv.URL)
	defer conn.Close()

	sub, _ := json.Marshal(map[string]interface{}{
		"id":      "1",
		"type":    "subscribe",
		"payload": map[string]string{"query": "subscription { tick { id } }"},
	})
	_ = conn.WriteMessage(websocket.TextMessage, sub)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatal("expected close after subscribe-before-init")
	}
	if cerr, ok := err.(*websocket.CloseError); !ok || cerr.Code != 4400 {
		t.Fatalf("expected close code 4400, got %v", err)
	}
}

func TestWSModern_DoubleInitCloses(t *testing.T) {
	parsed := graphql.MustParseSchema(subSDL, &subResolver{})
	srv := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer srv.Close()

	conn := dialModern(t, srv.URL)
	defer conn.Close()

	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)
	_ = readMsg(t, conn) // ack

	_ = conn.WriteMessage(websocket.TextMessage, init) // second init
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	if cerr, ok := err.(*websocket.CloseError); !ok || cerr.Code != 4429 {
		t.Fatalf("expected close code 4429, got %v", err)
	}
}

// ============================================================================
// Subscription flow (subscribe → next → complete)
// ============================================================================

func TestWSModern_SubscriptionFlow(t *testing.T) {
	res := &subResolver{}
	parsed := graphql.MustParseSchema(subSDL, res)
	srv := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer srv.Close()

	conn := dialModern(t, srv.URL)
	defer conn.Close()

	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)
	_ = readMsg(t, conn) // ack

	sub, _ := json.Marshal(map[string]interface{}{
		"id":      "1",
		"type":    "subscribe",
		"payload": map[string]interface{}{"query": "subscription { tick { id } }"},
	})
	_ = conn.WriteMessage(websocket.TextMessage, sub)

	waitForSubscribers(t, res, 1)
	res.publish(t, &Tick{IDValue: 42})

	m := readMsg(t, conn)
	if m["type"] != "next" {
		t.Fatalf("expected type=next, got %v", m["type"])
	}
	if m["id"] != "1" {
		t.Fatalf("expected id=1, got %v", m["id"])
	}
	payload := m["payload"].(map[string]interface{})
	data := payload["data"].(map[string]interface{})
	tick := data["tick"].(map[string]interface{})
	if tick["id"].(float64) != 42 {
		t.Fatalf("expected id=42, got %v", tick["id"])
	}
}

// ============================================================================
// Client-initiated complete
// ============================================================================

func TestWSModern_CompleteStopsSubscription(t *testing.T) {
	res := &subResolver{}
	parsed := graphql.MustParseSchema(subSDL, res)
	srv := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer srv.Close()

	conn := dialModern(t, srv.URL)
	defer conn.Close()

	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)
	_ = readMsg(t, conn)

	sub, _ := json.Marshal(map[string]interface{}{
		"id":      "1",
		"type":    "subscribe",
		"payload": map[string]interface{}{"query": "subscription { tick { id } }"},
	})
	_ = conn.WriteMessage(websocket.TextMessage, sub)

	waitForSubscribers(t, res, 1)

	complete, _ := json.Marshal(map[string]string{"id": "1", "type": "complete"})
	_ = conn.WriteMessage(websocket.TextMessage, complete)

	time.Sleep(100 * time.Millisecond)
	res.publish(t, &Tick{IDValue: 99})

	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected no message after client complete")
	}
}

// ============================================================================
// Duplicate subscriber id closes with 4409
// ============================================================================

func TestWSModern_DuplicateSubscriberIDCloses(t *testing.T) {
	parsed := graphql.MustParseSchema(subSDL, &subResolver{})
	srv := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer srv.Close()

	conn := dialModern(t, srv.URL)
	defer conn.Close()

	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)
	_ = readMsg(t, conn)

	mkSub := func(id string) []byte {
		b, _ := json.Marshal(map[string]interface{}{
			"id":      id,
			"type":    "subscribe",
			"payload": map[string]interface{}{"query": "subscription { tick { id } }"},
		})
		return b
	}
	_ = conn.WriteMessage(websocket.TextMessage, mkSub("1"))
	_ = conn.WriteMessage(websocket.TextMessage, mkSub("1")) // duplicate

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	if cerr, ok := err.(*websocket.CloseError); !ok || cerr.Code != 4409 {
		t.Fatalf("expected close code 4409, got %v", err)
	}
}

// ============================================================================
// ping → pong
// ============================================================================

func TestWSModern_PingPong(t *testing.T) {
	parsed := graphql.MustParseSchema(subSDL, &subResolver{})
	srv := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer srv.Close()

	conn := dialModern(t, srv.URL)
	defer conn.Close()

	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)
	_ = readMsg(t, conn)

	ping, _ := json.Marshal(map[string]interface{}{"type": "ping", "payload": map[string]string{"x": "y"}})
	_ = conn.WriteMessage(websocket.TextMessage, ping)

	m := readMsg(t, conn)
	if m["type"] != "pong" {
		t.Fatalf("expected pong, got %v", m["type"])
	}
	payload := m["payload"].(map[string]interface{})
	if payload["x"] != "y" {
		t.Fatalf("expected echoed payload, got %v", payload)
	}
}

// ============================================================================
// Unknown message type closes with 4400
// ============================================================================

func TestWSModern_UnknownMessageCloses(t *testing.T) {
	parsed := graphql.MustParseSchema(subSDL, &subResolver{})
	srv := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer srv.Close()

	conn := dialModern(t, srv.URL)
	defer conn.Close()

	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)
	_ = readMsg(t, conn)

	bad, _ := json.Marshal(map[string]string{"type": "bogus"})
	_ = conn.WriteMessage(websocket.TextMessage, bad)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	if cerr, ok := err.(*websocket.CloseError); !ok || cerr.Code != 4400 {
		t.Fatalf("expected close code 4400, got %v", err)
	}
}

// ============================================================================
// Auth via connection_init payload works on modern protocol
// ============================================================================

type staticAuth struct{ user any }

func (a staticAuth) Authenticate(ctx context.Context, token string) (any, error) {
	if token == "good" {
		return a.user, nil
	}
	return nil, wsTestError("bad token")
}

type wsTestError string

func (e wsTestError) Error() string { return string(e) }

func TestWSModern_AuthPayloadInjected(t *testing.T) {
	res := &subResolver{}
	parsed := graphql.MustParseSchema(subSDL, res)

	type ctxKey struct{}
	userFunc := func(ctx context.Context, u any) context.Context {
		return context.WithValue(ctx, ctxKey{}, u)
	}

	srv := httptest.NewServer(handlers.NewWebSocket(parsed,
		handlers.WithAuthenticator(staticAuth{user: "alice"}),
		handlers.WithUserFunc(userFunc),
	))
	defer srv.Close()

	conn := dialModern(t, srv.URL)
	defer conn.Close()

	init, _ := json.Marshal(map[string]interface{}{
		"type":    "connection_init",
		"payload": map[string]string{"Authorization": "Bearer good"},
	})
	_ = conn.WriteMessage(websocket.TextMessage, init)
	_ = readMsg(t, conn) // ack

	sub, _ := json.Marshal(map[string]interface{}{
		"id":      "1",
		"type":    "subscribe",
		"payload": map[string]interface{}{"query": "subscription { tick { id } }"},
	})
	_ = conn.WriteMessage(websocket.TextMessage, sub)

	waitForSubscribers(t, res, 1)
	res.publish(t, &Tick{IDValue: 7})

	m := readMsg(t, conn)
	if m["type"] != "next" {
		t.Fatalf("expected next, got %v", m["type"])
	}
}

// ============================================================================
// Legacy client still works (regression)
// ============================================================================

func TestWSLegacy_StillNegotiatesLegacy(t *testing.T) {
	parsed := graphql.MustParseSchema(subSDL, &subResolver{})
	srv := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer srv.Close()

	u := "ws" + strings.TrimPrefix(srv.URL, "http")
	d := websocket.Dialer{Subprotocols: []string{"graphql-ws"}}
	conn, resp, err := d.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if got := resp.Header.Get("Sec-WebSocket-Protocol"); got != "graphql-ws" {
		t.Fatalf("expected legacy subprotocol, got %q", got)
	}
}

// ============================================================================
// Concurrency: many subscription frames + control frames must not race
// ============================================================================

func TestWSModern_ConcurrentWrites(t *testing.T) {
	res := &subResolver{}
	parsed := graphql.MustParseSchema(subSDL, res)
	srv := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer srv.Close()

	conn := dialModern(t, srv.URL)
	defer conn.Close()

	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)
	_ = readMsg(t, conn) // ack

	sub, _ := json.Marshal(map[string]interface{}{
		"id":      "1",
		"type":    "subscribe",
		"payload": map[string]interface{}{"query": "subscription { tick { id } }"},
	})
	_ = conn.WriteMessage(websocket.TextMessage, sub)

	waitForSubscribers(t, res, 1)

	// Use n+8 so the reader definitely sees more than n messages if any
	// are retried, without masking drops. The buffer per subscription is
	// sized to n in Tick(), so publishes never block on a live subscriber.
	const n = 100
	go func() {
		for i := 0; i < n; i++ {
			res.publish(t, &Tick{IDValue: int32(i)})
		}
	}()

	ping, _ := json.Marshal(map[string]string{"type": "ping"})

	type result struct {
		nextCount int
		err       error
	}
	resCh := make(chan result, 1)
	go func() {
		var r result
		deadline := time.Now().Add(10 * time.Second)
		for r.nextCount < n {
			_ = conn.SetReadDeadline(deadline)
			_, raw, err := conn.ReadMessage()
			if err != nil {
				r.err = err
				resCh <- r
				return
			}
			var m map[string]interface{}
			if err := json.Unmarshal(raw, &m); err != nil {
				r.err = err
				resCh <- r
				return
			}
			if m["type"] == "next" {
				r.nextCount++
			}
		}
		resCh <- r
	}()

	// Interleave pings so writeMu is exercised from two writers:
	// the reader loop (pong replies) and runSubscription (next frames).
	for i := 0; i < 20; i++ {
		_ = conn.WriteMessage(websocket.TextMessage, ping)
		time.Sleep(5 * time.Millisecond)
	}

	select {
	case r := <-resCh:
		if r.err != nil {
			t.Fatalf("read error after %d frames: %v", r.nextCount, r.err)
		}
		if r.nextCount != n {
			t.Fatalf("expected %d next frames, got %d", n, r.nextCount)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for next frames")
	}
}

// Two subscriptions on one connection: each event must reach both, and
// both runSubscription goroutines must share writeMu without interleaving.
func TestWSModern_MultipleSubscriptionsOneConn(t *testing.T) {
	res := &subResolver{}
	parsed := graphql.MustParseSchema(subSDL, res)
	srv := httptest.NewServer(handlers.NewWebSocket(parsed))
	defer srv.Close()

	conn := dialModern(t, srv.URL)
	defer conn.Close()

	init, _ := json.Marshal(map[string]string{"type": "connection_init"})
	_ = conn.WriteMessage(websocket.TextMessage, init)
	_ = readMsg(t, conn)

	for _, id := range []string{"a", "b"} {
		sub, _ := json.Marshal(map[string]interface{}{
			"id":      id,
			"type":    "subscribe",
			"payload": map[string]interface{}{"query": "subscription { tick { id } }"},
		})
		_ = conn.WriteMessage(websocket.TextMessage, sub)
	}

	waitForSubscribers(t, res, 2)

	const n = 50
	// Blocking publishes: each fan-out waits for both subscribers to accept
	// the event before returning, so nothing is dropped.
	for i := 0; i < n; i++ {
		res.publish(t, &Tick{IDValue: int32(i)})
	}

	deadline := time.Now().Add(10 * time.Second)
	seen := map[string]int{}
	for total := 0; total < 2*n; {
		_ = conn.SetReadDeadline(deadline)
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read after %d frames: %v (a=%d b=%d)",
				total, err, seen["a"], seen["b"])
		}
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if m["type"] == "next" {
			id, _ := m["id"].(string)
			seen[id]++
			total++
		}
	}

	if seen["a"] != n || seen["b"] != n {
		t.Fatalf("expected %d per subscription, got a=%d b=%d", n, seen["a"], seen["b"])
	}
}
