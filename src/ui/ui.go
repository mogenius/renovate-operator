package ui

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
)

func (s *Server) registerUiRoutes(router *mux.Router) {
	router.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		s.serveHTML(w, r, "./static/pages/logs.html")
	}).Methods("GET")

	fileServer := http.FileServer(http.Dir("./static/"))
	base := BasePath()
	router.PathPrefix("/").Handler(cacheControl(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve the SPA entry point (with base-path injection) for the root and
		// any explicit index request; everything else is a static asset.
		rel := strings.TrimPrefix(r.URL.Path, base)
		if rel == "" || rel == "/" || rel == "/index.html" {
			s.serveHTML(w, r, "./static/index.html")
			return
		}
		http.StripPrefix(base, fileServer).ServeHTTP(w, r)
	})))
}

// serveHTML serves an HTML page, injecting a <base> tag and a
// window.__BASE_PATH__ global so the frontend can resolve assets and API
// calls relative to the configured sub-path.
func (s *Server) serveHTML(w http.ResponseWriter, r *http.Request, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		s.logger.Error(err, "failed to read HTML page", "path", path)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	base := BasePath()
	injection := fmt.Sprintf("<head>\n    <base href=\"%s/\">\n    <script>window.__BASE_PATH__ = %q;</script>", base, base)
	html := strings.Replace(string(data), "<head>", injection, 1)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if _, err := w.Write([]byte(html)); err != nil {
		s.logger.Error(err, "failed to write HTML response")
	}
}

var vendoredAssets = map[string]struct{}{
	"/js/babel.min.js":        {},
	"/js/react-bundle.esm.js": {},
	"/js/tailwind.min.js":     {},
}

func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, BasePath())
		if _, ok := vendoredAssets[rel]; ok {
			next.ServeHTTP(&longCacheWriter{ResponseWriter: w}, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

// wrap the request writer to only write cache headers if request was successful
type longCacheWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *longCacheWriter) WriteHeader(status int) {
	w.wroteHeader = true
	if status == http.StatusOK || status == http.StatusNotModified {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *longCacheWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
