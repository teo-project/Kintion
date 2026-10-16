package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testDist() fstest.MapFS {
	return fstest.MapFS{
		"index.html":          {Data: []byte("<title>Kintion</title>")},
		"favicon.ico":         {Data: []byte("ico")},
		"assets/index-abc.js": {Data: []byte("console.log(1)")},
		".gitkeep":            {Data: nil},
		"assets/sub/.hidden":  {Data: []byte("x")},
		legacyLicenses:        {Data: []byte("licenses")},
	}
}

func get(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestSPA(t *testing.T) {
	h := NewHandler(testDeps())

	for _, p := range []string{"/", "/any/path", "/dashboards/1/records", "/.gitkeep", "/index.html/", "/" + legacyLicenses} {
		rec := get(t, h, http.MethodGet, p)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Kintion") {
			t.Errorf("%s: status %d body %q", p, rec.Code, rec.Body.String())
		}
		if cc := rec.Header().Get("Cache-Control"); cc != cacheRevalidate {
			t.Errorf("%s: Cache-Control %q", p, cc)
		}
	}

	rec := get(t, h, http.MethodGet, "/assets/index-abc.js")
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Fatalf("asset: %d %q", rec.Code, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != cacheImmutable {
		t.Fatalf("asset Cache-Control %q", cc)
	}

	if rec := get(t, h, http.MethodGet, "/assets/missing.js"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing asset: %d", rec.Code)
	}
	if rec := get(t, h, http.MethodGet, "/assets/sub/.hidden"); rec.Code != http.StatusNotFound {
		t.Fatalf("hidden file: %d", rec.Code)
	}
	if rec := get(t, h, http.MethodGet, "/favicon.ico"); rec.Code != http.StatusOK || rec.Body.String() != "ico" {
		t.Fatalf("favicon: %d", rec.Code)
	}
	if rec := get(t, h, http.MethodPost, "/any"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rec.Code)
	}
}

func TestAPIUnknownIsJSON404(t *testing.T) {
	h := NewHandler(testDeps())
	for _, m := range []string{http.MethodGet, http.MethodPost} {
		for _, p := range []string{"/api", "/api/", "/api/unknown"} {
			rec := get(t, h, m, p)
			if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("%s %s: %d %q", m, p, rec.Code, rec.Header().Get("Content-Type"))
			}
		}
	}
}

func TestSPAWithoutBuild(t *testing.T) {
	d := testDeps()
	d.Static = fstest.MapFS{".gitkeep": {}}
	h := NewHandler(d)
	if rec := get(t, h, http.MethodGet, "/"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no build: %d", rec.Code)
	}
}

func TestSPALang(t *testing.T) {
	d := testDeps()
	d.Static = fstest.MapFS{"index.html": {Data: []byte(`<!doctype html><html lang="ru"><title>Kintion</title>`)}}
	d.Lang = "en"
	rec := get(t, NewHandler(d), http.MethodGet, "/x")
	if !strings.Contains(rec.Body.String(), `<html lang="en">`) {
		t.Fatalf("lang not substituted: %q", rec.Body.String())
	}
}

func TestSPAFaviconFilesPublic(t *testing.T) {
	d := testDeps()
	d.Static = fstest.MapFS{
		"index.html":           {Data: []byte(`<html lang="ru">`)},
		"favicon.svg":          {Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)},
		"favicon.ico":          {Data: []byte("\x00\x00\x01\x00\x01\x00")},
		"apple-touch-icon.png": {Data: []byte("\x89PNG\r\n\x1a\n")},
	}
	h := NewHandler(d)
	for p, ct := range map[string]string{
		"/favicon.svg":          "image/svg+xml",
		"/favicon.ico":          "image/",
		"/apple-touch-icon.png": "image/png",
	} {
		rec := get(t, h, http.MethodGet, p) // без сессии
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), ct) {
			t.Fatalf("%s: %d %q", p, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}
