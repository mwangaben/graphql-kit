package broker

import "sync"

// Safe is a thread-safe implementation of Broker[T].
//
// It uses a mutex to protect the subscriber map and supports
// multiple concurrent publishers and subscribers.
//
// The zero value is not usable; use New[T]().
type Safe[T any] struct {
	mu          sync.RWMutex
	subscribers map[string][]chan T
}

// New creates a new broker.
func New[T any]() *Safe[T] {
	return &Safe[T]{
		subscribers: make(map[string][]chan T),
	}
}

// Publish sends an event to all subscribers of a topic.
//
// Each subscriber gets the event on its own buffered channel. If
// the channel is full, the event is dropped for that subscriber.
// Publishing never blocks the caller.
func (b *Safe[T]) Publish(topic string, event T) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers[topic] {
		select {
		case ch <- event:
		default:
			// Subscriber's buffer is full — drop the event.
			// This prevents a slow subscriber from blocking publishers.
		}
	}
}

// Subscribe creates a channel that receives events for a topic.
//
// The returned channel is buffered (capacity 10). The returned
// cancel function removes the subscription and closes the channel.
//
// Example:
//
//	ch, cancel := broker.Subscribe("user.created")
//	defer cancel()
//	for user := range ch {
//	    // handle user
//	}
func (b *Safe[T]) Subscribe(topic string) (<-chan T, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan T, 10)
	b.subscribers[topic] = append(b.subscribers[topic], ch)

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		subs := b.subscribers[topic]
		for i, sub := range subs {
			if sub == ch {
				b.subscribers[topic] = append(subs[:i], subs[i+1:]...)
				close(ch)
				return
			}
		}
	}

	return ch, cancel
}

// SubscriberCount returns the number of active subscribers for a topic.
// Useful for debugging and tests.
func (b *Safe[T]) SubscriberCount(topic string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers[topic])
}

// Topics returns all topics that have at least one subscriber.
func (b *Safe[T]) Topics() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	topics := make([]string, 0, len(b.subscribers))
	for topic, subs := range b.subscribers {
		if len(subs) > 0 {
			topics = append(topics, topic)
		}
	}
	return topics
}
