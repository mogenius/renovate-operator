package ui

import (
	"encoding/json"
	"fmt"
	"net/http"

	"renovate-operator/assert"
	"renovate-operator/config"
	"renovate-operator/github"
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
	githubApp            github.GithubAppToken
	scheduler            scheduler.Scheduler
	logger               logr.Logger
	server               *http.Server
	health               health.HealthCheck
	version              string
	auth                 AuthProvider
	defaultAllowedGroups []string
	Router               *mux.Router
}

func NewServer(manager crdmanager.RenovateJobManager, discovery renovate.DiscoveryAgent, scheduler scheduler.Scheduler, logger logr.Logger, health health.HealthCheck, version string, auth AuthProvider, defaultAllowedGroups []string, githubApp github.GithubAppToken) *Server {
	return &Server{
		manager:              manager,
		logger:               logger,
		health:               health,
		discovery:            discovery,
		githubApp:            githubApp,
		scheduler:            scheduler,
		version:              version,
		auth:                 auth,
		defaultAllowedGroups: defaultAllowedGroups,
		Router:               mux.NewRouter(),
	}
}

func (s *Server) registerAuthRoutes(router *mux.Router) {
	if s.auth != nil {
		sub := router.PathPrefix("/auth").Subrouter()
		sub.Use(telemetry.MuxMiddleware("renovate-operator-ui-auth"))
		sub.HandleFunc("/login", s.auth.HandleLogin).Methods("GET")
		sub.HandleFunc("/callback", s.auth.HandleCallback).Methods("GET")
		sub.HandleFunc("/complete", s.auth.HandleComplete).Methods("GET")
		sub.HandleFunc("/logout", s.auth.HandleLogout).Methods("GET", "POST")
		sub.HandleFunc("/logged-out", s.handleLoggedOut).Methods("GET")
		sub.HandleFunc("/unauthorized", s.handleUnauthorized).Methods("GET")
	}

	router.HandleFunc("/api/v1/auth/status", s.getAuthStatus).Methods("GET")
}

func (s *Server) handleLoggedOut(w http.ResponseWriter, r *http.Request) {
	s.serveHTML(w, r, "./static/pages/logged-out.html")
}

func (s *Server) handleUnauthorized(w http.ResponseWriter, r *http.Request) {
	s.serveHTML(w, r, "./static/pages/unauthorized.html")
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

	// When a base path is configured, all UI, API, auth and health routes are
	// mounted under it so the operator can be co-hosted with other apps on the
	// same hostname. The root router keeps handling the plain "/" so we can add
	// a convenience redirect to the base path.
	base := s.Router
	basePath := BasePath()
	if basePath != "" {
		base = s.Router.PathPrefix(basePath).Subrouter()
		s.Router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, basePath+"/", http.StatusFound)
		}).Methods("GET")
	}

	s.registerAuthRoutes(base)
	s.registerApiV1Routes(base)
	s.registerHealthRoutes(base)
	s.registerUiRoutes(base)

	var handler http.Handler = s.Router
	if s.auth != nil {
		handler = s.auth.AuthMiddleware(s.Router)
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
