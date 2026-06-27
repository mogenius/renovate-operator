package webhook

import (
	"context"
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
	"renovate-operator/metricStore"

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
	router.Use(telemetry.MuxMiddleware("renovate-operator-webhook"))
	sub := router.PathPrefix("/webhook/v1").Subrouter()
	sub.HandleFunc("/schedule", s.runRenovate).Methods("POST")
	sub.HandleFunc("/gitlab", s.gitLabWebhook).Methods("POST")
	sub.HandleFunc("/github", s.githubWebhook).Methods("POST")
	sub.HandleFunc("/forgejo", s.forgejoWebhook).Methods("POST")

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
	ctx := r.Context()
	const provider = "schedule"

	project := r.URL.Query().Get("project")
	if project == "" {
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing project query parameter"})
		return
	}

	namespace := r.URL.Query().Get("namespace")
	jobName := r.URL.Query().Get("job")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read request body"})
		return
	}

	checker := buildAuthCheckerFromRequest(r, body, s.manager)
	jobId, err := FindAndAuthenticateJob(ctx, s.manager, namespace, jobName, project, checker)
	if err != nil {
		s.recordResolverAuthFailure(ctx, provider, err, signatureWasUsed(r))
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.handleResolverError(w, err)
		return
	}

	err = s.manager.UpdateProjectStatus(
		ctx,
		project,
		jobId,
		&types.RenovateStatusUpdate{
			Status:   api.JobStatusScheduled,
			Priority: 1,
		},
	)
	if s.handleUpdateProjectStatusError(w, err, project, jobId.Name, jobId.Namespace) {
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		return
	}

	metricStore.IncWebhookRequest(ctx, provider, "accepted")
	w.WriteHeader(http.StatusOK)
	s.logger.V(2).Info("Successfully triggered Renovate for project", "project", project, "renovateJob", jobId.Name, "namespace", jobId.Namespace, "priority", 1)
}

// recordResolverAuthFailure emits the appropriate webhook auth-failure metric for an
// error returned by FindAndAuthenticateJob:
//   - ErrNoMatchingJob       -> "no_matching_job"
//   - ErrAuthenticationFailed -> "auth_failed"
//   - any other surfaced error (e.g. secret/credential resolution) -> "secret_error"
//
// When authentication failed and an HMAC signature header (rather than a bearer
// token) was the credential actually used, it additionally records a signature
// verification failure. This is a heuristic: the resolver collapses signature and
// token mismatches into ErrAuthenticationFailed, so the signature failure is only
// counted on the path where a signature header was supplied and used.
func (s *Server) recordResolverAuthFailure(ctx context.Context, provider string, err error, signatureUsed bool) {
	switch {
	case errors.Is(err, ErrNoMatchingJob):
		metricStore.IncWebhookAuthFailure(ctx, provider, "no_matching_job")
	case errors.Is(err, ErrAuthenticationFailed):
		metricStore.IncWebhookAuthFailure(ctx, provider, "auth_failed")
		if signatureUsed {
			metricStore.IncWebhookSignatureFailure(ctx, provider)
		}
	default:
		metricStore.IncWebhookAuthFailure(ctx, provider, "secret_error")
	}
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
