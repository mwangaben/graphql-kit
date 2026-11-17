# Changelog
## [v0.0.4] - 2026-11-17

### Added
- `graphql-transport-ws` (modern graphql-ws) subprotocol support alongside
  the existing legacy `graphql-ws` (subscriptions-transport-ws) protocol.
  Clients opt in by their library choice; no server configuration needed.
- Spec-compliant close codes on the modern protocol:
  `4400` (bad request), `4409` (duplicate subscriber id), `4429` (too many
  `connection_init`), plus `ping`/`pong` liveness support.

### Changed
- `NewWebSocket` now advertises both subprotocols and negotiates per client
  via `Sec-WebSocket-Protocol`. Clients that send no subprotocol fall back
  to the legacy protocol, preserving backward compatibility.

### Fixed
- Serialized WebSocket writes with a per-connection `writeMu`.
  Multiple `runSubscription` goroutines plus the reader loop (pong, close
  frames) can no longer write to the same socket concurrently, which
  gorilla's `Conn.WriteMessage` forbids. Previously this could panic with
  "concurrent write to websocket connection" under multiple active
  subscriptions, or corrupt frames.

### Tests
- Added `TestWSModern_ConcurrentWrites` and
  `TestWSModern_MultipleSubscriptionsOneConn`, plus ten modern-protocol
  tests covering handshake, lifecycle, close codes, ping/pong, and auth.




## [v0.0.3] - 2026-11-17
## WebSocket Protocol

The WebSocket handler supports **two subprotocols**, negotiated via the
`Sec-WebSocket-Protocol` header:

| Subprotocol            | Client library                                                                              | Status                               |
|------------------------|---------------------------------------------------------------------------------------------|--------------------------------------|
| `graphql-transport-ws` | [`graphql-ws`](https://github.com/enisdenjo/graphql-ws) (JS)                                | Modern, recommended                  |
| `graphql-ws`           | [`subscriptions-transport-ws`](https://github.com/apollographql/subscriptions-transport-ws) | Legacy, maintained for compatibility |

Clients opt in simply by choosing a library — no server-side configuration
is required. If the client sends no subprotocol, the handler falls back to
the legacy protocol so existing deployments keep working unchanged.

Both protocols use the same endpoint (e.g. `/subscriptions`), the same
`connection_init` auth payload, and the same broker/resolver semantics.
Private subscriptions behave identically over either protocol.

### Legacy (`graphql-ws` / subscriptions-transport-ws)

| Client → Server        | Server → Client             |
|------------------------|-----------------------------|
| `connection_init`      | `connection_ack`            |
| `start`                | `data`, `error`, `complete` |
| `stop`                 | —                           |
| `connection_terminate` | —                           |

### Modern (`graphql-transport-ws` / graphql-ws)

| Client → Server   | Server → Client             |
|-------------------|-----------------------------|
| `connection_init` | `connection_ack`            |
| `subscribe`       | `next`, `error`, `complete` |
| `complete`        | —                           |
| `ping`            | `pong`                      |
| `pong`            | `ping` (liveness)           |

The modern protocol also closes with spec-defined codes:

| Code   | Meaning                                                                                  |
|--------|------------------------------------------------------------------------------------------|
| `4400` | Bad request (malformed JSON, unknown message type, `subscribe` before `connection_init`) |
| `4401` | Unauthorized (reserved; the kit does *not* reject on auth failure — see below)           |
| `4409` | Subscriber for this id already exists                                                    |
| `4429` | Too many `connection_init` requests                                                      |

> **Note on `4401`:** the kit keeps its non-fatal auth contract across both
> protocols. Invalid or missing tokens do not close the connection; private
> subscription resolvers return `ErrUnauthenticated` as a GraphQL `error`
> message instead. This preserves the "public subscriptions work anonymously"
> guarantee in both protocols.

### Client examples

**Modern (recommended):**

```ts
import { createClient } from 'graphql-ws';

const ws = createClient({
  url: 'wss://api.example.com/subscriptions',
  connectionParams: () => ({
    Authorization: `Bearer ${localStorage.getItem('access_token')}`,
  }),
});
```

**Legacy:**

```ts
import { SubscriptionClient } from 'subscriptions-transport-ws';

const ws = new SubscriptionClient('wss://api.example.com/subscriptions', {
  reconnect: true,
  connectionParams: () => ({
    Authorization: `Bearer ${localStorage.getItem('access_token')}`,
  }),
});
```





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