package ui

import (
	"net/http"

	"github.com/gorilla/mux"
)

func (s *Server) registerUiRoutes(router *mux.Router) {
	router.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/pages/logs.html")
	}).Methods("GET")
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))
}
