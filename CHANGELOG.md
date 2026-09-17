# Changelog

## [0.1.0] - 2026-09-17

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