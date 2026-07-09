package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/assert"
	"renovate-operator/config"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/telemetry"
	"renovate-operator/internal/types"

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

func RegisterWebhookRoutes(router *mux.Router, server *Server) {
	assert.Assert(server.manager != nil, "failed to register webhook routes. manager must not be nil")

	sub := router.PathPrefix("/webhook/v1").Subrouter()
	sub.Use(telemetry.MuxMiddleware("renovate-operator-webhook"))
	sub.HandleFunc("/schedule", server.runRenovate).Methods("POST")
	sub.HandleFunc("/gitlab", server.gitLabWebhook).Methods("POST")
	sub.HandleFunc("/github", server.githubWebhook).Methods("POST")
	sub.HandleFunc("/forgejo", server.forgejoWebhook).Methods("POST")
	sub.HandleFunc("/gitea", server.giteaWebhook).Methods("POST")
	sub.HandleFunc("/bitbucket", server.bitbucketWebhook).Methods("POST")
}

func (s *Server) Run() {
	assert.Assert(s.manager != nil, "failed to start server. manager must not be nil")

	router := mux.NewRouter()
	RegisterWebhookRoutes(router, s)

	port := config.GetValue("WEBHOOK_SERVER_PORT")

	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: router,
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
	project := r.URL.Query().Get("project")
	if project == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing project query parameter"})
		return
	}

	namespace := r.URL.Query().Get("namespace")
	jobName := r.URL.Query().Get("job")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read request body"})
		return
	}

	checker := buildAuthCheckerFromRequest(r, body, s.manager)
	jobId, err := FindAndAuthenticateJob(r.Context(), s.manager, namespace, jobName, project, checker)
	if err != nil {
		s.handleResolverError(w, err)
		return
	}

	err = s.manager.UpdateProjectStatus(
		r.Context(),
		project,
		jobId,
		&types.RenovateStatusUpdate{
			Status:   api.JobStatusScheduled,
			Priority: 1,
		},
	)
	if s.handleUpdateProjectStatusError(w, err, project, jobId.Name, jobId.Namespace) {
		return
	}

	w.WriteHeader(http.StatusOK)
	s.logger.V(2).Info("Successfully triggered Renovate for project", "project", project, "renovateJob", jobId.Name, "namespace", jobId.Namespace, "priority", 1)
}

func (s *Server) handleResolverError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNoMatchingJob) {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if errors.Is(err, ErrAuthenticationFailed) {
		s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	s.logger.Error(err, "unexpected error resolving webhook job")
	s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
}

func (s *Server) handleUpdateProjectStatusError(w http.ResponseWriter, err error, project, job, namespace string) bool {
	if err == nil {
		return false
	}

	s.logger.Error(err, "Failed to process webhook for project", "project", project, "renovateJob", job, "namespace", namespace)
	if err == crdmanager.ErrProjectNotFound {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("project '%s' not found", project)})
		return true
	}

	s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to process webhook"})
	return true
}

func (server *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		server.logger.Error(err, "failed to write JSON response")
	}
}

func (s *Server) handleWebhookTrigger(w http.ResponseWriter, r *http.Request, body []byte, payload WebhookPayload) {
	if valid, reason := isValidWebhookPayload(payload); !valid {
		s.logger.Info("ignoring "+payload.Provider+" webhook event", "event", payload.Event, "repository", payload.Project, "reason", reason)
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored", "reason": reason})
		return
	}

	namespace := r.URL.Query().Get("namespace")
	jobName := r.URL.Query().Get("job")

	checker := buildAuthCheckerFromRequest(r, body, s.manager)
	jobId, err := FindAndAuthenticateJob(r.Context(), s.manager, namespace, jobName, payload.Project, checker)
	if err != nil {
		s.logger.Info("webhook resolve failed", "project", payload.Project, "error", err)
		s.handleResolverError(w, err)
		return
	}

	s.logger.Info("received "+payload.Provider+" event", "repository", payload.Project, "event", payload.Event, "action", payload.Action, "priority", 1)
	err = s.manager.UpdateProjectStatus(
		r.Context(),
		payload.Project,
		jobId,
		&types.RenovateStatusUpdate{
			Status:   api.JobStatusScheduled,
			Priority: 1,
		},
	)
	if s.handleUpdateProjectStatusError(w, err, payload.Project, jobId.Name, jobId.Namespace) {
		return
	}

	s.writeJSON(w, http.StatusAccepted, map[string]string{"message": "renovate job scheduled", "repository": payload.Project})
}
