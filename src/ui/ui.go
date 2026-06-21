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
var vendoredAssets = map[string]bool{
	"/js/babel.min.js":        true,
	"/js/react-bundle.esm.js": true,
	"/js/tailwind.min.js":     true,
}

// cacheControl caches the large vendored bundles for a day while forcing
// revalidation of app-authored files. ("no-cache" still allows cheap 304
// revalidation via Last-Modified.) Content-hashed filenames would let the
// vendored bundles be cached immutably, but they aren't hashed today, so a
// day caps how long a pin bump can serve stale.
func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vendoredAssets[r.URL.Path] {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}
