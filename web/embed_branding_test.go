package web_test

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9router/proxy/web"
	"github.com/andybalholm/brotli"
)

func TestHandlerWithSettings_DynamicBranding(t *testing.T) {
	settings := map[string]any{
		"appName":       "Shiroine Router",
		"appSubtitle":   "High Performance Gateway",
		"appLogoUrl":    "https://shiroine.com/logo.png",
		"faviconUrl":    "https://shiroine.com/favicon.ico",
		"loginBgUrl":    "/uploads/login-bg.jpg",
		"accentColor":   "#3b82f6",
		"loginCardStyle": "glass",
	}

	h := web.HandlerWithSettings(func() map[string]any {
		return settings
	})

	t.Run("injects dynamic title, meta, opengraph, and twitter tags", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		body := rec.Body.String()

		// Title
		wantTitle := "<title>Shiroine Router - High Performance Gateway</title>"
		if !strings.Contains(body, wantTitle) {
			t.Errorf("expected body to contain %q, got: %s", wantTitle, body)
		}

		// OpenGraph
		if !strings.Contains(body, `property="og:site_name" content="Shiroine Router"`) {
			t.Errorf("missing or incorrect og:site_name")
		}
		if !strings.Contains(body, `property="og:title" content="Shiroine Router - High Performance Gateway"`) {
			t.Errorf("missing or incorrect og:title")
		}
		if !strings.Contains(body, `property="og:description" content="High Performance Gateway"`) {
			t.Errorf("missing or incorrect og:description")
		}
		if !strings.Contains(body, `property="og:image" content="http://example.com/uploads/login-bg.jpg"`) {
			t.Errorf("missing or incorrect og:image absolute URL")
		}

		// Twitter Cards
		if !strings.Contains(body, `name="twitter:card" content="summary_large_image"`) {
			t.Errorf("missing twitter:card summary_large_image")
		}
		if !strings.Contains(body, `name="twitter:title" content="Shiroine Router - High Performance Gateway"`) {
			t.Errorf("missing twitter:title")
		}
		if !strings.Contains(body, `name="twitter:image" content="http://example.com/uploads/login-bg.jpg"`) {
			t.Errorf("missing twitter:image")
		}

		// Favicon link
		if !strings.Contains(body, `href="https://shiroine.com/favicon.ico"`) {
			t.Errorf("missing custom favicon link in head")
		}

		// Initial branding script
		if !strings.Contains(body, `window.__INITIAL_BRANDING__ =`) {
			t.Errorf("missing window.__INITIAL_BRANDING__ injection")
		}
		if !strings.Contains(body, `"appName":"Shiroine Router"`) {
			t.Errorf("missing appName in window.__INITIAL_BRANDING__")
		}
	})

	t.Run("redirects favicon.ico to custom favicon URL", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusTemporaryRedirect {
			t.Fatalf("status = %d, want 307 redirect", rec.Code)
		}
		if got := rec.Header().Get("Location"); got != "https://shiroine.com/favicon.ico" {
			t.Errorf("Location = %q, want https://shiroine.com/favicon.ico", got)
		}
	})

	t.Run("serves dynamic webmanifest with custom branding", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/manifest+json" {
			t.Errorf("Content-Type = %q, want application/manifest+json", ct)
		}

		var m map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if m["short_name"] != "Shiroine Router" {
			t.Errorf("short_name = %v, want Shiroine Router", m["short_name"])
		}
		if m["theme_color"] != "#3b82f6" {
			t.Errorf("theme_color = %v, want #3b82f6", m["theme_color"])
		}
	})

	t.Run("serves compressed dynamic index.html with brotli and gzip", func(t *testing.T) {
		// Brotli
		reqBr := httptest.NewRequest(http.MethodGet, "/", nil)
		reqBr.Header.Set("Accept-Encoding", "br")
		recBr := httptest.NewRecorder()
		h.ServeHTTP(recBr, reqBr)

		if recBr.Header().Get("Content-Encoding") != "br" {
			t.Errorf("Content-Encoding = %q, want br", recBr.Header().Get("Content-Encoding"))
		}

		brReader := brotli.NewReader(recBr.Body)
		brDecoded, err := io.ReadAll(brReader)
		if err != nil {
			t.Fatalf("brotli decode failed: %v", err)
		}
		if !strings.Contains(string(brDecoded), "Shiroine Router") {
			t.Errorf("decoded brotli body missing Shiroine Router")
		}

		// Gzip
		reqGz := httptest.NewRequest(http.MethodGet, "/", nil)
		reqGz.Header.Set("Accept-Encoding", "gzip")
		recGz := httptest.NewRecorder()
		h.ServeHTTP(recGz, reqGz)

		if recGz.Header().Get("Content-Encoding") != "gzip" {
			t.Errorf("Content-Encoding = %q, want gzip", recGz.Header().Get("Content-Encoding"))
		}

		gzReader, err := gzip.NewReader(recGz.Body)
		if err != nil {
			t.Fatalf("gzip reader creation failed: %v", err)
		}
		gzDecoded, err := io.ReadAll(gzReader)
		_ = gzReader.Close()
		if err != nil {
			t.Fatalf("gzip decode failed: %v", err)
		}
		if !strings.Contains(string(gzDecoded), "Shiroine Router") {
			t.Errorf("decoded gzip body missing Shiroine Router")
		}
	})
}
