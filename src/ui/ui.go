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

var vendoredAssets = map[string]struct{}{
	"/js/babel.min.js":        {},
	"/js/react-bundle.esm.js": {},
	"/js/tailwind.min.js":     {},
}

func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := vendoredAssets[r.URL.Path]; ok {
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
