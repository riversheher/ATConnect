package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/riversheher/atconnect/internal/middleware"
)

// --- RequestID Middleware ---

func TestRequestID_GeneratesNewID(t *testing.T) {
	var ctxID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxID = middleware.RequestIDFromContext(r.Context())
	})

	handler := middleware.RequestID(inner)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if ctxID == "" {
		t.Fatal("expected non-empty request ID in context")
	}
	if len(ctxID) != 32 {
		t.Fatalf("expected 32-char hex ID, got %d chars: %q", len(ctxID), ctxID)
	}
	if got := rec.Header().Get("X-Request-ID"); got != ctxID {
		t.Fatalf("response header %q does not match context value %q", got, ctxID)
	}
}

func TestRequestID_PropagatesExisting(t *testing.T) {
	const existing = "upstream-trace-abc"
	var ctxID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxID = middleware.RequestIDFromContext(r.Context())
	})

	handler := middleware.RequestID(inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", existing)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if ctxID != existing {
		t.Fatalf("expected context ID %q, got %q", existing, ctxID)
	}
	if got := rec.Header().Get("X-Request-ID"); got != existing {
		t.Fatalf("expected response header %q, got %q", existing, got)
	}
}

// --- Logging Middleware ---

func TestLogging_CapturesStatusAndBytes(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))) })

	body := "hello world"
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(body))
	})

	handler := middleware.Logging(nil)(inner)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/test", nil))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}

	var entry map[string]any
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("failed to decode log entry: %v", err)
	}
	if entry["msg"] != "http request" {
		t.Fatalf("expected msg 'http request', got %q", entry["msg"])
	}
	if int(entry["status"].(float64)) != http.StatusCreated {
		t.Fatalf("expected logged status 201, got %v", entry["status"])
	}
	if int(entry["bytes"].(float64)) != len(body) {
		t.Fatalf("expected logged bytes %d, got %v", len(body), entry["bytes"])
	}
}

func TestLogging_LevelByStatus(t *testing.T) {
	tests := []struct {
		status    int
		wantLevel string
	}{
		{200, "INFO"},
		{301, "INFO"},
		{404, "WARN"},
		{500, "ERROR"},
	}

	for _, tt := range tests {
		var buf bytes.Buffer
		slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tt.status)
		})

		middleware.Logging(nil)(inner).ServeHTTP(
			httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "/", nil),
		)

		var entry map[string]any
		if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
			t.Fatalf("status %d: failed to decode log: %v", tt.status, err)
		}
		if got := entry["level"].(string); got != tt.wantLevel {
			t.Errorf("status %d: expected level %q, got %q", tt.status, tt.wantLevel, got)
		}
	}

	// Restore
	slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
}

func TestLogging_NilMetricsDoesNotPanic(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Should not panic when metrics is nil.
	handler := middleware.Logging(nil)(inner)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// --- Recovery Middleware ---

func TestRecovery_CatchesPanic(t *testing.T) {
	// Suppress panic log output during test.
	slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	t.Cleanup(func() { slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))) })

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := middleware.Recovery(nil)(inner)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON error body: %v", err)
	}
	if body["error"] != "internal_error" {
		t.Fatalf("expected error code 'internal_error', got %q", body["error"])
	}
}

func TestRecovery_PassthroughOnNoPanic(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	handler := middleware.Recovery(nil)(inner)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("expected body 'ok', got %q", got)
	}
}

func TestRecovery_LogsStackTrace(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))) })

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	middleware.Recovery(nil)(inner).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/explode", nil),
	)

	var entry map[string]any
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("failed to decode log: %v", err)
	}
	if entry["msg"] != "panic recovered" {
		t.Fatalf("expected msg 'panic recovered', got %q", entry["msg"])
	}
	stack, _ := entry["stack"].(string)
	if !strings.Contains(stack, "goroutine") {
		t.Fatal("expected stack trace in log entry")
	}
}

// --- Full Middleware Stack ---

func TestFullStack_RequestIDAndRecovery(t *testing.T) {
	// Suppress log output.
	slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	t.Cleanup(func() { slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))) })

	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("stack test")
	})

	handler := middleware.Recovery(nil)(
		middleware.RequestID(
			middleware.Logging(nil)(mux),
		),
	)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Normal request — should have X-Request-ID.
	resp, err := http.Get(srv.URL + "/ok")
	if err != nil {
		t.Fatalf("GET /ok: %v", err)
	}
	resp.Body.Close()
	if resp.Header.Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID header on response")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Panic request — should still get 500 and X-Request-ID.
	resp, err = http.Get(srv.URL + "/panic")
	if err != nil {
		t.Fatalf("GET /panic: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}
