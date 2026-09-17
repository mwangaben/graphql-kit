package schema_test

import (
	"errors"
	"testing"

	"github.com/mwangaben/graphql-kit/schema"
)

// ============================================================================
// ImplementsGraphQLType
// ============================================================================

func TestInt_ImplementsGraphQLType(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Int", true},
		{"int", false},
		{"ID", false},
		{"String", false},
		{"", false},
	}

	var i schema.Int
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := i.ImplementsGraphQLType(tt.name); got != tt.want {
				t.Errorf("ImplementsGraphQLType(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// ============================================================================
// UnmarshalGraphQL — success cases
// ============================================================================

func TestInt_UnmarshalGraphQL_Success(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  schema.Int
	}{
		{"int", 42, 42},
		{"int_zero", 0, 0},
		{"int_negative", -1, -1},
		{"int32", int32(42), 42},
		{"int64", int64(42), 42},
		{"float64_integer", float64(42), 42},
		{"float64_truncated", float64(42.9), 42},
		{"string_numeric", "42", 42},
		{"string_negative", "-42", -42},
		{"string_zero", "0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var i schema.Int
			if err := i.UnmarshalGraphQL(tt.input); err != nil {
				t.Fatalf("UnmarshalGraphQL(%v) error: %v", tt.input, err)
			}
			if i != tt.want {
				t.Errorf("UnmarshalGraphQL(%v) = %d, want %d", tt.input, i, tt.want)
			}
		})
	}
}

// ============================================================================
// UnmarshalGraphQL — error cases
// ============================================================================

func TestInt_UnmarshalGraphQL_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
	}{
		{"string_non_numeric", "not-a-number"},
		{"string_empty", ""},
		{"string_float", "42.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var i schema.Int
			err := i.UnmarshalGraphQL(tt.input)
			if err == nil {
				t.Errorf("expected error for %v, got nil", tt.input)
			}
		})
	}
}

// ============================================================================
// UnmarshalGraphQL — unsupported types (return nil per spec)
// ============================================================================

func TestInt_UnmarshalGraphQL_UnsupportedType(t *testing.T) {
	var i schema.Int
	// Passing a bool — the default branch returns nil
	if err := i.UnmarshalGraphQL(true); err != nil {
		t.Errorf("expected nil error for unsupported type, got %v", err)
	}
	// Value unchanged
	if i != 0 {
		t.Errorf("expected unchanged value 0, got %d", i)
	}
}

// ============================================================================
// MarshalGraphQL
// ============================================================================

func TestInt_MarshalGraphQL(t *testing.T) {
	tests := []struct {
		input schema.Int
		want  int
	}{
		{0, 0},
		{42, 42},
		{-1, -1},
		{1 << 30, 1 << 30},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := tt.input.MarshalGraphQL()
			gotInt, ok := got.(int)
			if !ok {
				t.Fatalf("expected int, got %T", got)
			}
			if gotInt != tt.want {
				t.Errorf("MarshalGraphQL() = %d, want %d", gotInt, tt.want)
			}
		})
	}
}

// Ensure we're compatible with error interface for wrapped errors.
func TestInt_UnmarshalGraphQL_ErrorIsNotNil(t *testing.T) {
	var i schema.Int
	err := i.UnmarshalGraphQL("bad")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, nil) {
		t.Error("error should not be nil")
	}
}
