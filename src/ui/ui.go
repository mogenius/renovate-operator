package ui

import (
	"net/http"

	"github.com/gorilla/mux"
)

func (s *Server) registerUiRoutes(router *mux.Router) {
	router.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, "./static/pages/logs.html")
	}).Methods("GET")
	router.PathPrefix("/").Handler(cacheControl(http.FileServer(http.Dir("./static/"))))
}

// vendoredAssets are version-pinned third-party bundles served under stable
// filenames. They change only when their pin is bumped (which ships a new
// image), so they can be cached. Everything else the UI serves is app-authored
// and changes every deploy, so it must be revalidated to avoid a browser
// running a stale index.html against changed scripts.
var vendoredAssets = map[string]struct{}{
	"/js/babel.min.js":        {},
	"/js/react-bundle.esm.js": {},
	"/js/tailwind.min.js":     {},
}

// cacheControl caches the large vendored bundles for a day while forcing
// revalidation of app-authored files. ("no-cache" still allows cheap 304
// revalidation via Last-Modified.) Content-hashed filenames would let the
// vendored bundles be cached immutably, but they aren't hashed today, so a
// day caps how long a pin bump can serve stale.
func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := vendoredAssets[r.URL.Path]; ok {
			// Defer the long-lived header to WriteHeader so a transient
			// non-200 (e.g. a missing bundle) isn't cached for a day.
			next.ServeHTTP(&longCacheWriter{ResponseWriter: w}, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

// longCacheWriter sets the day-long Cache-Control header only on successful
// (200) or revalidated (304) responses, so error responses stay uncached.
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
