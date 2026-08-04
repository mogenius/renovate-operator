package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/renovate"
	"renovate-operator/internal/telemetry"
	"renovate-operator/internal/types"
	"renovate-operator/internal/utils"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/api/errors"
)

type RenovateJobInfo struct {
	Name             string                             `json:"name"`
	Namespace        string                             `json:"namespace"`
	CronExpression   string                             `json:"cronExpression"`
	NextSchedule     time.Time                          `json:"nextSchedule"`
	DiscoveryStatus  api.RenovateProjectStatus          `json:"discoveryStatus"`
	Projects         []crdmanager.RenovateProjectStatus `json:"projects"`
	Platform         string                             `json:"platform,omitempty"`
	PlatformEndpoint string                             `json:"platformEndpoint,omitempty"`
	ExecutionOptions *ExecutionOptions                  `json:"executionOptions,omitempty"`
}

type ExecutionOptions struct {
	Debug bool `json:"debug,omitempty"`
}

// filterRenovateJobsByGroups filters jobs based on user groups and job's allowedGroups.
// Authorization rules:
// - When auth is disabled (authEnabled == false): all jobs visible
// - When auth is enabled (authEnabled == true):
//   - Jobs without allowedGroups use defaultAllowedGroups (from operator config)
//   - If defaultAllowedGroups is also empty, job is visible to all authenticated users
//   - Jobs with allowedGroups shown only if user has at least one matching group
//   - Users without groups see no jobs (unless the job has no group restrictions)
//   - If session is nil (edge case/bug), return empty list for security
func filterRenovateJobsByGroups(jobs []api.RenovateJob, authEnabled bool, session *sessionData, defaultAllowedGroups []string) []api.RenovateJob {
	// If auth is disabled, return all jobs
	if !authEnabled {
		return jobs
	}

	// If auth is enabled but no session, return empty for security (defense in depth)
	if session == nil {
		return []api.RenovateJob{}
	}

	userGroups := session.Groups
	if userGroups == nil {
		userGroups = []string{}
	}

	filtered := make([]api.RenovateJob, 0, len(jobs))
	for _, job := range jobs {
		// Determine effective allowed groups for this job
		effectiveAllowedGroups := normalizeGroups(job.Spec.AllowedGroups)
		if len(effectiveAllowedGroups) == 0 {
			// Use default groups if job has no explicit allowedGroups
			effectiveAllowedGroups = defaultAllowedGroups
		}

		// If no effective groups (neither job-specific nor defaults), job is visible to all authenticated users
		if len(effectiveAllowedGroups) == 0 {
			filtered = append(filtered, job)
			continue
		}

		// Check if user has any matching group
		if hasIntersection(userGroups, effectiveAllowedGroups) {
			filtered = append(filtered, job)
		}
	}
	return filtered
}

// hasIntersection returns true if there's at least one common element between two string slices.
// Uses a map for O(n+m) performance, optimized for scenarios with large group lists.
func hasIntersection(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}

	// Use the smaller slice for the map to reduce memory
	if len(a) > len(b) {
		a, b = b, a
	}

	set := make(map[string]struct{}, len(a))
	for _, item := range a {
		set[item] = struct{}{}
	}

	for _, item := range b {
		if _, exists := set[item]; exists {
			return true
		}
	}
	return false
}

// authorizeAndGetJob checks authorization and returns the job if authorized.
// Returns (job, true) if authorized, (nil, false) otherwise.
// This avoids duplicate K8s API calls by returning the job for reuse.
func (s *Server) authorizeAndGetJob(r *http.Request, namespace, jobName string) (*api.RenovateJob, bool) {
	// If auth is disabled, fetch and return the job
	if s.auth == nil {
		job, err := s.manager.GetRenovateJob(r.Context(), jobName, namespace)
		if err != nil || job == nil {
			return nil, false
		}
		return job, true
	}

	// Get session from context
	session := getSessionFromContext(r)
	if session == nil {
		// Auth enabled but no session - deny access (should not happen due to middleware)
		s.logger.Info("Authorization denied: no session in context",
			"action", "job_access",
			"resource", jobName,
			"namespace", namespace,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr)
		return nil, false
	}

	// Get the job
	job, err := s.manager.GetRenovateJob(r.Context(), jobName, namespace)
	if err != nil || job == nil {
		// Return false for both "not found" and "error" to prevent information disclosure
		s.logger.V(1).Info("Authorization check: job not found or error",
			"user", session.Email,
			"resource", jobName,
			"namespace", namespace,
			"error", err)
		return nil, false
	}

	// Determine effective allowed groups for this job
	effectiveAllowedGroups := normalizeGroups(job.Spec.AllowedGroups)
	if len(effectiveAllowedGroups) == 0 {
		// Use default groups if job has no explicit allowedGroups
		effectiveAllowedGroups = s.defaultAllowedGroups
	}

	// If no effective groups (neither job-specific nor defaults), job is visible to all authenticated users
	if len(effectiveAllowedGroups) == 0 {
		s.logger.V(1).Info("Authorization granted: job has no group restrictions",
			"user", session.Email,
			"resource", jobName,
			"namespace", namespace,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr)
		return job, true
	}

	// Check if user has any matching group
	userGroups := session.Groups
	if userGroups == nil {
		userGroups = []string{}
	}

	authorized := hasIntersection(userGroups, effectiveAllowedGroups)

	// Audit log the authorization decision
	if authorized {
		s.logger.V(1).Info("Authorization granted",
			"user", session.Email,
			"user_groups", session.Groups,
			"resource", jobName,
			"namespace", namespace,
			"allowed_groups", effectiveAllowedGroups,
			"path", r.URL.Path,
			"method", r.Method,
			"remote_addr", r.RemoteAddr)
		return job, true
	}

	s.logger.Info("Authorization denied: no matching groups",
		"user", session.Email,
		"user_groups", session.Groups,
		"resource", jobName,
		"namespace", namespace,
		"allowed_groups", effectiveAllowedGroups,
		"path", r.URL.Path,
		"method", r.Method,
		"remote_addr", r.RemoteAddr)

	return nil, false
}

// authorizeJobAccess checks if the current user is authorized to access the given RenovateJob.
// Returns true if authorized, false otherwise. Includes comprehensive audit logging.
// For endpoints that need the job after authorization, use authorizeAndGetJob to avoid duplicate fetches.
func (s *Server) authorizeJobAccess(r *http.Request, namespace, jobName string) bool {
	_, authorized := s.authorizeAndGetJob(r, namespace, jobName)
	return authorized
}

func (s *Server) registerApiV1Routes(router *mux.Router) {
	apiV1 := router.PathPrefix("/api/v1").Subrouter()
	apiV1.Use(telemetry.MuxMiddleware("renovate-operator-ui-api-v1"))
	apiV1.HandleFunc("/version", s.getVersion).Methods("GET")
	apiV1.HandleFunc("/renovatejobs", s.getRenovateJobs).Methods("GET")
	apiV1.HandleFunc("/renovate", s.runRenovateForProject).Methods("POST")
	apiV1.HandleFunc("/renovate/all", s.runRenovateForAllProjects).Methods("POST")
	apiV1.HandleFunc("/renovate/cancel", s.cancelRenovateForProject).Methods("POST")
	apiV1.HandleFunc("/logs", s.getRenovateJobLogs).Methods("GET")
	apiV1.HandleFunc("/discovery/start", s.runDiscoveryForProject).Methods("POST")
	apiV1.HandleFunc("/discovery/status", s.discoveryStatusForProject).Methods("GET")
	apiV1.HandleFunc("/executionOptions", s.updateExecutionOptions).Methods("POST")
}

func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Version string `json:"version"`
	}{
		Version: s.version,
	})
}

func (s *Server) getRenovateJobs(w http.ResponseWriter, r *http.Request) {
	renovateJobs, err := s.manager.ListRenovateJobsFull(r.Context())
	if err != nil {
		internalServerError(w, err, "failed to load renovatejobs")
		return
	}

	// Filter jobs based on user's groups
	authEnabled := s.auth != nil
	session := getSessionFromContext(r)
	renovateJobs = filterRenovateJobsByGroups(renovateJobs, authEnabled, session, s.defaultAllowedGroups)

	result := make([]RenovateJobInfo, 0)
	for i := range renovateJobs {
		renovateJob := &renovateJobs[i]

		discoveryStatus, err := s.discovery.GetDiscoveryJobStatus(r.Context(), renovateJob)
		if err != nil {
			if errors.IsNotFound(err) {
				discoveryStatus = api.JobStatusScheduled
			} else {
				// it might not be failed, but we dont want to block the whole response
				discoveryStatus = api.JobStatusFailed
			}
		}

		platform, _ := utils.GetPlatformAndEndpoint(renovateJob.Spec.Provider)
		platformEndpoint := utils.GetPublicEndpoint(renovateJob.Spec.Provider)

		projects := make([]crdmanager.RenovateProjectStatus, 0, len(renovateJob.Status.Projects))
		for _, p := range renovateJob.Status.Projects {
			projects = append(projects, crdmanager.RenovateProjectStatus{
				Name:                 p.Name,
				Status:               p.Status,
				LastTransition:       crdmanager.NonZeroTime(p.LastTransition.Time),
				Priority:             p.Priority,
				RenovateResultStatus: p.RenovateResultStatus,
				Duration:             p.Duration,
				PRActivity:           p.PRActivity,
				LogIssues:            p.LogIssues,
			})
		}

		result = append(result, RenovateJobInfo{
			Name:             renovateJob.Name,
			Namespace:        renovateJob.Namespace,
			NextSchedule:     s.scheduler.GetNextRunOnSchedule(renovateJob.Spec.Schedule, renovateJob.Fullname()),
			Projects:         projects,
			CronExpression:   renovateJob.Spec.Schedule,
			DiscoveryStatus:  discoveryStatus,
			Platform:         platform,
			PlatformEndpoint: platformEndpoint,
			ExecutionOptions: &ExecutionOptions{
				Debug: renovateJob.Status.ExecutionOptions != nil && renovateJob.Status.ExecutionOptions.Debug,
			},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) getRenovateJobLogs(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	renovate := r.URL.Query().Get("renovate")
	project := r.URL.Query().Get("project")

	if !s.authorizeJobAccess(r, namespace, renovate) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	stream, err := s.manager.StreamLogsForProject(
		r.Context(),
		crdmanager.RenovateJobIdentifier{
			Name:      renovate,
			Namespace: namespace,
		},
		project,
	)
	if err != nil {
		internalServerError(w, err, "failed to get logs for project, probably the completed job has been cleaned up already")
		return
	}
	defer func() { _ = stream.Close() }()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, _ := w.(http.Flusher)

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MB per line — Renovate logs can be verbose
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !json.Valid([]byte(line)) {
			continue
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", line); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	// scanner.Err() is nil on clean EOF (pod exited, log store exhausted) or context cancellation.
	// Either way we send the done event so the client closes its EventSource.
	_ = scanner.Err()

	if _, err := fmt.Fprint(w, "event: done\ndata: {}\n\n"); err != nil {
		return
	}
	if flusher != nil {
		flusher.Flush()
	}
}

func getRenovateJsonBody(r *http.Request) (*struct {
	name      string
	namespace string
	project   string
}, error,
) {
	var renovateJob, namespace, project string
	if r.Header.Get("Content-Type") == "application/json" {
		var params struct {
			RenovateJob string `json:"renovateJob"`
			Namespace   string `json:"namespace"`
			Project     string `json:"project"`
		}
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			return nil, err
		}
		renovateJob = params.RenovateJob
		namespace = params.Namespace
		project = params.Project
	} else {
		// fallback to form values
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		renovateJob = r.FormValue("renovateJob")
		namespace = r.FormValue("namespace")
		project = r.FormValue("project")
	}

	return &struct {
		name      string
		namespace string
		project   string
	}{
		name:      renovateJob,
		namespace: namespace,
		project:   project,
	}, nil
}

func (s *Server) runRenovateForProject(w http.ResponseWriter, r *http.Request) {
	// Expect application/json or form values
	params, err := getRenovateJsonBody(r)
	if err != nil {
		badRequestError(w, err, "failed to parse request body")
		return
	}

	if params.name == "" || params.namespace == "" || params.project == "" {
		badRequestError(w, err, "Missing parameters")
		return
	}

	// Authorization check
	if !s.authorizeJobAccess(r, params.namespace, params.name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	err = s.manager.UpdateProjectStatus(
		r.Context(),
		params.project,
		crdmanager.RenovateJobIdentifier{
			Name:      params.name,
			Namespace: params.namespace,
		},
		&types.RenovateStatusUpdate{
			Status:   api.JobStatusScheduled,
			Priority: 2,
		},
	)
	if err != nil {
		s.logger.Error(err, "Failed to run Renovate for project", "project", params.project, "renovateJob", params.name, "namespace", params.namespace)
		internalServerError(w, err, "failed to run Renovate for project")
		return
	}

	writeSuccess(w, SuccessResult{Message: "Renovate job triggered for project"})
	s.logger.V(2).Info("Successfully triggered Renovate for project", "project", params.project, "renovateJob", params.name, "namespace", params.namespace, "priority", 2)
}

func (s *Server) cancelRenovateForProject(w http.ResponseWriter, r *http.Request) {
	params, err := getRenovateJsonBody(r)
	if err != nil {
		badRequestError(w, err, "failed to parse request body")
		return
	}

	if params.name == "" || params.namespace == "" || params.project == "" {
		badRequestError(w, err, "Missing parameters")
		return
	}

	if !s.authorizeJobAccess(r, params.namespace, params.name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	err = s.manager.CancelProjectJob(
		r.Context(),
		params.project,
		crdmanager.RenovateJobIdentifier{
			Name:      params.name,
			Namespace: params.namespace,
		},
	)
	if err != nil {
		s.logger.Error(err, "Failed to cancel Renovate for project", "project", params.project, "renovateJob", params.name, "namespace", params.namespace)
		internalServerError(w, err, "failed to cancel Renovate for project")
		return
	}

	writeSuccess(w, SuccessResult{Message: "Renovate job cancelled for project"})
	s.logger.V(2).Info("Successfully cancelled Renovate for project", "project", params.project, "renovateJob", params.name, "namespace", params.namespace)
}

func (s *Server) runRenovateForAllProjects(w http.ResponseWriter, r *http.Request) {
	params, err := getRenovateJsonBody(r)
	if err != nil {
		badRequestError(w, err, "failed to parse request body")
		return
	}

	if params.name == "" || params.namespace == "" {
		badRequestError(w, err, "Missing parameters")
		return
	}

	if !s.authorizeJobAccess(r, params.namespace, params.name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	jobIdentifier := crdmanager.RenovateJobIdentifier{
		Name:      params.name,
		Namespace: params.namespace,
	}

	err = s.manager.UpdateProjectStatusBatched(
		r.Context(),
		func(p api.ProjectStatus) bool {
			return p.Status != api.JobStatusRunning && p.Status != api.JobStatusScheduled
		},
		jobIdentifier,
		&types.RenovateStatusUpdate{
			Status:   api.JobStatusScheduled,
			Priority: 2,
		},
	)
	if err != nil {
		s.logger.Error(err, "Failed to trigger all projects", "renovateJob", params.name, "namespace", params.namespace)
		internalServerError(w, err, "failed to trigger all projects")
		return
	}

	writeSuccess(w, SuccessResult{Message: "All projects triggered"})
	s.logger.V(2).Info("Successfully triggered all projects", "renovateJob", params.name, "namespace", params.namespace)
}

func (s *Server) runDiscoveryForProject(w http.ResponseWriter, r *http.Request) {
	params, err := getRenovateJsonBody(r)
	if err != nil {
		badRequestError(w, err, "failed to parse request body")
		return
	}

	if params.name == "" || params.namespace == "" {
		badRequestError(w, err, "missing parameters")
		return
	}

	// Authorization check (returns job to avoid duplicate K8s API call)
	job, authorized := s.authorizeAndGetJob(r, params.namespace, params.name)
	if !authorized {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if job == nil {
		internalServerError(w, nil, "failed to get renovate job")
		return
	}

	ctx := r.Context()
	// discovery mus only run once
	status, err := s.discovery.GetDiscoveryJobStatus(ctx, job)
	if err == nil && status == api.JobStatusRunning {
		// discovery job is already running
		writeSuccess(w, SuccessResult{Message: "discovery job is already running"})
		return
	}

	if job.Spec.GithubEnterpriseAppReference != nil {
		secretNames, err := s.githubApp.EnsureTokensForEnterpriseApp(ctx, job)
		if err != nil {
			internalServerError(w, err, "failed to ensure enterprise github app tokens")
			return
		}
		for _, secretName := range secretNames {
			if _, err := s.discovery.CreateDiscoveryJob(ctx, *job, renovate.DiscoveryJobOptions{
				TriggerAllProjects: false,
				TokenSecretName:    secretName,
			}); err != nil {
				s.logger.Error(err, "Failed to start discovery for installation", "secretName", secretName)
				internalServerError(w, err, "failed to create discovery job")
				return
			}
		}
	} else {
		if _, err := s.discovery.CreateDiscoveryJob(ctx, *job, renovate.DiscoveryJobOptions{TriggerAllProjects: false}); err != nil {
			s.logger.Error(err, "Failed to start discovery for RenovateJob", "renovateJob", params.name, "namespace", params.namespace)
			internalServerError(w, err, "failed to create discovery job")
			return
		}
	}

	writeSuccess(w, SuccessResult{Message: "discovery job started"})
	s.logger.V(2).Info("Successfully started discovery for RenovateJob", "renovateJob", params.name, "namespace", params.namespace)
}

func (s *Server) updateExecutionOptions(w http.ResponseWriter, r *http.Request) {
	var params struct {
		RenovateJob string `json:"renovateJob"`
		Namespace   string `json:"namespace"`
		Debug       bool   `json:"debug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		badRequestError(w, err, "failed to parse request body")
		return
	}
	if params.RenovateJob == "" || params.Namespace == "" {
		badRequestError(w, nil, "missing parameters")
		return
	}

	if !s.authorizeJobAccess(r, params.Namespace, params.RenovateJob) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	err := s.manager.UpdateExecutionOptions(
		r.Context(),
		crdmanager.RenovateJobIdentifier{
			Name:      params.RenovateJob,
			Namespace: params.Namespace,
		},
		&api.RenovateExecutionOptions{
			Debug: params.Debug,
		},
	)
	if err != nil {
		s.logger.Error(err, "Failed to update execution options", "renovateJob", params.RenovateJob, "namespace", params.Namespace)
		internalServerError(w, err, "failed to update execution options")
		return
	}

	writeSuccess(w, SuccessResult{Message: "execution options updated"})
}

func (s *Server) discoveryStatusForProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	namespace := r.URL.Query().Get("namespace")
	renovate := r.URL.Query().Get("renovate")

	// Authorization check (returns job to avoid duplicate K8s API call)
	job, authorized := s.authorizeAndGetJob(r, namespace, renovate)
	if !authorized {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if job == nil {
		internalServerError(w, nil, "failed to get renovate job")
		return
	}
	status, err := s.discovery.GetDiscoveryJobStatus(ctx, job)
	if err != nil {
		if errors.IsNotFound(err) {
			status = api.JobStatusScheduled
		} else {
			internalServerError(w, err, "failed to get discovery job status")
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Status api.RenovateProjectStatus `json:"status"`
	}{
		Status: status,
	})
}
