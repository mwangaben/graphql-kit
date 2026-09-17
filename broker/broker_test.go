package broker_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mwangaben/graphql-kit/broker"
)

// ============================================================================
// Basic pub/sub
// ============================================================================

func TestBroker_SubscribePublish(t *testing.T) {
	b := broker.New[string]()
	ch, cancel := b.Subscribe("topic")
	defer cancel()

	b.Publish("topic", "hello")

	select {
	case got := <-ch:
		if got != "hello" {
			t.Errorf("expected 'hello', got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestBroker_MultipleSubscribers(t *testing.T) {
	b := broker.New[int]()
	ch1, c1 := b.Subscribe("topic")
	ch2, c2 := b.Subscribe("topic")
	defer c1()
	defer c2()

	b.Publish("topic", 42)

	for i, ch := range []<-chan int{ch1, ch2} {
		select {
		case got := <-ch:
			if got != 42 {
				t.Errorf("subscriber %d: expected 42, got %d", i, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timeout", i)
		}
	}
}

func TestBroker_NoSubscribers(t *testing.T) {
	b := broker.New[string]()
	// Should not panic
	b.Publish("topic", "hello")
}

func TestBroker_PublishToCorrectTopic(t *testing.T) {
	b := broker.New[string]()
	chA, cA := b.Subscribe("topic-a")
	chB, cB := b.Subscribe("topic-b")
	defer cA()
	defer cB()

	b.Publish("topic-a", "for-A")

	select {
	case got := <-chA:
		if got != "for-A" {
			t.Errorf("expected 'for-A', got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout on channel A")
	}

	select {
	case got := <-chB:
		t.Errorf("channel B should not receive, got %q", got)
	case <-time.After(50 * time.Millisecond):
		// Good — B got nothing
	}
}

// ============================================================================
// Cancel
// ============================================================================

func TestBroker_CancelClosesChannel(t *testing.T) {
	b := broker.New[string]()
	ch, cancel := b.Subscribe("topic")

	cancel()

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}

func TestBroker_CancelIsIdempotent(t *testing.T) {
	b := broker.New[string]()
	_, cancel := b.Subscribe("topic")

	// Should not panic on double-cancel
	cancel()
	cancel()
}

func TestBroker_CancelOnlyAffectsOwnSubscription(t *testing.T) {
	b := broker.New[string]()
	ch1, c1 := b.Subscribe("topic")
	ch2, c2 := b.Subscribe("topic")
	defer c2()

	c1() // cancel only the first

	b.Publish("topic", "hello")

	// ch1 should be closed
	select {
	case _, ok := <-ch1:
		if ok {
			t.Error("ch1 should be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout on ch1")
	}

	// ch2 should still work
	select {
	case got := <-ch2:
		if got != "hello" {
			t.Errorf("expected 'hello', got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("ch2 should have received")
	}
}

// ============================================================================
// Non-blocking publish
// ============================================================================

func TestBroker_SlowSubscriberDropsEvents(t *testing.T) {
	b := broker.New[int]()
	ch, cancel := b.Subscribe("topic")
	defer cancel()

	// Publish 100 events but never read
	for i := 0; i < 100; i++ {
		b.Publish("topic", i)
	}

	// Should not have blocked
	// Read what's in the buffer (capacity is 10)
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count > 20 {
		t.Errorf("expected ~10 buffered events, got %d (drop not working?)", count)
	}
}

// ============================================================================
// Observable methods
// ============================================================================

func TestBroker_SubscriberCount(t *testing.T) {
	b := broker.New[string]()

	if got := b.SubscriberCount("topic"); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}

	_, c1 := b.Subscribe("topic")
	_, c2 := b.Subscribe("topic")
	defer c1()
	defer c2()

	if got := b.SubscriberCount("topic"); got != 2 {
		t.Errorf("expected 2, got %d", got)
	}

	c1()
	if got := b.SubscriberCount("topic"); got != 1 {
		t.Errorf("expected 1 after cancel, got %d", got)
	}
}

func TestBroker_Topics(t *testing.T) {
	b := broker.New[string]()

	_, c1 := b.Subscribe("a")
	_, c2 := b.Subscribe("b")
	_, c3 := b.Subscribe("a") // duplicate topic
	defer c1()
	defer c2()
	defer c3()

	topics := b.Topics()
	if len(topics) != 2 {
		t.Errorf("expected 2 unique topics, got %d: %v", len(topics), topics)
	}
}

// ============================================================================
// Concurrency
// ============================================================================

func TestBroker_ConcurrentPublish(t *testing.T) {
	b := broker.New[int]()
	ch, cancel := b.Subscribe("topic")
	defer cancel()

	var wg sync.WaitGroup
	publishers := 10
	perPublisher := 100

	for i := 0; i < publishers; i++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := 0; j < perPublisher; j++ {
				b.Publish("topic", base*1000+j)
			}
		}(i)
	}

	// Drain channel
	received := int64(0)
	done := make(chan struct{})
	go func() {
		for range ch {
			atomic.AddInt64(&received, 1)
		}
		close(done)
	}()

	wg.Wait()
	cancel()
	<-done

	// We won't receive all 1000 (some dropped), but should receive > 0
	if received == 0 {
		t.Error("expected to receive at least some events")
	}
}

func TestBroker_ConcurrentSubscribe(t *testing.T) {
	b := broker.New[string]()

	var wg sync.WaitGroup
	cancels := make([]func(), 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, cancel := b.Subscribe("topic")
			cancels[idx] = cancel
		}(i)
	}
	wg.Wait()

	if got := b.SubscriberCount("topic"); got != 100 {
		t.Errorf("expected 100 subscribers, got %d", got)
	}

	for _, c := range cancels {
		c()
	}

	if got := b.SubscriberCount("topic"); got != 0 {
		t.Errorf("expected 0 subscribers after cancel, got %d", got)
	}
}

// ============================================================================
// Type parameter support
// ============================================================================

func TestBroker_WorksWithStructs(t *testing.T) {
	type Event struct {
		ID   int
		Name string
	}

	b := broker.New[Event]()
	ch, cancel := b.Subscribe("events")
	defer cancel()

	expected := Event{ID: 1, Name: "test"}
	b.Publish("events", expected)

	select {
	case got := <-ch:
		if got != expected {
			t.Errorf("expected %+v, got %+v", expected, got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestBroker_WorksWithPointers(t *testing.T) {
	type Event struct{ ID int }

	b := broker.New[*Event]()
	ch, cancel := b.Subscribe("events")
	defer cancel()

	expected := &Event{ID: 42}
	b.Publish("events", expected)

	select {
	case got := <-ch:
		if got != expected {
			t.Error("expected same pointer")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}
