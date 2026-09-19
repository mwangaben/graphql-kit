package handlers

import "context"

// WSOptions configures the WebSocket handler.
type WSOptions struct {
	// Authenticator validates tokens from connection_init payloads
	// and/or the ?token= query parameter.
	//
	// If nil, all connections are treated as unauthenticated.
	Authenticator Authenticator

	// WithUserFunc injects the authenticated user into the context.
	//
	// This is how the kit bridges its internal "user any" value to
	// the app's context mechanism (e.g., appctx.WithUser).
	//
	// If nil, the user is NOT injected into context — private
	// subscriptions will see no user and return Unauthenticated.
	WithUserFunc func(ctx context.Context, user any) context.Context

	// AllowedOrigins restricts WebSocket origins. Empty means allow all.
	//
	// In production, set this to your frontend's origin(s).
	// Example: []string{"https://app.example.com"}
	AllowedOrigins []string
}

// WithAuthenticator configures the WS authenticator.
func WithAuthenticator(a Authenticator) func(*WSOptions) {
	return func(o *WSOptions) { o.Authenticator = a }
}

// WithUserFunc configures the context injector.
func WithUserFunc(fn func(ctx context.Context, user any) context.Context) func(*WSOptions) {
	return func(o *WSOptions) { o.WithUserFunc = fn }
}

// WithAllowedOrigins restricts WebSocket origins.
func WithAllowedOrigins(origins ...string) func(*WSOptions) {
	return func(o *WSOptions) { o.AllowedOrigins = origins }
}
