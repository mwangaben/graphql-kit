// Package broker provides a generic publish/subscribe broker for
// GraphQL subscriptions.
//
// Each feature owns its own broker instance. The type parameter T
// is the event payload type (typically the GraphQL resolver type).
package broker

// Broker is the interface satisfied by Broker[T].
//
// Defined as an interface so tests can supply fakes and so that
// multiple broker implementations can coexist.
type Broker[T any] interface {
	// Publish sends an event to all subscribers of a topic.
	//
	// If a subscriber's channel is full, the event is dropped for
	// that subscriber only. Publishing never blocks.
	Publish(topic string, event T)

	// Subscribe creates a channel that receives events for a topic.
	//
	// The returned function removes the subscription and closes
	// the channel. Callers MUST call it when the subscription ends.
	Subscribe(topic string) (<-chan T, func())
}
