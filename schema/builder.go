// Package schema provides GraphQL schema composition utilities.
//
// It allows features to contribute fragments that merge cleanly
// using GraphQL's `extend type` syntax.
package schema

import (
	_ "embed"
	"strings"
)

//go:embed base.graphql
var baseSchema string

// Builder combines the base schema with feature schema fragments.
//
// Each feature contributes a schema fragment using `extend type
// Query/Mutation/Subscription`. GraphQL merges them automatically.
//
// Example:
//
//	builder := schema.NewBuilder().
//	    Add(userFeature.Schema).
//	    Add(postFeature.Schema)
//
//	sdl := builder.Build()
//	s := graphql.MustParseSchema(sdl, rootResolver)
type Builder struct {
	schemas []string
}

// NewBuilder creates a builder seeded with the base schema.
func NewBuilder() *Builder {
	return &Builder{
		schemas: []string{baseSchema},
	}
}

// Add appends a feature's schema fragment.
//
// Empty or whitespace-only fragments are ignored. Returns the
// builder for fluent chaining.
func (b *Builder) Add(fragment string) *Builder {
	if strings.TrimSpace(fragment) != "" {
		b.schemas = append(b.schemas, fragment)
	}
	return b
}

// Build returns the combined GraphQL SDL.
func (b *Builder) Build() string {
	return strings.Join(b.schemas, "\n\n")
}

// Count returns the number of feature schemas added (excluding base).
func (b *Builder) Count() int {
	if len(b.schemas) == 0 {
		return 0
	}
	return len(b.schemas) - 1
}
