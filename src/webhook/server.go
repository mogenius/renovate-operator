package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/assert"
	"renovate-operator/config"
	crdmanager "renovate-operator/internal/crdManager"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

type Server struct {
	manager crdmanager.RenovateJobManager
	logger  logr.Logger
	server  *http.Server
}

func NewWebookServer(manager crdmanager.RenovateJobManager, logger logr.Logger) *Server {
	return &Server{
		manager: manager,
		logger:  logger,
	}
}

func (s *Server) Run() {
	assert.Assert(s.manager != nil, "failed to start server. manager must not be nil")

	router := mux.NewRouter()
	sub := router.PathPrefix("/webhook/v1").Subrouter()
	sub.HandleFunc("/schedule", s.runRenovate).Methods("POST")

	port := config.GetValue("WEBHOOK_SERVER_PORT")

	handler := s.authMiddleware(router)
	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: handler,
	}

	s.server = server
	go func() {
		s.logger.Info("Starting webhook server", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error(err, "failed to start the server")
		} else {
			s.logger.Info("Server started")
		}
	}()
}

func (s *Server) runRenovate(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	renovate := r.URL.Query().Get("job")
	project := r.URL.Query().Get("project")

	err := s.manager.UpdateProjectStatus(
		r.Context(),
		project,
		crdmanager.RenovateJobIdentifier{
			Name:      renovate,
			Namespace: namespace,
		},
		api.JobStatusScheduled,
	)
	if err != nil {
		s.logger.Error(err, "Failed to run Renovate for project", "project", project, "renovateJob", renovate, "namespace", namespace)
		s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "failed to run renovate for project"})
		return
	}

	w.WriteHeader(http.StatusOK)
	s.logger.Info("Successfully triggered Renovate for project", "project", project, "renovateJob", renovate, "namespace", namespace)
}

func (server *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		namespace := r.URL.Query().Get("namespace")
		job := r.URL.Query().Get("job")

		renovateJob, err := server.manager.GetRenovateJob(r.Context(), job, namespace)
		if err != nil || renovateJob == nil {
			server.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "renovate job not found"})
			return
		}

		if !renovateJob.Spec.Webhook.Enabled {
			server.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "webhook not enabled for this renovate job"})
			return
		}

		if renovateJob.Spec.Webhook.Authentication == nil || !renovateJob.Spec.Webhook.Authentication.Enabled {
			server.logger.Info("Webhook authentication not enabled, skipping auth")
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			server.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization header"})
			return
		}

		// Check if the header has the Bearer prefix
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			server.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid authorization header format"})
			return
		}

		token := parts[1]
		valid, err := server.manager.IsWebhookTokenValid(r.Context(), crdmanager.RenovateJobIdentifier{
			Name:      job,
			Namespace: namespace,
		}, token)
		if err != nil {
			server.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}
		if !valid {
			server.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (server *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		server.logger.Error(err, "failed to write JSON response")
	}
}
