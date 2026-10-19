package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The web server sent no Cache-Control, so a browser could reuse an
// index.html from before a deploy without asking, and that page names bundle
// files the deploy removed: a blank page until a hard reload. index.html and
// the other unhashed files are now revalidated on every load; the hashed
// build files under /assets/ are cached for good, since a new build gives
// them new names.

const (
	revalidate = "no-cache"
	immutable  = "public, max-age=31536000, immutable"
)

// buildDir lays out a Vite build: index.html, hashed files under assets/ and
// unhashed public files, all last modified ten days ago.
func buildDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html":              `<html><script src="/assets/index-AbC123.js"></script></html>`,
		"assets/index-AbC123.js":  "console.log(1)",
		"assets/index-DeF456.css": "body{}",
		"favicon.svg":             "<svg/>",
		"geo/countries.geo.json":  "{}",
		"defensepoint-logo.svg":   "<svg/>",
	}
	old := time.Now().Add(-10 * 24 * time.Hour)
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func get(t *testing.T, h http.Handler, path string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCacheControl_ThePageIsRevalidatedOnEveryLoad(t *testing.T) {
	h := createSPAHandler(buildDir(t))

	for _, path := range []string{"/", "/demo1/alerts", "/tenants"} {
		rec := get(t, h, path, nil)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "index-AbC123.js") {
			t.Fatalf("%s: status %d, body %q; want index.html", path, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != revalidate {
			t.Errorf("%s: Cache-Control = %q, want %q", path, got, revalidate)
		}
	}
}

func TestCacheControl_HashedBuildFilesAreCachedForGood(t *testing.T) {
	h := createSPAHandler(buildDir(t))

	for _, path := range []string{"/assets/index-AbC123.js", "/assets/index-DeF456.css"} {
		rec := get(t, h, path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", path, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != immutable {
			t.Errorf("%s: Cache-Control = %q, want %q", path, got, immutable)
		}
	}
}

// Public files keep their names from build to build, so they are revalidated
// like the page.
func TestCacheControl_UnhashedFilesAreRevalidated(t *testing.T) {
	h := createSPAHandler(buildDir(t))

	for _, path := range []string{"/favicon.svg", "/defensepoint-logo.svg", "/geo/countries.geo.json"} {
		rec := get(t, h, path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", path, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != revalidate {
			t.Errorf("%s: Cache-Control = %q, want %q", path, got, revalidate)
		}
	}
}

// A page from before a deploy asks for build files the deploy removed. Its
// 404 must not be stored anywhere, or a file that a later deploy brings back
// would stay missing.
func TestCacheControl_AMissingBuildFileIsNotStored(t *testing.T) {
	h := createSPAHandler(buildDir(t))

	rec := get(t, h, "/assets/index-Gone99.js", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

// Revalidating is cheap: an unchanged page answers 304 with no body.
func TestCacheControl_AnUnchangedPageAnswers304(t *testing.T) {
	h := createSPAHandler(buildDir(t))

	first := get(t, h, "/", nil)
	lastModified := first.Header().Get("Last-Modified")
	if lastModified == "" {
		t.Fatal("no Last-Modified on the page")
	}
	for _, path := range []string{"/", "/demo1/alerts"} {
		rec := get(t, h, path, http.Header{"If-Modified-Since": {lastModified}})
		if rec.Code != http.StatusNotModified {
			t.Errorf("%s: status %d, want 304", path, rec.Code)
		}
	}
}

// Only files under /assets/ are named after their content. The directory
// itself, which the file server lists, changes with every build.
func TestCacheControl_TheBuildDirectoryIsNotCachedForGood(t *testing.T) {
	h := createSPAHandler(buildDir(t))

	rec := get(t, h, "/assets/", nil)
	if got := rec.Header().Get("Cache-Control"); got == immutable {
		t.Errorf("/assets/ (status %d): Cache-Control = %q, want anything but cached for good", rec.Code, got)
	}
}
