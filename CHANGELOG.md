# Changelog
## [v0.0.2] - 2026-09-19

### Added
- **WebSocket authentication** — private subscriptions can now require auth
- `Authenticator` interface — pluggable token validation
- `AuthenticatorFunc` — function adapter
- `WithAuthenticator` — configure WS auth
- `WithUserFunc` — inject authenticated user into context
- `WithAllowedOrigins` — restrict WebSocket origins
- Token extraction from `connection_init` payload (protocol-standard)
- Token extraction from `?token=` query parameter (convenient for dev)

### Design
- Auth is optional. Unauthenticated connections are allowed;
  private subscriptions fail with `Unauthenticated`.
- Public subscriptions work without auth.
- The kit is auth-agnostic — apps provide their own Authenticator.

### Compatible With
- `graphql-ws` protocol (as used by Apollo, graph-gophers, etc.)
- Works with any JWT / token mechanism






## [v0.0.1] - 2026-09-17

### Added
- `schema.Builder` — Compose GraphQL schemas from feature fragments
- `schema.Int` — Custom Int scalar mapping to Go's int
- `broker.Safe[T]` — Generic thread-safe pub/sub broker
- `broker.Broker[T]` — Broker interface
- `feature.Feature` — Interface contract for GraphQL features
- `feature.Brokered` — Optional interface for brokers
- `feature.Named` — Optional interface for topic customization
- `handlers.HTTPHandler` — HTTP handler with CORS and Playground
- `handlers.NewWebSocket` — WebSocket handler for subscriptions

### Testing
- 60+ unit tests
- 8 benchmarks
- 5 fuzzers with 10M+ executions
- 88.4% code coverage
- Race-detector clean

### Design
- Zero coupling to consumer applications
- Type-safe composition via Go embedding
- Generic broker over event type T
- Only 2 dependencies: graph-gophers and gorilla/websocket