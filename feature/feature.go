// Package feature defines the contract every GraphQL feature must
// satisfy to be composed into a root resolver.
//
// Features contribute:
//   - A name (for diagnostics and topic namespacing)
//   - A GraphQL schema fragment
//   - A resolver (embedded into the root resolver)
//   - Optionally, subscription resolvers and a broker
package feature

// Feature is the minimum contract for a GraphQL feature.
//
// The resolver and subscription resolver are returned as `any`
// because the root resolver uses Go struct embedding to compose
// them — the exact types vary per feature.
//
// Features are typically implemented as a struct that bundles
// the resolver(s), schema, and broker together.
type Feature interface {
	// Name returns the feature's unique identifier.
	//
	// Examples: "user", "post", "comment".
	//
	// The name is used for diagnostics and to namespace
	// subscription topics (e.g., "user.created").
	Name() string

	// Schema returns the feature's GraphQL SDL fragment.
	//
	// The fragment should use `extend type Query/Mutation/
	// Subscription` to add fields to the base schema.
	Schema() string

	// Resolver returns the feature's query/mutation resolver.
	//
	// The returned value is embedded into the root resolver
	// using Go's struct embedding. It must be a pointer to
	// a struct with methods matching the schema fields.
	Resolver() any

	// SubscriptionResolver returns the feature's subscription
	// resolver, or nil if the feature has no subscriptions.
	//
	// Same embedding rules as Resolver().
	SubscriptionResolver() any
}

// Brokered is an optional interface for features that expose
// a subscription broker.
//
// The kit's WebSocket handler can use this to monitor feature
// broker state (e.g., for observability).
type Brokered interface {
	// Broker returns the feature's broker, or nil if it has none.
	//
	// The returned value is `any` because the broker's event
	// type varies per feature.
	Broker() any
}

// Named is an optional interface for features that want to
// customize their namespacing.
//
// Default topic format is "<feature>.<event>".
type Named interface {
	// TopicPrefix returns the prefix used for subscription topics.
	//
	// Defaults to Name() if not implemented.
	TopicPrefix() string
}
