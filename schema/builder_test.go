package schema_test

import (
	"strings"
	"testing"

	"github.com/mwangaben/graphql-kit/schema"
)

// ============================================================================
// NewBuilder
// ============================================================================

func TestNewBuilder_StartsWithBaseSchema(t *testing.T) {
	b := schema.NewBuilder()
	if b == nil {
		t.Fatal("expected non-nil builder")
	}
	// Count excludes base
	if b.Count() != 0 {
		t.Errorf("expected 0 features, got %d", b.Count())
	}
}

// ============================================================================
// Add
// ============================================================================

func TestBuilder_Add_SingleFragment(t *testing.T) {
	b := schema.NewBuilder().
		Add("extend type Query { foo: String! }")

	if b.Count() != 1 {
		t.Errorf("expected 1 feature, got %d", b.Count())
	}

	out := b.Build()
	if !strings.Contains(out, "foo: String!") {
		t.Errorf("expected fragment in output, got %q", out)
	}
}

func TestBuilder_Add_MultipleFragments(t *testing.T) {
	b := schema.NewBuilder().
		Add("extend type Query { a: String! }").
		Add("extend type Query { b: String! }").
		Add("extend type Mutation { c: String! }")

	if b.Count() != 3 {
		t.Errorf("expected 3 features, got %d", b.Count())
	}

	out := b.Build()
	for _, field := range []string{"a: String!", "b: String!", "c: String!"} {
		if !strings.Contains(out, field) {
			t.Errorf("expected %q in output", field)
		}
	}
}

func TestBuilder_Add_Empty(t *testing.T) {
	b := schema.NewBuilder().
		Add("").
		Add("   ").
		Add("\n\t\n")

	if b.Count() != 0 {
		t.Errorf("expected 0 features (empty ignored), got %d", b.Count())
	}
}

func TestBuilder_Add_Chaining(t *testing.T) {
	// Fluent API must return same builder
	b := schema.NewBuilder()
	result := b.Add("a").Add("b").Add("c")
	if result != b {
		t.Error("Add should return the same builder for chaining")
	}
}

// ============================================================================
// Build
// ============================================================================

func TestBuilder_Build_ContainsBaseSchema(t *testing.T) {
	b := schema.NewBuilder()
	out := b.Build()

	// Base schema must be present
	if !strings.Contains(out, "version: String!") {
		t.Error("expected base schema with version field")
	}
	if !strings.Contains(out, "noopMutation: Boolean") {
		t.Error("expected base schema with noopMutation")
	}
	if !strings.Contains(out, "noopSubscription: Boolean") {
		t.Error("expected base schema with noopSubscription")
	}
}

func TestBuilder_Build_ConcatenatesFragments(t *testing.T) {
	frag1 := "extend type Query { first: String! }"
	frag2 := "extend type Query { second: String! }"

	b := schema.NewBuilder().Add(frag1).Add(frag2)
	out := b.Build()

	idx1 := strings.Index(out, frag1)
	idx2 := strings.Index(out, frag2)

	if idx1 == -1 || idx2 == -1 {
		t.Fatal("expected both fragments in output")
	}
	if idx1 > idx2 {
		t.Error("expected fragment order preserved")
	}
}

func TestBuilder_Build_IsDeterministic(t *testing.T) {
	b := schema.NewBuilder().Add("extend type Query { a: String! }")

	first := b.Build()
	second := b.Build()

	if first != second {
		t.Error("Build() should be deterministic")
	}
}

// ============================================================================
// Count
// ============================================================================

func TestBuilder_Count(t *testing.T) {
	tests := []struct {
		name      string
		fragments []string
		want      int
	}{
		{"empty", nil, 0},
		{"one", []string{"a"}, 1},
		{"three", []string{"a", "b", "c"}, 3},
		{"with empty", []string{"a", "", "  ", "b"}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := schema.NewBuilder()
			for _, f := range tt.fragments {
				b.Add(f)
			}
			if got := b.Count(); got != tt.want {
				t.Errorf("Count() = %d, want %d", got, tt.want)
			}
		})
	}
}

// ============================================================================
// Concurrency safety
// ============================================================================

func TestBuilder_IsNotShared(t *testing.T) {
	// Builders should not share state
	b1 := schema.NewBuilder().Add("a")
	b2 := schema.NewBuilder().Add("b")

	if b1.Count() != 1 {
		t.Errorf("b1 count = %d, want 1", b1.Count())
	}
	if b2.Count() != 1 {
		t.Errorf("b2 count = %d, want 1", b2.Count())
	}

	out1 := b1.Build()
	out2 := b2.Build()

	if strings.Contains(out1, "b") && !strings.Contains(out1, "a") {
		t.Error("b1 should contain 'a', not 'b'")
	}
	if strings.Contains(out2, "a") && !strings.Contains(out2, "b") {
		t.Error("b2 should contain 'b', not 'a'")
	}
}
