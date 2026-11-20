package web

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
)

func init() {
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

//go:embed dist/*
var DistFS embed.FS

// SettingsProvider supplies the gateway's active configuration map.
type SettingsProvider func() map[string]any

// Handler returns an http.Handler that serves the embedded SPA files with index.html fallback for client-side routing.
func Handler() http.Handler {
	return HandlerWithSettings(nil)
}

// HandlerWithSettings returns an http.Handler that serves the embedded SPA files,
// dynamically injecting operator branding (title, meta headers, OpenGraph, Twitter cards,
// custom favicon redirects, and webmanifest) when settingsProvider is provided.
func HandlerWithSettings(settingsFn SettingsProvider) http.Handler {
	subFS, err := fs.Sub(DistFS, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Dashboard web assets not available", http.StatusNotFound)
		})
	}

	cache := &brandingIndexCache{}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}

		// Dynamic Favicon redirect if a custom favicon or logo is configured
		if settingsFn != nil && (name == "favicon.ico" || name == "favicon.svg") {
			settings := settingsFn()
			fav := getSettingString(settings, "faviconUrl")
			if fav == "" {
				fav = getSettingString(settings, "appLogoUrl")
			}
			if fav != "" && fav != "/favicon.ico" && fav != "/favicon.svg" && fav != name && fav != "/"+name {
				w.Header().Set("Cache-Control", "no-cache, must-revalidate")
				http.Redirect(w, r, fav, http.StatusTemporaryRedirect)
				return
			}
		}

		// Dynamic webmanifest if custom branding is configured
		if settingsFn != nil && (name == "manifest.webmanifest" || name == "manifest.json") {
			settings := settingsFn()
			if hasCustomBranding(settings) {
				serveDynamicManifest(w, r, settings)
				return
			}
		}

		// Dynamic index.html entry document or client-side routing fallback
		if settingsFn != nil && (name == "index.html" || isSPARoute(subFS, name)) {
			serveDynamicIndex(w, r, subFS, settingsFn, cache)
			return
		}

		// A file in the embedded FS is served as itself: the CSS, the JS, the
		// SVG, the PNG.
		if isFile(subFS, name) {
			applyAssetHeaders(w, name)
			serveFile(w, r, subFS, name)
			return
		}

		// A path with a file extension that is not in the FS is a missing
		// asset, not a client-side route: answering it with the entry document
		// would hand the browser HTML for a script it asked to execute.
		if strings.Contains(path.Base(name), ".") && !strings.HasSuffix(name, ".html") {
			http.NotFound(w, r)
			return
		}

		// Everything else is a client-side route, which the SPA resolves on
		// its own. The entry document answers it.
		if !isFile(subFS, "index.html") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		serveFile(w, r, subFS, "index.html")
	})
}

// isSPARoute reports whether `name` represents a client-side routing fallback.
func isSPARoute(subFS fs.FS, name string) bool {
	if isFile(subFS, name) {
		return false
	}
	if strings.Contains(path.Base(name), ".") && !strings.HasSuffix(name, ".html") {
		return false
	}
	return isFile(subFS, "index.html")
}

// isFile reports whether `name` is a regular file in the embedded FS.
func isFile(subFS fs.FS, name string) bool {
	f, err := subFS.Open(name)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return !stat.IsDir()
}

// applyAssetHeaders sets the cache policy and the content type for an asset.
func applyAssetHeaders(w http.ResponseWriter, name string) {
	switch {
	case name == "index.html":
		w.Header().Set("Cache-Control", "no-cache")
	case strings.HasPrefix(name, "assets/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case name == "sw.js":
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Service-Worker-Allowed", "/")
	case name == "manifest.webmanifest":
		w.Header().Set("Content-Type", "application/manifest+json")
	}
}

// serveFile serves `name` from the embedded FS, preferring the pre-compressed
// variant the client's Accept-Encoding allows.
func serveFile(w http.ResponseWriter, r *http.Request, subFS fs.FS, name string) {
	w.Header().Set("Vary", "Accept-Encoding")

	encoding, variant := negotiateEncoding(r.Header.Get("Accept-Encoding"))
	if variant != "" {
		if serveVariant(w, r, subFS, name+variant, name, encoding) {
			return
		}
	}

	// No usable variant: serve the original.
	serveVariant(w, r, subFS, name, name, "")
}

// serveVariant writes one file from the embedded FS to the response.
func serveVariant(w http.ResponseWriter, r *http.Request, subFS fs.FS, name, contentName, encoding string) bool {
	f, err := subFS.Open(name)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		return false
	}

	if encoding != "" {
		w.Header().Set("Content-Encoding", encoding)
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", contentTypeFor(contentName))
	}

	seeker, ok := f.(io.ReadSeeker)
	if !ok {
		data, readErr := io.ReadAll(f)
		if readErr != nil {
			http.Error(w, "asset unavailable", http.StatusInternalServerError)
			return true
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
		return true
	}

	if r.Header.Get("Range") == "" {
		w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	}
	http.ServeContent(w, r, contentName, stat.ModTime(), seeker)
	return true
}

// Regex patterns for dynamic branding replacement in index.html.
var (
	reTitle       = regexp.MustCompile(`(?i)<title>[^<]*</title>`)
	reAppleTitle  = regexp.MustCompile(`(?i)<meta\s+name=["']apple-mobile-web-app-title["'][^>]*>`)
	reThemeColor  = regexp.MustCompile(`(?i)<meta\s+name=["']theme-color["'][^>]*>`)
	reFaviconIco  = regexp.MustCompile(`(?i)<link\s+rel=["']icon["'][^>]*href=["'][^"']*\.ico["'][^>]*>`)
	reFaviconSvg  = regexp.MustCompile(`(?i)<link\s+rel=["']icon["'][^>]*href=["'][^"']*\.svg["'][^>]*>`)
	reAppleTouch  = regexp.MustCompile(`(?i)<link\s+rel=["']apple-touch-icon["'][^>]*>`)
	reDescription = regexp.MustCompile(`(?i)<meta\s+name=["']description["'][^>]*>`)
	reOG          = regexp.MustCompile(`(?i)<meta\s+property=["']og:[^"']+["'][^>]*>`)
	reTwitter     = regexp.MustCompile(`(?i)<meta\s+name=["']twitter:[^"']+["'][^>]*>`)
	reCanonical   = regexp.MustCompile(`(?i)<link\s+rel=["']canonical["'][^>]*>`)
	reInitScript  = regexp.MustCompile(`(?i)<script>\s*window\.__INITIAL_BRANDING__\s*=\s*[\s\S]*?</script>`)
)

type cachedPage struct {
	raw []byte
	br  []byte
	gz  []byte
}

type brandingIndexCache struct {
	mu    sync.RWMutex
	pages map[string]cachedPage
}

func (c *brandingIndexCache) get(key string) (cachedPage, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.pages == nil {
		return cachedPage{}, false
	}
	p, ok := c.pages[key]
	return p, ok
}

func (c *brandingIndexCache) set(key string, page cachedPage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pages == nil {
		c.pages = make(map[string]cachedPage)
	}
	if len(c.pages) > 20 {
		c.pages = make(map[string]cachedPage)
	}
	c.pages[key] = page
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	return scheme + "://" + host
}

func getSettingString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func getSettingBool(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func hasCustomBranding(settings map[string]any) bool {
	if settings == nil {
		return false
	}
	return getSettingString(settings, "appName") != "" ||
		getSettingString(settings, "appSubtitle") != "" ||
		getSettingString(settings, "appLogoUrl") != "" ||
		getSettingString(settings, "faviconUrl") != "" ||
		getSettingString(settings, "loginBgUrl") != "" ||
		getSettingString(settings, "accentColor") != ""
}

// injectBranding injects operator branding into the raw index.html document.
func injectBranding(rawHTML []byte, settings map[string]any, fullURL, origin string) []byte {
	htmlStr := string(rawHTML)

	appName := getSettingString(settings, "appName")
	if appName == "" {
		appName = "9router-go"
	}
	appSubtitle := getSettingString(settings, "appSubtitle")
	appLogoUrl := getSettingString(settings, "appLogoUrl")
	faviconUrl := getSettingString(settings, "faviconUrl")
	if faviconUrl == "" {
		faviconUrl = appLogoUrl
	}
	if faviconUrl == "" {
		faviconUrl = "/favicon.svg"
	}

	loginBgUrl := getSettingString(settings, "loginBgUrl")
	accentColor := getSettingString(settings, "accentColor")
	if accentColor == "" {
		accentColor = "#0b0e13"
	}

	pageTitle := appName
	if appSubtitle != "" {
		pageTitle = appName + " - " + appSubtitle
	} else if appName != "9router-go" {
		pageTitle = appName + " - High-Throughput AI Gateway"
	} else {
		pageTitle = "9router-go - High-Throughput AI Gateway"
	}

	description := appSubtitle
	if description == "" {
		description = "High-Throughput AI Gateway - Unified LLM Proxy & Router"
	}

	imageUrl := loginBgUrl
	if imageUrl == "" {
		imageUrl = appLogoUrl
	}
	if imageUrl != "" && strings.HasPrefix(imageUrl, "/") && origin != "" {
		imageUrl = strings.TrimRight(origin, "/") + imageUrl
	}

	// 1. Replace <title>
	escapedTitle := html.EscapeString(pageTitle)
	if reTitle.MatchString(htmlStr) {
		htmlStr = reTitle.ReplaceAllString(htmlStr, "<title>"+escapedTitle+"</title>")
	}

	// 2. Replace apple-mobile-web-app-title
	escapedAppName := html.EscapeString(appName)
	if reAppleTitle.MatchString(htmlStr) {
		htmlStr = reAppleTitle.ReplaceAllString(htmlStr, `<meta name="apple-mobile-web-app-title" content="`+escapedAppName+`" />`)
	}

	// 3. Replace theme-color
	escapedTheme := html.EscapeString(accentColor)
	if reThemeColor.MatchString(htmlStr) {
		htmlStr = reThemeColor.ReplaceAllString(htmlStr, `<meta name="theme-color" content="`+escapedTheme+`" />`)
	}

	// 4. Update favicon links
	escapedFav := html.EscapeString(faviconUrl)
	if faviconUrl != "" && faviconUrl != "/favicon.svg" {
		favLink := `<link rel="icon" href="` + escapedFav + `" />`
		if reFaviconIco.MatchString(htmlStr) {
			htmlStr = reFaviconIco.ReplaceAllString(htmlStr, favLink)
		}
		if reFaviconSvg.MatchString(htmlStr) {
			htmlStr = reFaviconSvg.ReplaceAllString(htmlStr, "")
		}
		if reAppleTouch.MatchString(htmlStr) {
			htmlStr = reAppleTouch.ReplaceAllString(htmlStr, `<link rel="apple-touch-icon" href="`+escapedFav+`" />`)
		}
	}

	// 5. Clean up any pre-existing description, OG, Twitter, canonical, or init script
	htmlStr = reDescription.ReplaceAllString(htmlStr, "")
	htmlStr = reOG.ReplaceAllString(htmlStr, "")
	htmlStr = reTwitter.ReplaceAllString(htmlStr, "")
	htmlStr = reCanonical.ReplaceAllString(htmlStr, "")
	htmlStr = reInitScript.ReplaceAllString(htmlStr, "")

	// 6. Build meta tags
	escapedDesc := html.EscapeString(description)
	var metaBuilder strings.Builder
	metaBuilder.WriteString("\n    <meta name=\"description\" content=\"" + escapedDesc + "\" />\n")
	metaBuilder.WriteString("    <meta property=\"og:type\" content=\"website\" />\n")
	metaBuilder.WriteString("    <meta property=\"og:site_name\" content=\"" + escapedAppName + "\" />\n")
	metaBuilder.WriteString("    <meta property=\"og:title\" content=\"" + escapedTitle + "\" />\n")
	metaBuilder.WriteString("    <meta property=\"og:description\" content=\"" + escapedDesc + "\" />\n")

	if imageUrl != "" {
		escapedImg := html.EscapeString(imageUrl)
		metaBuilder.WriteString("    <meta property=\"og:image\" content=\"" + escapedImg + "\" />\n")
		metaBuilder.WriteString("    <meta name=\"twitter:card\" content=\"summary_large_image\" />\n")
		metaBuilder.WriteString("    <meta name=\"twitter:image\" content=\"" + escapedImg + "\" />\n")
	} else {
		metaBuilder.WriteString("    <meta name=\"twitter:card\" content=\"summary\" />\n")
	}

	metaBuilder.WriteString("    <meta name=\"twitter:title\" content=\"" + escapedTitle + "\" />\n")
	metaBuilder.WriteString("    <meta name=\"twitter:description\" content=\"" + escapedDesc + "\" />\n")

	if fullURL != "" {
		escapedURL := html.EscapeString(fullURL)
		metaBuilder.WriteString("    <meta property=\"og:url\" content=\"" + escapedURL + "\" />\n")
		metaBuilder.WriteString("    <link rel=\"canonical\" href=\"" + escapedURL + "\" />\n")
	}

	// 7. Inject window.__INITIAL_BRANDING__
	initBranding := map[string]any{
		"appName":                      appName,
		"appSubtitle":                  appSubtitle,
		"appLogoUrl":                   appLogoUrl,
		"faviconUrl":                   faviconUrl,
		"loginTitle":                   getSettingString(settings, "loginTitle"),
		"loginSubtitle":                getSettingString(settings, "loginSubtitle"),
		"loginBgUrl":                   loginBgUrl,
		"loginBgBlur":                  getSettingBool(settings, "loginBgBlur"),
		"loginHideDefaultPasswordHint": getSettingBool(settings, "loginHideDefaultPasswordHint"),
		"hideBrandFooter":              getSettingBool(settings, "hideBrandFooter"),
		"footerText":                   getSettingString(settings, "footerText"),
		"accentColor":                  accentColor,
		"loginCardStyle":               getSettingString(settings, "loginCardStyle"),
	}
	if bJSON, err := json.Marshal(initBranding); err == nil {
		metaBuilder.WriteString("    <script>window.__INITIAL_BRANDING__ = " + string(bJSON) + ";</script>\n")
	}

	// Insert before </head>
	if idx := strings.Index(strings.ToLower(htmlStr), "</head>"); idx != -1 {
		htmlStr = htmlStr[:idx] + metaBuilder.String() + "  " + htmlStr[idx:]
	} else {
		htmlStr += metaBuilder.String()
	}

	return []byte(htmlStr)
}

func serveDynamicIndex(w http.ResponseWriter, r *http.Request, subFS fs.FS, settingsFn SettingsProvider, cache *brandingIndexCache) {
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Cache-Control", "no-cache")

	settings := settingsFn()
	origin := requestOrigin(r)
	fullURL := origin + r.URL.Path

	cacheKey := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		origin,
		r.URL.Path,
		getSettingString(settings, "appName"),
		getSettingString(settings, "appSubtitle"),
		getSettingString(settings, "appLogoUrl"),
		getSettingString(settings, "faviconUrl"),
		getSettingString(settings, "loginBgUrl"),
		getSettingString(settings, "accentColor"),
		getSettingString(settings, "loginTitle"),
		getSettingString(settings, "loginSubtitle"),
	)

	page, ok := cache.get(cacheKey)
	if !ok {
		f, err := subFS.Open("index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		rawHTML, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			http.Error(w, "failed to read index.html", http.StatusInternalServerError)
			return
		}

		injected := injectBranding(rawHTML, settings, fullURL, origin)

		var brBuf bytes.Buffer
		brWriter := brotli.NewWriterLevel(&brBuf, brotli.DefaultCompression)
		_, _ = brWriter.Write(injected)
		_ = brWriter.Close()

		var gzBuf bytes.Buffer
		gzWriter, _ := gzip.NewWriterLevel(&gzBuf, gzip.DefaultCompression)
		_, _ = gzWriter.Write(injected)
		_ = gzWriter.Close()

		page = cachedPage{
			raw: injected,
			br:  brBuf.Bytes(),
			gz:  gzBuf.Bytes(),
		}
		cache.set(cacheKey, page)
	}

	encoding, _ := negotiateEncoding(r.Header.Get("Accept-Encoding"))
	var data []byte
	switch encoding {
	case "br":
		w.Header().Set("Content-Encoding", "br")
		data = page.br
	case "gzip":
		w.Header().Set("Content-Encoding", "gzip")
		data = page.gz
	default:
		data = page.raw
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("Range") == "" {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	}
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(data))
}

func serveDynamicManifest(w http.ResponseWriter, r *http.Request, settings map[string]any) {
	appName := getSettingString(settings, "appName")
	if appName == "" {
		appName = "9router-go"
	}
	appSubtitle := getSettingString(settings, "appSubtitle")
	desc := appSubtitle
	if desc == "" {
		desc = "High-throughput AI gateway & infrastructure router in Go"
	}
	accentColor := getSettingString(settings, "accentColor")
	if accentColor == "" {
		accentColor = "#0b0e13"
	}
	fav := getSettingString(settings, "faviconUrl")
	if fav == "" {
		fav = getSettingString(settings, "appLogoUrl")
	}

	manifest := map[string]any{
		"name":             appName + " - High-Throughput AI Gateway",
		"short_name":       appName,
		"description":      desc,
		"start_url":        "/",
		"scope":            "/",
		"display":          "standalone",
		"background_color": "#0b0e13",
		"theme_color":      accentColor,
		"orientation":      "any",
	}

	icons := []map[string]any{
		{"src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png"},
		{"src": "/icons/icon-512.png", "sizes": "512x512", "type": "image/png"},
		{"src": "/icons/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable"},
	}
	if fav != "" {
		iconType := "image/png"
		if strings.HasSuffix(fav, ".svg") {
			iconType = "image/svg+xml"
		} else if strings.HasSuffix(fav, ".ico") {
			iconType = "image/x-icon"
		}
		icons = append([]map[string]any{
			{"src": fav, "sizes": "any", "type": iconType},
		}, icons...)
	} else {
		icons = append([]map[string]any{
			{"src": "/favicon.svg", "sizes": "any", "type": "image/svg+xml"},
		}, icons...)
	}
	manifest["icons"] = icons

	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "no-cache")
	data, err := json.Marshal(manifest)
	if err != nil {
		http.Error(w, "manifest generation error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// contentTypeFor returns the media type for a served asset.
//
// It exists because the served path sets the type itself rather than letting
// http.FileServer infer it from the request URL — the SPA fallback serves
// index.html for a request whose URL says something else entirely.
//
// The types the dashboard ships are pinned rather than looked up.
// mime.TypeByExtension consults the machine's MIME database (/etc/mime.types
// and friends) on top of Go's small builtin table, so the same binary serves
// a different Content-Type on a different host — and a script served as the
// wrong type is refused by the browser. The pinned list covers every
// extension in the built dashboard; anything else falls back to the lookup.
func contentTypeFor(name string) string {
	switch path.Ext(name) {
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".html":
		return "text/html; charset=utf-8"
	case ".json":
		return "application/json"
	case ".webmanifest":
		return "application/manifest+json"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/vnd.microsoft.icon"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	case ".txt":
		return "text/plain; charset=utf-8"
	}
	if t := mime.TypeByExtension(path.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// negotiateEncoding picks the compression variant to serve.
//
// It returns the Content-Encoding value and the file suffix of the variant
// ("br" or "gz"), or empty strings when the client asked for nothing this
// handler can provide.
//
// The `Accept-Encoding` header is a list with optional quality values
// ("gzip;q=0.5", "*", "identity"). Only an explicit `br` or `gzip` counts:
// a wildcard is not enough, because `br` and `gzip` are not always
// interchangeable with whatever else the client accepts, and the entry
// document has to be readable by the browser that asked for it.
//
// On equal quality the smaller encoding wins, so brotli is preferred over
// gzip. Chrome sends "gzip, deflate, br" with no quality values at all, and
// a handler that keeps the first match on a tie hands that browser the
// larger of the two — the header order says nothing about preference.
func negotiateEncoding(header string) (encoding string, suffix string) {
	if header == "" {
		return "", ""
	}

	// Preference on a quality tie. Lower is better.
	preference := map[string]int{"br": 0, "gzip": 1}

	bestName := ""
	bestSuffix := ""
	bestQ := -1.0
	bestPreference := 0

	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		name := strings.ToLower(strings.TrimSpace(fields[0]))
		rank, supported := preference[name]
		if !supported {
			continue
		}

		q := 1.0
		for _, field := range fields[1:] {
			field = strings.TrimSpace(field)
			if !strings.HasPrefix(strings.ToLower(field), "q=") {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(field[2:]), 64)
			if err != nil {
				continue
			}
			q = parsed
		}
		// q=0 means "not acceptable", so the variant is never chosen.
		if q <= 0 {
			continue
		}

		if q > bestQ || (q == bestQ && rank < bestPreference) {
			bestName = name
			bestSuffix = nameSuffix(name)
			bestQ = q
			bestPreference = rank
		}
	}

	if bestName == "" {
		return "", ""
	}
	return bestName, bestSuffix
}

// nameSuffix maps a Content-Encoding name to the suffix the pre-compressed
// variant was written under.
func nameSuffix(name string) string {
	if name == "br" {
		return ".br"
	}
	return ".gz"
}
