package feature_test

import (
	"testing"

	"github.com/mwangaben/graphql-kit/feature"
)

// ============================================================================
// Feature interface contract
// ============================================================================

// testFeature implements Feature.
type testFeature struct {
	name         string
	schema       string
	resolver     any
	subscription any
}

func (f *testFeature) Name() string              { return f.name }
func (f *testFeature) Schema() string            { return f.schema }
func (f *testFeature) Resolver() any             { return f.resolver }
func (f *testFeature) SubscriptionResolver() any { return f.subscription }

// Compile-time check
var _ feature.Feature = (*testFeature)(nil)

func TestFeature_Contract(t *testing.T) {
	f := &testFeature{
		name:   "user",
		schema: "extend type Query { user: String! }",
	}

	if f.Name() != "user" {
		t.Errorf("expected 'user', got %q", f.Name())
	}
	if f.Schema() != "extend type Query { user: String! }" {
		t.Errorf("unexpected schema")
	}
	if f.Resolver() != nil {
		t.Error("expected nil resolver")
	}
	if f.SubscriptionResolver() != nil {
		t.Error("expected nil subscription")
	}
}

// ============================================================================
// Brokered optional interface
// ============================================================================

type brokeredFeature struct {
	testFeature
	broker any
}

func (f *brokeredFeature) Broker() any { return f.broker }

var _ feature.Brokered = (*brokeredFeature)(nil)

func TestBrokered_Optional(t *testing.T) {
	f := &brokeredFeature{broker: "some-broker"}

	// Should satisfy both interfaces
	if f.Name() == "" && f.Broker() == nil {
		t.Error("expected feature to be Brokered")
	}
}

// ============================================================================
// Named optional interface
// ============================================================================

type namedFeature struct {
	testFeature
	prefix string
}

func (f *namedFeature) TopicPrefix() string { return f.prefix }

var _ feature.Named = (*namedFeature)(nil)

func TestNamed_Optional(t *testing.T) {
	f := &namedFeature{prefix: "usr"}
	if f.TopicPrefix() != "usr" {
		t.Errorf("expected 'usr', got %q", f.TopicPrefix())
	}
}

// ============================================================================
// Interface assertions — verify plain Feature does not satisfy optional ones
// ============================================================================

func TestFeature_NotBrokered(t *testing.T) {
	var f feature.Feature = &testFeature{name: "x"}

	if _, ok := f.(feature.Brokered); ok {
		t.Error("plain testFeature should not satisfy Brokered")
	}
	if _, ok := f.(feature.Named); ok {
		t.Error("plain testFeature should not satisfy Named")
	}
}
