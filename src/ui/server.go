package ui

import (
	"encoding/json"
	"fmt"
	"net/http"

	"renovate-operator/assert"
	"renovate-operator/config"
	"renovate-operator/health"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/renovate"
	"renovate-operator/internal/telemetry"
	"renovate-operator/scheduler"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

type Server struct {
	manager              crdmanager.RenovateJobManager
	discovery            renovate.DiscoveryAgent
	scheduler            scheduler.Scheduler
	logger               logr.Logger
	server               *http.Server
	health               health.HealthCheck
	version              string
	auth                 AuthProvider
	defaultAllowedGroups []string
}

func NewServer(manager crdmanager.RenovateJobManager, discovery renovate.DiscoveryAgent, scheduler scheduler.Scheduler, logger logr.Logger, health health.HealthCheck, version string, auth AuthProvider, defaultAllowedGroups []string) *Server {
	return &Server{
		manager:              manager,
		logger:               logger,
		health:               health,
		discovery:            discovery,
		scheduler:            scheduler,
		version:              version,
		auth:                 auth,
		defaultAllowedGroups: defaultAllowedGroups,
	}
}

func (s *Server) registerAuthRoutes(router *mux.Router) {
	if s.auth != nil {
		router.HandleFunc("/auth/login", s.auth.HandleLogin).Methods("GET")
		router.HandleFunc("/auth/callback", s.auth.HandleCallback).Methods("GET")
		router.HandleFunc("/auth/complete", s.auth.HandleComplete).Methods("GET")
		router.HandleFunc("/auth/logout", s.auth.HandleLogout).Methods("GET", "POST")
		router.HandleFunc("/auth/logged-out", s.handleLoggedOut).Methods("GET")
	}

	router.HandleFunc("/api/v1/auth/status", s.getAuthStatus).Methods("GET")
}

func (s *Server) handleLoggedOut(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, err := fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Logged Out - Renovate Operator</title>
  <script src="/js/tailwind.min.js"></script>
</head>
<body class="bg-gray-50 min-h-screen flex items-center justify-center">
  <div class="text-center">
    <h1 class="text-2xl font-bold text-gray-800 mb-4">Successfully logged out</h1>
    <a href="/auth/login" class="inline-block px-6 py-2 bg-sky-600 text-white rounded-lg hover:bg-sky-700 transition-colors">
      Log in again
    </a>
  </div>
</body>
</html>`)

	if err != nil {
		s.logger.Error(err, "failed to write logged-out response")
	}
}

func (s *Server) getAuthStatus(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"enabled": false,
		})
		return
	}
	s.auth.HandleAuthStatus(w, r)
}

func (s *Server) Run() {
	assert.Assert(s.manager != nil, "failed to start server. manager must not be nil")
	assert.Assert(s.health != nil, "failed to start server. health check must not be nil")

	router := mux.NewRouter()
	router.Use(telemetry.MuxMiddleware("renovate-operator-ui"))

	s.registerAuthRoutes(router)
	s.registerApiV1Routes(router)
	s.registerHealthRoutes(router)
	s.registerUiRoutes(router)

	var handler http.Handler = router
	if s.auth != nil {
		handler = s.auth.AuthMiddleware(router)
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
