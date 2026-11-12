package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/assert"
	"renovate-operator/config"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/policy"
	"renovate-operator/internal/renovate"
	"renovate-operator/internal/telemetry"
	"renovate-operator/internal/types"
	"renovate-operator/metricStore"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Server struct {
	manager          crdmanager.RenovateJobManager
	discovery        renovate.DiscoveryAgent
	logger           logr.Logger
	server           *http.Server
	installationSets client.Client
	policy           policy.Policy
}

// webhookProcessingTimeout bounds how long a delivery is processed once received, whether or
// not its sender is still waiting for the answer.
const webhookProcessingTimeout = 2 * time.Minute

// detachFromClient lets a delivery finish after its sender hangs up. Senders give up on their
// own clock (GitLab after 10 seconds by default) and the operator can need longer, e.g. when it
// is short of CPU. Tied to the request context, the work was cancelled midway: the first live
// API call, the webhook secret read, failed and the trigger was dropped instead of answered late.
func detachFromClient(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), webhookProcessingTimeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func NewWebookServer(manager crdmanager.RenovateJobManager, discovery renovate.DiscoveryAgent, logger logr.Logger) *Server {
	return &Server{
		manager:   manager,
		discovery: discovery,
		logger:    logger,
	}
}

func RegisterWebhookRoutes(router *mux.Router, server *Server) {
	assert.Assert(server.manager != nil, "failed to register webhook routes. manager must not be nil")

	sub := router.PathPrefix("/webhook/v1").Subrouter()
	sub.Use(telemetry.MuxMiddleware("renovate-operator-webhook"), detachFromClient)
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

// runRenovate handles /schedule: one project, or with all=true every project of the job named
// exactly by job and namespace (see AuthenticateJobForAllProjects).
func (s *Server) runRenovate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	const provider = "schedule"

	query := r.URL.Query()
	project := query.Get("project")
	namespace := query.Get("namespace")
	jobName := query.Get("job")

	var all bool
	var invalid string
	if v := query.Get("all"); v != "" {
		var err error
		if all, err = strconv.ParseBool(v); err != nil {
			invalid = "invalid all query parameter, expected true or false"
		}
	}
	switch {
	case invalid != "":
	case all && project != "":
		invalid = "project and all query parameters are mutually exclusive"
	case all && (namespace == "" || jobName == ""):
		invalid = "all requires the job and namespace query parameters"
	case !all && project == "":
		invalid = "missing project query parameter"
	}
	if invalid != "" {
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": invalid})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read request body"})
		return
	}

	checker := buildAuthCheckerFromRequest(r, body, s.manager)
	var jobId crdmanager.RenovateJobIdentifier
	if all {
		jobId, err = AuthenticateJobForAllProjects(ctx, s.manager, namespace, jobName, checker)
	} else {
		jobId, err = FindAndAuthenticateJob(ctx, s.manager, namespace, jobName, project, checker)
	}
	if err != nil {
		s.recordResolverAuthFailure(ctx, provider, err, signatureWasUsed(r))
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.logger.Info("webhook resolve failed", "project", project, "all", all, "renovateJob", jobName, "namespace", namespace, "error", err)
		s.handleResolverError(w, err)
		return
	}

	status := &types.RenovateStatusUpdate{
		Status:   api.JobStatusScheduled,
		Priority: 1,
	}
	if all {
		isNotRunning := func(p crdmanager.RenovateProjectStatus) bool { return p.Status != api.JobStatusRunning }
		if err := s.manager.UpdateProjectStatusBatched(ctx, isNotRunning, jobId, status); err != nil {
			// The batch goes on past a failed project, so the others may already be scheduled;
			// repeating the request is safe.
			s.logger.Error(err, "Failed to schedule all projects", "renovateJob", jobId.Name, "namespace", jobId.Namespace)
			metricStore.IncWebhookRequest(ctx, provider, "rejected")
			s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to schedule all projects"})
			return
		}
	} else {
		err = s.manager.UpdateProjectStatus(ctx, project, jobId, status)
		if s.handleUpdateProjectStatusError(w, err, project, jobId.Name, jobId.Namespace) {
			metricStore.IncWebhookRequest(ctx, provider, "rejected")
			return
		}
	}

	metricStore.IncWebhookRequest(ctx, provider, "accepted")
	w.WriteHeader(http.StatusOK)
	s.logger.V(2).Info("Successfully triggered Renovate", "project", project, "all", all, "renovateJob", jobId.Name, "namespace", jobId.Namespace, "priority", 1)
}

// recordResolverAuthFailure emits the appropriate webhook auth-failure metric for an
// error returned by FindAndAuthenticateJob or AuthenticateJobForAllProjects:
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
	if errors.Is(err, ErrAuthenticationFailed) || errors.Is(err, ErrNoMatchingJob) {
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
