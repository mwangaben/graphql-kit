package graphqlkit_test

import (
	"strings"
	"testing"

	"github.com/mwangaben/graphql-kit/broker"
	"github.com/mwangaben/graphql-kit/schema"
)

// ============================================================================
// Schema builder fuzzers
// ============================================================================

func FuzzSchemaBuilder_Add(f *testing.F) {
	f.Add("extend type Query { foo: String! }")
	f.Add("")
	f.Add("   ")
	f.Add("\n\t")
	f.Add(strings.Repeat("x", 10000))
	f.Add("type Query { a: String! }")
	f.Add("extend type Query { a: String! b: Int! }")

	f.Fuzz(func(t *testing.T, fragment string) {
		builder := schema.NewBuilder()
		builder.Add(fragment)

		// Should not panic
		out := builder.Build()

		// Non-empty fragments should appear in output
		if strings.TrimSpace(fragment) != "" {
			if !strings.Contains(out, fragment) {
				t.Errorf("output should contain non-empty fragment")
			}
			if builder.Count() != 1 {
				t.Errorf("expected count 1, got %d", builder.Count())
			}
		} else {
			if builder.Count() != 0 {
				t.Errorf("expected count 0 for empty, got %d", builder.Count())
			}
		}
	})
}

func FuzzSchemaBuilder_ManyFragments(f *testing.F) {
	f.Add("a", "b", "c")
	f.Add("", "", "")
	f.Add("same", "same", "same")

	f.Fuzz(func(t *testing.T, f1, f2, f3 string) {
		builder := schema.NewBuilder().
			Add(f1).
			Add(f2).
			Add(f3)

		out := builder.Build()

		// Should not panic, output should be a string
		if out == "" {
			t.Error("Build() should always return at least the base schema")
		}

		// Count should reflect non-empty fragments only
		expectedCount := 0
		for _, f := range []string{f1, f2, f3} {
			if strings.TrimSpace(f) != "" {
				expectedCount++
			}
		}
		if builder.Count() != expectedCount {
			t.Errorf("expected count %d, got %d", expectedCount, builder.Count())
		}
	})
}

// ============================================================================
// Broker fuzzers
// ============================================================================

func FuzzBroker_TopicNames(f *testing.F) {
	f.Add("topic")
	f.Add("")
	f.Add("with spaces")
	f.Add("with/slashes")
	f.Add("with!@#$%^&*()")
	f.Add(strings.Repeat("x", 1000))

	f.Fuzz(func(t *testing.T, topic string) {
		br := broker.New[string]()

		ch, cancel := br.Subscribe(topic)
		defer cancel()

		br.Publish(topic, "event")

		// Should not panic
		select {
		case got := <-ch:
			if got != "event" {
				t.Errorf("expected 'event', got %q", got)
			}
		default:
			// OK — event may still be in flight
		}
	})
}

func FuzzBroker_EventValues(f *testing.F) {
	f.Add("hello")
	f.Add("")
	f.Add(strings.Repeat("x", 10000))
	f.Add("with\nnewlines\ttabs")

	f.Fuzz(func(t *testing.T, value string) {
		br := broker.New[string]()
		ch, cancel := br.Subscribe("topic")
		defer cancel()

		br.Publish("topic", value)

		select {
		case got := <-ch:
			if got != value {
				t.Errorf("expected %q, got %q", value, got)
			}
		default:
			// OK
		}
	})
}

// ============================================================================
// Int scalar fuzzers
// ============================================================================

func FuzzScalar_Int_Unmarshal(f *testing.F) {
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(-1))
	f.Add(int64(9223372036854775807))
	f.Add(int64(-9223372036854775808))

	f.Fuzz(func(t *testing.T, i int64) {
		var s schema.Int
		// Should not panic
		_ = s.UnmarshalGraphQL(i)

		// Marshal should return the value
		got := s.MarshalGraphQL()
		if _, ok := got.(int); !ok {
			t.Errorf("MarshalGraphQL should return int, got %T", got)
		}
	})
}

func FuzzScalar_Int_StringInput(f *testing.F) {
	f.Add("42")
	f.Add("")
	f.Add("-1")
	f.Add("not a number")
	f.Add("42.5")
	f.Add("999999999999999999999999999999")

	f.Fuzz(func(t *testing.T, input string) {
		var s schema.Int
		err := s.UnmarshalGraphQL(input)

		// Should not panic either way
		if err != nil {
			// Non-numeric strings return error — expected
			return
		}
	})
}
