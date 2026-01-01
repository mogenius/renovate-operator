package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	sub.HandleFunc("/gitlab", s.gitLabWebhook).Methods("POST")
	sub.HandleFunc("/github", s.githubWebhook).Methods("POST")

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
	if namespace == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing namespace query parameter"})
		return
	}
	job := r.URL.Query().Get("job")
	if job == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing job query parameter"})
		return
	}
	project := r.URL.Query().Get("project")
	if project == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing project query parameter"})
		return
	}

	err := s.manager.UpdateProjectStatus(
		r.Context(),
		project,
		crdmanager.RenovateJobIdentifier{
			Name:      job,
			Namespace: namespace,
		},
		api.JobStatusScheduled,
	)
	if err != nil {
		s.logger.Error(err, "Failed to run Renovate for project", "project", project, "renovateJob", job, "namespace", namespace)
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to run renovate for project"})
		return
	}

	w.WriteHeader(http.StatusOK)
	s.logger.Info("Successfully triggered Renovate for project", "project", project, "renovateJob", job, "namespace", namespace)
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

		if renovateJob.Spec.Webhook == nil || !renovateJob.Spec.Webhook.Enabled {
			server.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "webhook not enabled for this renovate job"})
			return
		}

		if renovateJob.Spec.Webhook.Authentication == nil || !renovateJob.Spec.Webhook.Authentication.Enabled {
			server.logger.Info("Webhook authentication not enabled, skipping auth")
			next.ServeHTTP(w, r)
			return
		}

		// Bearer Token authentication
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			authHeader = r.Header.Get("X-Gitlab-Token")
		}
		if authHeader != "" {
			valid, reason := server.validateBearerToken(r.Context(), namespace, job, authHeader)
			if !valid {
				server.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": reason})
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// Fallback to X-Hub-Signature-256 for GitHub compatibility
		signature := r.Header.Get("X-Hub-Signature-256")

		if signature != "" {
			valid, reason := server.validateSignature(r.Context(), r, namespace, job, signature)
			if !valid {
				server.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": reason})
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		server.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "no valid authentication method provided"})
	})
}

func (server *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		server.logger.Error(err, "failed to write JSON response")
	}
}

func (server *Server) validateBearerToken(ctx context.Context, namespace, job, authHeader string) (bool, string) {
	if authHeader == "" {
		return false, "missing authorization header"
	}

	// Check if the header has the Bearer prefix

	token := authHeader
	if strings.HasPrefix(authHeader, "Bearer ") {
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			return false, "invalid authorization header format"
		}
		token = parts[1]
	}
	token = strings.TrimSpace(token)

	valid, err := server.manager.IsWebhookTokenValid(ctx, crdmanager.RenovateJobIdentifier{
		Name:      job,
		Namespace: namespace,
	}, token)
	if err != nil {
		return false, err.Error()
	}
	if !valid {
		return false, "invalid token"
	}
	return true, ""
}

func (server *Server) validateSignature(ctx context.Context, r *http.Request, namespace, job, signature string) (bool, string) {

	if signature == "" {
		return false, "missing signature header"
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return false, "failed to read request body"
	}
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	valid, err := server.manager.IsWebhookSignatureValid(ctx, crdmanager.RenovateJobIdentifier{
		Name:      job,
		Namespace: namespace,
	}, signature, body)
	if err != nil || !valid {
		return false, "invalid signature"
	}
	return true, ""
}
