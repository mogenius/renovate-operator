package ui

import (
	"net/http"
	"os"
	"renovate-operator/config"

	"github.com/gorilla/mux"
)

func (s *Server) registerUiRoutes(router *mux.Router) {
	router.HandleFunc("/css/styles.css", s.handleCssStyles).Methods("GET")
	router.HandleFunc("/favicon.ico", s.handleFavicon).Methods("GET")
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))
}

func (s *Server) handleCssStyles(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "text/css")

	// Determine directory to serve
	cssFile := config.GetValue("CUSTOM_CSS_FILE_PATH")
	if cssFile == "" {
		cssFile = "static/css/styles.css"
	}
	// Ensure file exists
	if _, err := os.Stat(cssFile); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	// Serve the CSS file
	http.ServeFile(w, r, cssFile)
}
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "image/x-icon")

	// Determine directory to serve
	favicon := config.GetValue("CUSTOM_FAVICON_FILE_PATH")
	if favicon == "" {
		favicon = "static/favicon.ico"
	}
	// Ensure file exists
	if _, err := os.Stat(favicon); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	// Serve the CSS file
	http.ServeFile(w, r, favicon)
}
