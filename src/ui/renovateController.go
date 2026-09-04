package ui

import (
	"bufio"
	"context"
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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// Accepted is false when the operator's policy refuses this job, in which case
	// nothing runs for it and AcceptedMessage says what to fix. Jobs reconciled by an
	// older operator have no condition yet and are reported as accepted.
	Accepted         bool     `json:"accepted"`
	AcceptedMessage  string   `json:"acceptedMessage,omitempty"`
	Role             string   `json:"role,omitempty"`
	Permissions      []string `json:"permissions"`
	DiscoveryFilters []string `json:"discoveryFilters,omitempty"`
	DiscoverTopics   []string `json:"discoverTopics,omitempty"`
}

func (s *Server) decideJobAccess(r *http.Request, job *api.RenovateJob) accessDecision {
	if s.auth == nil {
		return adminDecision()
	}
	return resolveAccess(job, getSessionFromContext(r), s.accessDefaults, s.logger)
}

// checkAccessEnforceable reports whether the configured access rules can be
// enforced, given the full set of jobs, and refreshes the cached verdict.
func (s *Server) checkAccessEnforceable(jobs []api.RenovateJob) *AccessMisconfiguration {
	misconfiguration, jobsWithGroups := detectAccessMisconfiguration(s.auth, s.accessDefaults, jobs)

	// Logged only on a transition, because the dashboard polls: repeating this
	// for every request, or even once per cache period, buries the log.
	if s.accessCheck.store(misconfiguration) {
		if misconfiguration != nil {
			s.logger.Error(nil, "access rules cannot be enforced, hiding every RenovateJob",
				"reason", misconfiguration.Reason,
				"detail", misconfiguration.Message,
				"defaultReaderGroups", s.accessDefaults.ReaderGroups,
				"defaultAdminGroups", s.accessDefaults.AdminGroups,
				"jobsWithGroups", jobsWithGroups)
		} else {
			s.logger.Info("access rules are enforceable again, RenovateJobs are visible")
		}
	}

	return misconfiguration
}

// accessEnforceable is checkAccessEnforceable for the endpoints that hold a
// single job and therefore have no job list of their own.
//
// The verdict is cached for accessCheckTTL. Without it every request to the
// endpoints the dashboard polls -- and which anonymous read exposes without a
// session -- would take the manager's global lock to list every RenovateJob.
func (s *Server) accessEnforceable(ctx context.Context) *AccessMisconfiguration {
	if s.auth == nil || s.auth.SupportsGroups() {
		return nil
	}

	if verdict, ok := s.accessCheck.load(); ok {
		return verdict
	}

	jobs, err := s.manager.ListRenovateJobsFull(ctx)
	if err != nil {
		// Cannot prove the configuration is unenforceable, so do not treat it as
		// such: the per-job resolution still fails closed.
		s.logger.Error(err, "failed to list renovatejobs to validate access rules, continuing")
		return nil
	}
	return s.checkAccessEnforceable(jobs)
}

func (s *Server) filterReadableJobs(r *http.Request, jobs []api.RenovateJob) ([]api.RenovateJob, []accessDecision) {
	if s.checkAccessEnforceable(jobs) != nil {
		return nil, nil
	}

	readable := make([]api.RenovateJob, 0, len(jobs))
	decisions := make([]accessDecision, 0, len(jobs))
	for i := range jobs {
		decision := s.decideJobAccess(r, &jobs[i])
		if !decision.canRead() {
			continue
		}
		readable = append(readable, jobs[i])
		decisions = append(decisions, decision)
	}
	return readable, decisions
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

// resolveJobAccess fetches a job and evaluates the request's access to it.
// Returns the job for reuse so callers do not fetch it twice.
func (s *Server) resolveJobAccess(r *http.Request, namespace, jobName string) (*api.RenovateJob, accessDecision) {
	if s.accessEnforceable(r.Context()) != nil {
		return nil, accessDecision{}
	}

	job, err := s.manager.GetRenovateJob(r.Context(), jobName, namespace)
	if err != nil || job == nil {
		// Treat "not found" and "error" alike to prevent information disclosure
		s.logger.V(1).Info("Access check: job not found or error",
			"user", sessionEmail(r),
			"resource", jobName,
			"namespace", namespace,
			"error", err)
		return nil, accessDecision{}
	}

	decision := s.decideJobAccess(r, job)

	// Audit log the authorization decision
	s.logger.V(1).Info("Access resolved",
		"user", sessionEmail(r),
		"role", decision.Role.String(),
		"resource", jobName,
		"namespace", namespace,
		"path", r.URL.Path,
		"method", r.Method,
		"remote_addr", r.RemoteAddr)

	return job, decision
}

func (s *Server) requireRead(w http.ResponseWriter, r *http.Request, namespace, jobName string) (*api.RenovateJob, bool) {
	job, decision := s.resolveJobAccess(r, namespace, jobName)
	if !decision.canRead() {
		http.Error(w, "not found", http.StatusNotFound)
		return nil, false
	}
	return job, true
}

func (s *Server) requirePermission(w http.ResponseWriter, r *http.Request, namespace, jobName, permission string) (*api.RenovateJob, bool) {
	job, decision := s.resolveJobAccess(r, namespace, jobName)
	if !decision.canRead() {
		http.Error(w, "not found", http.StatusNotFound)
		return nil, false
	}

	if !decision.has(permission) {
		s.logger.Info("Access denied: missing permission",
			"user", sessionEmail(r),
			"role", decision.Role.String(),
			"permission", permission,
			"resource", jobName,
			"namespace", namespace,
			"path", r.URL.Path,
			"method", r.Method,
			"remote_addr", r.RemoteAddr)
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}

	return job, true
}

// sessionEmail returns the session's email for audit logs, or "anonymous" when
// the request carries no session.
func sessionEmail(r *http.Request) string {
	if session := getSessionFromContext(r); session != nil {
		return session.Email
	}
	return "anonymous"
}

func (s *Server) registerApiV1Routes(router *mux.Router) {
	apiV1 := router.PathPrefix("/api/v1").Subrouter()
	apiV1.Use(telemetry.MuxMiddleware("renovate-operator-ui-api-v1"))
	apiV1.HandleFunc("/version", s.getVersion).Methods("GET")
	apiV1.HandleFunc("/access/status", s.getAccessStatus).Methods("GET")
	apiV1.HandleFunc("/setup/status", s.getSetupStatus).Methods("GET")
	apiV1.HandleFunc("/setup/secret", s.getSetupSecretCheck).Methods("GET")
	apiV1.HandleFunc("/renovatejobs", s.getRenovateJobs).Methods("GET")
	apiV1.HandleFunc("/renovate", s.runRenovateForProject).Methods("POST")
	apiV1.HandleFunc("/renovate/all", s.runRenovateForAllProjects).Methods("POST")
	apiV1.HandleFunc("/renovate/cancel", s.cancelRenovateForProject).Methods("POST")
	apiV1.HandleFunc("/logs", s.getRenovateJobLogs).Methods("GET")
	apiV1.HandleFunc("/discovery/start", s.runDiscoveryForProject).Methods("POST")
	apiV1.HandleFunc("/discovery/status", s.discoveryStatusForProject).Methods("GET")
}

func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Version string `json:"version"`
	}{
		Version: s.version,
	})
}

// getAccessStatus reports whether the operator can enforce its access rules,
// and warns when jobs exist that the rules hide from absolutely everyone.
func (s *Server) getAccessStatus(w http.ResponseWriter, r *http.Request) {
	result := struct {
		Misconfigured bool                    `json:"misconfigured"`
		Reason        string                  `json:"reason,omitempty"`
		Message       string                  `json:"message,omitempty"`
		Warning       *AccessMisconfiguration `json:"warning,omitempty"`
	}{}

	if misconfiguration := s.accessEnforceable(r.Context()); misconfiguration != nil {
		result.Misconfigured = true
		result.Reason = misconfiguration.Reason
		result.Message = misconfiguration.Message
	} else {
		result.Warning = s.lockedOutJobsWarning(r.Context())
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// lockedOutJobsWarning checks for jobs no one can see, cached like the
// misconfiguration verdict because this endpoint is public and polled.
func (s *Server) lockedOutJobsWarning(ctx context.Context) *AccessMisconfiguration {
	if s.auth == nil || s.accessDefaults.AuthorizationDisabled {
		return nil
	}

	if verdict, ok := s.lockoutCheck.load(); ok {
		return verdict
	}

	jobs, err := s.manager.ListRenovateJobsFull(ctx)
	if err != nil {
		// No list, no verdict — an API blip must not fabricate or clear a warning.
		s.logger.Error(err, "failed to list renovatejobs to check for locked-out jobs, continuing")
		return nil
	}

	verdict, lockedJobs := detectLockedOutJobs(s.auth, s.accessDefaults, jobs)
	if s.lockoutCheck.store(verdict) {
		if verdict != nil {
			s.logger.Error(nil, "RenovateJobs exist that grant no one access",
				"reason", verdict.Reason,
				"lockedJobs", lockedJobs)
		} else {
			s.logger.Info("every RenovateJob grants someone access again")
		}
	}
	return verdict
}

func (s *Server) getRenovateJobs(w http.ResponseWriter, r *http.Request) {
	renovateJobs, err := s.manager.ListRenovateJobsFull(r.Context())
	if err != nil {
		internalServerError(w, err, "failed to load renovatejobs")
		return
	}

	renovateJobs, decisions := s.filterReadableJobs(r, renovateJobs)

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

		jobId := crdmanager.RenovateJobIdentifier{Name: renovateJob.Name, Namespace: renovateJob.Namespace}
		projects, projErr := s.manager.GetProjectsForRenovateJob(r.Context(), jobId)
		if projErr != nil {
			s.logger.Error(projErr, "failed to get projects for job", "job", renovateJob.Name)
			projects = []crdmanager.RenovateProjectStatus{}
		}

		accepted, acceptedMessage := acceptedState(renovateJob)

		result = append(result, RenovateJobInfo{
			Name:             renovateJob.Name,
			Namespace:        renovateJob.Namespace,
			Accepted:         accepted,
			AcceptedMessage:  acceptedMessage,
			NextSchedule:     s.scheduler.GetNextRunOnSchedule(renovateJob.Spec.Schedule, renovateJob.Fullname()),
			Projects:         projects,
			CronExpression:   renovateJob.Spec.Schedule,
			DiscoveryStatus:  discoveryStatus,
			Platform:         platform,
			PlatformEndpoint: platformEndpoint,
			Role:             decisions[i].Role.String(),
			Permissions:      decisions[i].permissions(),
			DiscoveryFilters: renovateJob.Spec.DiscoveryFilters,
			DiscoverTopics:   renovateJob.Spec.DiscoverTopics,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// acceptedState reads the Accepted condition.
func acceptedState(job *api.RenovateJob) (bool, string) {
	condition := meta.FindStatusCondition(job.Status.Conditions, api.ConditionAccepted)
	if condition == nil {
		return true, ""
	}
	return condition.Status != metav1.ConditionFalse, condition.Message
}

func (s *Server) getRenovateJobLogs(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	renovate := r.URL.Query().Get("renovate")
	project := r.URL.Query().Get("project")

	if _, ok := s.requirePermission(w, r, namespace, renovate, permLogs); !ok {
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
	var body struct {
		RenovateJob      string                        `json:"renovateJob"`
		Namespace        string                        `json:"namespace"`
		Project          string                        `json:"project"`
		ExecutionOptions *api.RenovateExecutionOptions `json:"executionOptions,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badRequestError(w, err, "failed to parse request body")
		return
	}

	if body.RenovateJob == "" || body.Namespace == "" || body.Project == "" {
		badRequestError(w, nil, "Missing parameters")
		return
	}

	if _, ok := s.requirePermission(w, r, body.Namespace, body.RenovateJob, permTrigger); !ok {
		return
	}

	err := s.manager.UpdateProjectStatus(
		r.Context(),
		body.Project,
		crdmanager.RenovateJobIdentifier{
			Name:      body.RenovateJob,
			Namespace: body.Namespace,
		},
		&types.RenovateStatusUpdate{
			Status:           api.JobStatusScheduled,
			Priority:         2,
			ExecutionOptions: body.ExecutionOptions,
		},
	)
	if err != nil {
		s.logger.Error(err, "Failed to run Renovate for project", "project", body.Project, "renovateJob", body.RenovateJob, "namespace", body.Namespace)
		internalServerError(w, err, "failed to run Renovate for project")
		return
	}

	writeSuccess(w, SuccessResult{Message: "Renovate job triggered for project"})
	s.logger.V(2).Info("Successfully triggered Renovate for project", "project", body.Project, "renovateJob", body.RenovateJob, "namespace", body.Namespace, "priority", 2)
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

	if _, ok := s.requirePermission(w, r, params.namespace, params.name, permCancel); !ok {
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
	var body struct {
		RenovateJob      string                        `json:"renovateJob"`
		Namespace        string                        `json:"namespace"`
		ExecutionOptions *api.RenovateExecutionOptions `json:"executionOptions,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badRequestError(w, err, "failed to parse request body")
		return
	}

	if body.RenovateJob == "" || body.Namespace == "" {
		badRequestError(w, nil, "Missing parameters")
		return
	}

	if _, ok := s.requirePermission(w, r, body.Namespace, body.RenovateJob, permTriggerAll); !ok {
		return
	}

	jobIdentifier := crdmanager.RenovateJobIdentifier{
		Name:      body.RenovateJob,
		Namespace: body.Namespace,
	}

	err := s.manager.UpdateProjectStatusBatched(
		r.Context(),
		func(p crdmanager.RenovateProjectStatus) bool {
			return p.Status != api.JobStatusRunning && p.Status != api.JobStatusScheduled
		},
		jobIdentifier,
		&types.RenovateStatusUpdate{
			Status:           api.JobStatusScheduled,
			Priority:         2,
			ExecutionOptions: body.ExecutionOptions,
		},
	)
	if err != nil {
		s.logger.Error(err, "Failed to trigger all projects", "renovateJob", body.RenovateJob, "namespace", body.Namespace)
		internalServerError(w, err, "failed to trigger all projects")
		return
	}

	writeSuccess(w, SuccessResult{Message: "All projects triggered"})
	s.logger.V(2).Info("Successfully triggered all projects", "renovateJob", body.RenovateJob, "namespace", body.Namespace)
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

	// requirePermission returns the job to avoid a duplicate K8s API call
	job, ok := s.requirePermission(w, r, params.namespace, params.name, permDiscovery)
	if !ok {
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

	if _, err := s.discovery.CreateDiscoveryJob(ctx, *job, renovate.DiscoveryJobOptions{TriggerAllProjects: false}); err != nil {
		s.logger.Error(err, "Failed to start discovery for RenovateJob", "renovateJob", params.name, "namespace", params.namespace)
		internalServerError(w, err, "failed to create discovery job")
		return
	}

	writeSuccess(w, SuccessResult{Message: "discovery job started"})
	s.logger.V(2).Info("Successfully started discovery for RenovateJob", "renovateJob", params.name, "namespace", params.namespace)
}

func (s *Server) discoveryStatusForProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	namespace := r.URL.Query().Get("namespace")
	renovate := r.URL.Query().Get("renovate")

	// requireRead returns the job to avoid a duplicate K8s API call
	job, ok := s.requireRead(w, r, namespace, renovate)
	if !ok {
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
