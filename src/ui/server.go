package ui

import (
	"fmt"
	"net/http"

	"encoding/json"
	"renovate-operator/assert"
	"renovate-operator/config"
	"renovate-operator/health"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/renovate"
	"renovate-operator/scheduler"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

type Server struct {
	manager   crdmanager.RenovateJobManager
	discovery renovate.DiscoveryAgent
	scheduler scheduler.Scheduler
	logger    logr.Logger
	server    *http.Server
	health    health.HealthCheck
	version   string
	oidc      *OIDCAuth
}

func NewServer(manager crdmanager.RenovateJobManager, discovery renovate.DiscoveryAgent, scheduler scheduler.Scheduler, logger logr.Logger, health health.HealthCheck, version string, oidc *OIDCAuth) *Server {
	return &Server{
		manager:   manager,
		logger:    logger,
		health:    health,
		discovery: discovery,
		scheduler: scheduler,
		version:   version,
		oidc:      oidc,
	}
}

func (s *Server) registerAuthRoutes(router *mux.Router) {
	if s.oidc != nil {
		router.HandleFunc("/auth/login", s.oidc.handleLogin).Methods("GET")
		router.HandleFunc("/auth/callback", s.oidc.handleCallback).Methods("GET")
		router.HandleFunc("/auth/logout", s.oidc.handleLogout).Methods("GET", "POST")
	}

	router.HandleFunc("/api/v1/auth/status", s.getAuthStatus).Methods("GET")
}

func (s *Server) getAuthStatus(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"enabled": false,
		})
		return
	}
	s.oidc.handleAuthStatus(w, r)
}

func (s *Server) Run() {
	assert.Assert(s.manager != nil, "failed to start server. manager must not be nil")
	assert.Assert(s.health != nil, "failed to start server. health check must not be nil")

	router := mux.NewRouter()

	s.registerAuthRoutes(router)
	s.registerApiV1Routes(router)
	s.registerHealthRoutes(router)
	s.registerUiRoutes(router)

	var handler http.Handler = router
	if s.oidc != nil {
		handler = s.oidc.authMiddleware(router)
	}

	port := config.GetValue("SERVER_PORT")
	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: handler,
	}

	s.server = server
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error(err, "failed to start the server")
		} else {
			s.logger.Info("Server started")
		}
	}()
}
