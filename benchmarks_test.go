package graphqlkit_test

import (
	"strings"
	"testing"

	"github.com/mwangaben/graphql-kit/broker"
	"github.com/mwangaben/graphql-kit/schema"
)

// ============================================================================
// Schema builder benchmarks
// ============================================================================

func BenchmarkSchemaBuilder_Add(b *testing.B) {
	fragment := "extend type Query { foo: String! }"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		builder := schema.NewBuilder()
		builder.Add(fragment)
	}
}

func BenchmarkSchemaBuilder_Add10(b *testing.B) {
	fragments := make([]string, 10)
	for i := range fragments {
		fragments[i] = "extend type Query { foo: String! }"
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		builder := schema.NewBuilder()
		for _, f := range fragments {
			builder.Add(f)
		}
	}
}

func BenchmarkSchemaBuilder_Build(b *testing.B) {
	builder := schema.NewBuilder().
		Add("extend type Query { a: String! }").
		Add("extend type Query { b: String! }").
		Add("extend type Query { c: String! }")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = builder.Build()
	}
}

// ============================================================================
// Broker benchmarks
// ============================================================================

func BenchmarkBroker_Subscribe(b *testing.B) {
	br := broker.New[string]()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, cancel := br.Subscribe("topic")
		cancel()
	}
}

func BenchmarkBroker_Publish_NoSubscribers(b *testing.B) {
	br := broker.New[string]()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		br.Publish("topic", "event")
	}
}

func BenchmarkBroker_Publish_OneSubscriber(b *testing.B) {
	br := broker.New[string]()
	ch, cancel := br.Subscribe("topic")
	defer cancel()

	// Drain channel in background
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		br.Publish("topic", "event")
	}
}

func BenchmarkBroker_Publish_10Subscribers(b *testing.B) {
	br := broker.New[string]()

	cancels := make([]func(), 10)
	for i := 0; i < 10; i++ {
		_, cancel := br.Subscribe("topic")
		cancels[i] = cancel
	}
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		br.Publish("topic", "event")
	}
}

// ============================================================================
// Concurrency
// ============================================================================

func BenchmarkBroker_PublishParallel(b *testing.B) {
	br := broker.New[int]()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			br.Publish("topic", 42)
		}
	})
}

// Ensure we use the strings import
var _ = strings.Contains
