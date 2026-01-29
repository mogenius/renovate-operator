package ui

import (
	"net/http"

	"github.com/gorilla/mux"
)

func (s *Server) registerUiRoutes(router *mux.Router) {
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))
}
