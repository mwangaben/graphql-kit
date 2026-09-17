package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/graph-gophers/graphql-go"
	"github.com/mwangaben/graphql-kit/handlers"
)

// ============================================================================
// Test fixtures
// ============================================================================

// resolver implements the base schema.
type resolver struct{}

func (r *resolver) Version() string     { return "1.0.0" }
func (r *resolver) NoopMutation() *bool { return nil }
func (r *resolver) NoopSubscription() <-chan *bool {
	ch := make(chan *bool)
	return ch
}

const testSDL = `
type Query {
    version: String!
}
type Mutation {
    noopMutation: Boolean
}
type Subscription {
    noopSubscription: Boolean
}
`

func newTestHandler(opts ...func(*handlers.HTTPOptions)) *handlers.HTTPHandler {
	parsed := graphql.MustParseSchema(testSDL, &resolver{})
	return handlers.NewHTTP(parsed, opts...)
}

// ============================================================================
// POST
// ============================================================================

func TestHTTP_PostQuery(t *testing.T) {
	h := newTestHandler()

	body := `{"query": "{ version }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	data := resp["data"].(map[string]interface{})
	if data["version"] != "1.0.0" {
		t.Errorf("expected '1.0.0', got %v", data["version"])
	}
}

// ============================================================================
// GET
// ============================================================================

func TestHTTP_GetQuery(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/graphql?query={version}", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "1.0.0") {
		t.Errorf("expected version in response, got %s", rec.Body.String())
	}
}

func TestHTTP_GetPlayground(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "GraphQL Playground") {
		t.Error("expected Playground HTML")
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Errorf("expected HTML content type, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestHTTP_GetPlaygroundDisabled(t *testing.T) {
	h := newTestHandler(func(o *handlers.HTTPOptions) {
		o.Playground = false
	})

	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ============================================================================
// OPTIONS (CORS preflight)
// ============================================================================

func TestHTTP_Options(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodOptions, "/graphql", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected CORS header, got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestHTTP_CustomCORSOrigin(t *testing.T) {
	h := newTestHandler(func(o *handlers.HTTPOptions) {
		o.CORSOrigin = "https://example.com"
	})

	req := httptest.NewRequest(http.MethodOptions, "/graphql", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Errorf("expected custom origin, got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

// ============================================================================
// Method not allowed
// ============================================================================

func TestHTTP_DeleteNotAllowed(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodDelete, "/graphql", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}
