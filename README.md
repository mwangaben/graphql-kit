# graphql-kit

Composable GraphQL infrastructure for Go.

## Features

- 🎯 **Feature-based schema composition** — each feature owns its schema fragment
- 🔌 **Interface-driven** — features satisfy contracts, kit provides infrastructure
- 📦 **Generic broker** — pub/sub for subscriptions, typed per feature
- 🌐 **HTTP + WebSocket handlers** — ready for queries, mutations, subscriptions
- 🎨 **GraphQL Playground** — built-in UI for development
- ⚡ **Zero reflection at runtime** — type-safe composition via Go embedding
- 🧩 **Zero coupling to apps** — no imports of consumer code

## Installation

```bash
go get github.com/mwangaben/graphql-kit
```

## Quick Start
#### 1. Write a feature

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

func (f *Feature) Name() string           { return "user" }
func (f *Feature) Schema() string         { return f.Schema }
func (f *Feature) Resolver() any          { return f.Resolver }
func (f *Feature) SubscriptionResolver() any { return nil }
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

// Subscriptions
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
##### A feature must implement feature.Feature:

```go
type Feature interface {
    Name() string
    Schema() string
    Resolver() any
    SubscriptionResolver() any
}
```

Optional interfaces:

- feature.Brokered — if the feature exposes a broker
- feature.Named — if the feature needs custom topic namespacing


#### Schema Composition
####  Features use extend type to add fields:

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

The SchemaBuilder concatenates them with the base schema.
GraphQL merges them at parse time.


#### Broker
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


### WebSocket Protocol
The WebSocket handler implements graphql-ws:


| Client → Server       | Server → Client         |
|-----------------------|-------------------------|
| connection_init       | connection_ack          |
| start                 | `data, error, complete` |
| stop                  | =                       |
| connection_terminate	 | -                       |


### Examples
See examples/basic/ for a complete working example.


### **License**
**MIT**

```text

---
MIT License

Copyright (c) 2026 Benedict Mwanga

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
