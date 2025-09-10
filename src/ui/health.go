package ui

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

func (s *Server) registerHealthRoutes(router *mux.Router) {
	router.HandleFunc("/health", s.handleHealthCheck).Methods("GET")
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	healthStatus := s.health.GetHealth()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthStatus)
}
