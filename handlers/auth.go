package handlers

import "context"

// Authenticator validates a token and returns the authenticated user.
//
// The kit doesn't know about JWTs, users, or permissions. It only
// needs something that turns a token string into a value that gets
// stored in the connection context.
//
// The returned value is opaque to the kit — it's injected into the
// subscription context using the user-provided WithUserFunc, then
// read by the app's resolvers.
//
// Return values:
//   - (user, nil)      → authenticated; user stored in context
//   - (nil, err)       → unauthenticated; connection proceeds without user
//
// The kit does not reject connections on auth failure. This allows
// public subscriptions to work over unauthenticated connections,
// while private subscriptions fail naturally when the resolver
// checks for a user.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (any, error)
}

// AuthenticatorFunc adapts a plain function to the Authenticator interface.
//
// Example:
//
//	auth := handlers.AuthenticatorFunc(func(ctx context.Context, token string) (any, error) {
//	    return myPassport.ValidateToken(ctx, token)
//	})
type AuthenticatorFunc func(ctx context.Context, token string) (any, error)

// Authenticate implements Authenticator.
func (f AuthenticatorFunc) Authenticate(ctx context.Context, token string) (any, error) {
	return f(ctx, token)
}
