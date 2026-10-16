package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func testDeps() Deps { return Deps{Log: discardLog(), DB: fakeDB{}, Static: testDist()} }

func TestHealthz(t *testing.T) {
	h := NewHandler(testDeps())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["status"] != "ok" || body["db"] != "ok" {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	if rec.Header().Get(RequestIDHeader) == "" {
		t.Fatal("no X-Request-Id")
	}
}

func TestHealthzDBDown(t *testing.T) {
	d := testDeps()
	d.DB = fakeDB{err: errors.New("connection refused")}
	rec := httptest.NewRecorder()
	NewHandler(d).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["db"] != "error" {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
}

func TestRequestIDIsGeneratedByServer(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = RequestIDFrom(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "client-supplied")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if seen == "" || seen == "client-supplied" || rec.Header().Get(RequestIDHeader) != seen {
		t.Fatalf("request id %q, header %q", seen, rec.Header().Get(RequestIDHeader))
	}
}

func TestRecover(t *testing.T) {
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}), RequestID, Logging(discardLog()), Recover(discardLog()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	var body ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error != "internal" {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
}
