package observability_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/riversheher/atconnect/internal/observability"
	"github.com/riversheher/atconnect/pkg/store"
)

// mockStore satisfies store.Store with controllable Ping behaviour.
// Embedding the interface provides nil stubs for the many methods we
// don't need in health check tests.
type mockStore struct {
	store.Store
	pingErr error
}

func (m *mockStore) Ping(ctx context.Context) error { return m.pingErr }
func (m *mockStore) Close() error                   { return nil }

func TestLivez_AlwaysReturns200(t *testing.T) {
	hc := observability.NewHealthChecker(&mockStore{})

	rec := httptest.NewRecorder()
	hc.ServeLivez(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status 'ok', got %q", body["status"])
	}
}

func TestReadyz_HealthyStore(t *testing.T) {
	hc := observability.NewHealthChecker(&mockStore{})

	rec := httptest.NewRecorder()
	hc.ServeReadyz(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["status"] != "ready" {
		t.Fatalf("expected status 'ready', got %q", body["status"])
	}
}

func TestReadyz_UnhealthyStore(t *testing.T) {
	hc := observability.NewHealthChecker(&mockStore{pingErr: errors.New("connection refused")})

	rec := httptest.NewRecorder()
	hc.ServeReadyz(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["status"] != "not ready" {
		t.Fatalf("expected status 'not ready', got %q", body["status"])
	}

	checks, ok := body["checks"].(map[string]any)
	if !ok {
		t.Fatal("expected 'checks' object in response")
	}
	storeCheck, _ := checks["store"].(string)
	if storeCheck == "" || storeCheck == "ok" {
		t.Fatalf("expected store check to report error, got %q", storeCheck)
	}
}
