# graphql-kit

Composable GraphQL infrastructure for Go.

## Features

- 🎯 **Feature-based schema composition** — each feature owns its schema fragment
- 🔌 **Interface-driven** — features satisfy contracts, kit provides infrastructure
- 📦 **Generic broker** — pub/sub for subscriptions, typed per feature
- 🌐 **HTTP + WebSocket handlers** — ready for queries, mutations, subscriptions
- 🔐 **Optional WebSocket auth** — pluggable token validation for private subscriptions
- 🎨 **GraphQL Playground** — built-in UI for development, token-aware
- ⚡ **Zero reflection at runtime** — type-safe composition via Go embedding
- 🧩 **Zero coupling to apps** — no imports of consumer code

## Installation

```bash
go get github.com/mwangaben/graphql-kit
```

## Quick Start

### 1. Write a feature

```go
// feature/user/resolvers/schema.go
package resolvers

import _ "embed"

//go:embed schema.graphql
var Schema string
```

```graphql
# feature/user/resolvers/schema.graphql
type User {
    id: Int!
    name: String!
}

extend type Query {
    user(id: Int!): User
    users: [User!]!
}
```

```go
// feature/user/resolvers/user_resolver.go
package resolvers

type UserResolver struct{ /* ... */ }

func (r *UserResolver) User(ctx context.Context, args struct{ ID int32 }) (*User, error) {
    // ...
}

func (r *UserResolver) Users(ctx context.Context) ([]*User, error) {
    // ...
}
```

```go
// feature/user/register.go
package user

type Feature struct {
    Resolver *resolvers.UserResolver
    Schema   string
}

func (f *Feature) Name() string               { return "user" }
func (f *Feature) Schema() string             { return f.Schema }
func (f *Feature) Resolver() any              { return f.Resolver }
func (f *Feature) SubscriptionResolver() any  { return nil }
```

### 2. Compose the schema

```go
import (
    "github.com/graph-gophers/graphql-go"
    "github.com/mwangaben/graphql-kit/schema"
)

sdl := schema.NewBuilder().
    Add(userFeature.Schema()).
    Add(postFeature.Schema()).
    Build()

parsed := graphql.MustParseSchema(sdl, rootResolver)
```

### 3. Wire up handlers

```go
import "github.com/mwangaben/graphql-kit/handlers"

// Queries and mutations
http.Handle("/graphql", handlers.NewHTTP(parsed))

// Subscriptions (no auth)
http.HandleFunc("/subscriptions", handlers.NewWebSocket(parsed))
```

### 4. Create a broker for a feature

```go
import "github.com/mwangaben/graphql-kit/broker"

type UserEventBroker = broker.Safe[*User]

func NewUserEventBroker() *UserEventBroker {
    return broker.New[*User]()
}
```

## Package Structure

```text
graphql-kit/
├── schema/       — SchemaBuilder + base schema + Int scalar
├── broker/       — Generic pub/sub broker
├── feature/      — Feature interface contract
└── handlers/     — HTTP + WebSocket handlers
```

## Feature Contract

A feature must implement `feature.Feature`:

```go
type Feature interface {
    Name() string
    Schema() string
    Resolver() any
    SubscriptionResolver() any
}
```

Optional interfaces:

- `feature.Brokered` — if the feature exposes a broker
- `feature.Named` — if the feature needs custom topic namespacing

## Schema Composition

Features use `extend type` to add fields:

```graphql
# feature/a/schema.graphql
extend type Query {
    aField: String!
}
```

```graphql
# feature/b/schema.graphql
extend type Query {
    bField: String!
}
```

The `SchemaBuilder` concatenates them with the base schema.
GraphQL merges them at parse time.

## Broker

The broker is generic over the event type:

```go
b := broker.New[*User]()

// Subscribe
ch, cancel := b.Subscribe("user.created")
defer cancel()

// Publish
b.Publish("user.created", &User{...})

// Receive
for user := range ch {
    // ...
}
```

Publishers never block. Subscribers with full buffers drop events.

## WebSocket Protocol

The WebSocket handler implements [graphql-ws](https://github.com/apollographql/subscriptions-transport-ws/blob/master/PROTOCOL.md):

| Client → Server | Server → Client |
|-----------------|-----------------|
| `connection_init` | `connection_ack` |
| `start` | `data`, `error`, `complete` |
| `stop` | — |
| `connection_terminate` | — |

## WebSocket Authentication

The kit supports optional authentication for subscriptions. This enables
**private subscriptions** (requiring a user) while keeping **public
subscriptions** (like `version`, `healthCheck`) accessible to anyone.

### Design

- **Auth is opt-in.** Without an `Authenticator`, all connections are anonymous.
- **Failures are non-fatal.** A bad token doesn't close the connection —
  the subscription simply runs without a user.
- **Two token sources.** The kit extracts tokens from:
    1. `connection_init` payload: `{"Authorization": "Bearer <token>"}`
    2. URL query parameter: `?token=<token>` (convenient for development)
- **The kit is auth-agnostic.** It doesn't know about JWTs, users, or
  permissions — you provide an `Authenticator` that turns a token into a user.

### The `Authenticator` Interface

```go
// handlers.Authenticator
type Authenticator interface {
    // Authenticate validates a token and returns the authenticated user.
    //
    //   - (user, nil) → authenticated; user injected into context
    //   - (nil, err)  → unauthenticated; connection proceeds without user
    Authenticate(ctx context.Context, token string) (any, error)
}
```

### Wiring Auth

**Step 1: Implement the `Authenticator`**

```go
// myapp/middleware/ws_auth.go
type WSAuthenticator struct {
    client   *ent.Client
    passport *passport.Passport
}

func NewWSAuthenticator(client *ent.Client, p *passport.Passport) *WSAuthenticator {
    return &WSAuthenticator{client: client, passport: p}
}

func (a *WSAuthenticator) Authenticate(ctx context.Context, token string) (any, error) {
    claims, err := a.passport.ValidateToken(ctx, token)
    if err != nil {
        return nil, fmt.Errorf("invalid token: %w", err)
    }

    userID, err := strconv.Atoi(claims.UserID)
    if err != nil {
        return nil, fmt.Errorf("invalid user ID: %w", err)
    }

    return a.client.User.Get(ctx, userID)
}
```

**Step 2: Pass it to `NewWebSocket`**

```go
import (
    "context"
    graphqlhandlers "github.com/mwangaben/graphql-kit/handlers"
    "myapp/ent"
    appctx "myapp/internal/context"
)

wsAuth := middleware.NewWSAuthenticator(client, passport)

http.HandleFunc("/subscriptions",
    graphqlhandlers.NewWebSocket(schema,
        graphqlhandlers.WithAuthenticator(wsAuth),
        graphqlhandlers.WithUserFunc(func(ctx context.Context, user any) context.Context {
            u, ok := user.(*ent.User)
            if !ok {
                return ctx
            }
            return appctx.WithUser(ctx, u)
        }),
    ),
)
```

### Options Reference

| Option | Purpose |
|--------|---------|
| `WithAuthenticator(a Authenticator)` | Enable auth; provide the token validator |
| `WithUserFunc(fn func(ctx, user) ctx)` | Inject the user into subscription contexts |
| `WithAllowedOrigins(origins ...string)` | Restrict WebSocket origins (default: allow all) |

### The `WithUserFunc` Contract

The kit doesn't know how your app stores users in context. `WithUserFunc`
bridges the kit's `any` user value to your app's typed context helpers:

```go
// Signature
type UserInjector func(ctx context.Context, user any) context.Context
```

**Why it's needed:** The `Authenticate` method returns `any` — the kit
doesn't know your user type. `WithUserFunc` lets you assert the type
and call your own `WithUser`:

```go
graphqlhandlers.WithUserFunc(func(ctx context.Context, user any) context.Context {
    u, ok := user.(*ent.User)     // your type
    if !ok {
        return ctx                 // type mismatch → no user
    }
    return appctx.WithUser(ctx, u) // your context helper
})
```

### Subscription Resolver Pattern

With auth wired, subscription resolvers can:

**Public subscriptions** — ignore auth entirely:

```go
func (r *SystemResolver) Version(ctx context.Context) (<-chan string, error) {
    return r.broker.Subscribe("system.version"), nil
}
```

**Private subscriptions** — require auth, then filter per event:

```go
func (r *UserSubscriptionResolver) UserCreated(ctx context.Context) (<-chan *User, error) {
    // 1. Require authentication
    if _, ok := appctx.GetUserTyped(ctx); !ok {
        return nil, policy.ErrUnauthenticated()
    }

    // 2. Subscribe to raw events
    rawCh, _ := r.broker.Subscribe("user.created")

    // 3. Filter events via policy
    out := make(chan *User, 10)
    go func() {
        defer close(out)
        for u := range rawCh {
            if !r.gate.Allows(ctx, "user.view", u) {
                continue
            }
            select {
            case out <- NewUser(u, r.client):
            case <-ctx.Done():
                return
            }
        }
    }()
    return out, nil
}
```

The `gate.Allows` call uses the **same policy** that queries and mutations
use — one rule, many entry points.

### Client-Side Auth

**Playground (built-in)**

Open Playground with a token in the URL:

```
http://localhost:8080/graphql?token=<your-jwt>
```

The kit's Playground HTML reads the `token` query parameter and
automatically includes it in the `connection_init` payload.

**Any GraphQL WS client**

Send the token in the `connection_init` payload (standard graphql-ws):

```json
{
  "type": "connection_init",
  "payload": {
    "Authorization": "Bearer <your-jwt>"
  }
}
```

**`wscat` example:**

```bash
wscat -c ws://localhost:8080/subscriptions

> {"type":"connection_init","payload":{"Authorization":"Bearer eyJ..."}}
< {"type":"connection_ack"}

> {"id":"1","type":"start","payload":{"query":"subscription { userCreated { id name } }"}}
< {"id":"1","type":"data","payload":{"data":{"userCreated":{"id":1,"name":"Alice"}}}}
```

### Security Considerations

**Do:**

- ✅ Use `WithAllowedOrigins` in production to restrict WS connections
- ✅ Use short-lived JWTs (15 min) — WS reconnects will refresh
- ✅ Require auth in resolvers for private subscriptions
- ✅ Filter events via policy — don't rely on connection-level auth alone

**Don't:**

- ❌ Pass tokens in URLs in production (they may end up in logs)
- ❌ Trust the client to authorize itself — always check on the server
- ❌ Broadcast events to all connections — filter per subscriber

**Recommended flow in production:**

1. Client authenticates via HTTP (`/graphql` login mutation)
2. Client stores the JWT
3. Client opens WS with `connection_init` **payload** (not URL) containing the JWT
4. Server validates, injects user
5. Each subscription filters events via the `policy` gate

## Examples

See `examples/basic/` for a complete working example.

## License

MIT