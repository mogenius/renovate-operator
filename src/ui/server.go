package ui

import (
	"fmt"
	"net/http"

	"renovate-operator/assert"
	"renovate-operator/config"
	"renovate-operator/health"
	crdmanager "renovate-operator/internal/crdManager"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

type Server struct {
	manager crdmanager.RenovateJobManager
	logger  logr.Logger
	server  *http.Server
	health  health.HealthCheck
}

func NewServer(manager crdmanager.RenovateJobManager, logger logr.Logger, health health.HealthCheck) *Server {
	return &Server{
		manager: manager,
		logger:  logger,
		health:  health,
	}
}

func (s *Server) Run() {
	assert.Assert(s.manager != nil, "failed to start server. manager must not be nil")
	assert.Assert(s.health != nil, "failed to start server. health check must not be nil")

	router := mux.NewRouter()

	s.registerApiV1Routes(router)
	s.registerHealthRoutes(router)
	s.registerUiRoutes(router)

	port := config.GetValue("SERVER_PORT")
	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: router,
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
