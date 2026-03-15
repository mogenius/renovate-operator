package ui

import (
	"context"
	"encoding/json"
	"net/http"
	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/types"
	"renovate-operator/internal/utils"
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

func (s *Server) registerApiV1Routes(router *mux.Router) {
	apiV1 := router.PathPrefix("/api/v1").Subrouter()
	apiV1.HandleFunc("/version", s.getVersion).Methods("GET")
	apiV1.HandleFunc("/renovatejobs", s.getRenovateJobs).Methods("GET")
	apiV1.HandleFunc("/renovate", s.runRenovateForProject).Methods("POST")
	apiV1.HandleFunc("/renovate/all", s.runRenovateForAllProjects).Methods("POST")
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
	result := make([]RenovateJobInfo, 0)
	for i := range renovateJobs {
		renovateJob := &renovateJobs[i]

		discoveryStatus, err := s.discovery.GetDiscoveryJobStatus(r.Context(), renovateJob, "")
		if err != nil {
			if errors.IsNotFound(err) {
				discoveryStatus = api.JobStatusScheduled
			} else {
				// it might not be failed, but we dont want to block the whole response
				discoveryStatus = api.JobStatusFailed
			}
		}

		platform, platformEndpoint := utils.GetPlatformAndEndpoint(renovateJob.Spec.Provider)

		projects := make([]crdmanager.RenovateProjectStatus, 0, len(renovateJob.Status.Projects))
		for _, p := range renovateJob.Status.Projects {
			projects = append(projects, crdmanager.RenovateProjectStatus{
				Name:                 p.Name,
				Status:               p.Status,
				LastRun:              p.LastRun.Time,
				Priority:             p.Priority,
				RenovateResultStatus: p.RenovateResultStatus,
				Duration:             p.Duration,
			})
		}

		result = append(result, RenovateJobInfo{
			Name:             renovateJob.Name,
			Namespace:        renovateJob.Namespace,
			NextSchedule:     s.scheduler.GetNextRunOnSchedule(renovateJob.Spec.Schedule),
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

	logs, err := s.manager.GetLogsForProject(
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

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(logs))
}

func getRenovateJsonBody(r *http.Request) (*struct {
	name      string
	namespace string
	project   string
}, error) {
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
	ctx := r.Context()

	job, err := s.manager.GetRenovateJob(ctx, params.name, params.namespace)
	if err != nil || job == nil {
		internalServerError(w, err, "failed to get renovate job")
		return
	}
	// discovery mus only run once
	status, err := s.discovery.GetDiscoveryJobStatus(ctx, job, "")
	if err == nil && status == api.JobStatusRunning {
		// discovery job is already running
		writeSuccess(w, SuccessResult{Message: "discovery job is already running"})
		return
	}

	generation, err := s.discovery.CreateDiscoveryJob(ctx, *job)
	if err != nil {
		s.logger.Error(err, "Failed to start discovery for RenovateJob", "renovateJob", params.name, "namespace", params.namespace)
		internalServerError(w, err, "failed to create discovery job")
		return
	}
	go func() {
		ctxBackground := context.Background()
		projects, err := s.discovery.WaitForDiscoveryJob(ctxBackground, job, generation)
		if err != nil {
			s.logger.Error(err, "Discovery job failed for RenovateJob", "renovateJob", params.name, "namespace", params.namespace)
			return
		}
		// update all projects to scheduled
		jobIdentifier := crdmanager.RenovateJobIdentifier{
			Name:      params.name,
			Namespace: params.namespace,
		}
		err = s.manager.ReconcileProjects(ctxBackground, jobIdentifier, projects)
		if err != nil {
			s.logger.Error(err, "failed to reconcile projects")
			return
		}
	}()

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

	job, err := s.manager.GetRenovateJob(ctx, renovate, namespace)
	if err != nil || job == nil {
		internalServerError(w, err, "failed to get renovate job")
		return
	}
	status, err := s.discovery.GetDiscoveryJobStatus(ctx, job, "")
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
