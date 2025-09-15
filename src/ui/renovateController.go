package ui

import (
	"context"
	"encoding/json"
	"net/http"
	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"

	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/api/errors"
)

type RenovateJobInfo struct {
	Name      string                             `json:"name"`
	Namespace string                             `json:"namespace"`
	Projects  []crdmanager.RenovateProjectStatus `json:"projects"`
}

func (s *Server) registerApiV1Routes(router *mux.Router) {
	apiV1 := router.PathPrefix("/api/v1").Subrouter()
	apiV1.HandleFunc("/renovatejobs", s.getRenovateJobs).Methods("GET")
	apiV1.HandleFunc("/renovate", s.runRenovateForProject).Methods("POST")
	apiV1.HandleFunc("/logs", s.getRenovateJobLogs).Methods("GET")
	apiV1.HandleFunc("/discovery/start", s.runDiscoveryForProject).Methods("POST")
	apiV1.HandleFunc("/discovery/status", s.discoveryStatusForProject).Methods("GET")
}

func (s *Server) getRenovateJobs(w http.ResponseWriter, r *http.Request) {
	renovateJobs, err := s.manager.ListRenovateJobs(r.Context())
	if err != nil {
		internalServerError(w, err, "failed to load renovatejobs")
		return
	}
	result := make([]RenovateJobInfo, 0)
	for _, job := range renovateJobs {
		projects, err := s.manager.GetProjectsForRenovateJob(r.Context(), job)
		if err != nil {
			internalServerError(w, err, "failed to load projects")
			return
		}
		result = append(result, RenovateJobInfo{
			Name:      job.Name,
			Namespace: job.Namespace,
			Projects:  projects,
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
		internalServerError(w, err, "failed to get logs for project")
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
		api.JobStatusScheduled,
	)
	if err != nil {
		s.logger.Error(err, "Failed to run Renovate for project", "project", params.project, "renovateJob", params.name, "namespace", params.namespace)
		internalServerError(w, err, "failed to run Renovate for project")
		return
	}

	w.WriteHeader(http.StatusOK)
	s.logger.Info("Successfully triggered Renovate for project", "project", params.project, "renovateJob", params.name, "namespace", params.namespace)
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
	status, err := s.discovery.GetDiscoveryJobStatus(ctx, job)
	if err != nil {
		internalServerError(w, err, "failed to get discovery job status")
		return
	}
	if status == api.JobStatusRunning {
		badRequestError(w, err, "discovery job already running")
		return
	}

	_, err = s.discovery.CreateDiscoveryJob(ctx, *job)
	if err != nil {
		s.logger.Error(err, "Failed to start discovery for RenovateJob", "renovateJob", params.name, "namespace", params.namespace)
		internalServerError(w, err, "failed to create discovery job")
		return
	}
	go func() {
		ctxBackground := context.Background()
		projects, err := s.discovery.WaitForDiscoveryJob(ctxBackground, job)
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

	w.WriteHeader(http.StatusOK)
	s.logger.Info("Successfully started discovery for RenovateJob", "renovateJob", params.name, "namespace", params.namespace)
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
