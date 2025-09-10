package ui

import (
	"encoding/json"
	"net/http"
	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"

	"github.com/gorilla/mux"
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
}

func (s *Server) getRenovateJobs(w http.ResponseWriter, r *http.Request) {
	renovateJobs, err := s.manager.ListRenovateJobs(r.Context())
	if err != nil {
		http.Error(w, "failed to load renovatejobs", http.StatusBadRequest)
		return
	}
	result := make([]RenovateJobInfo, 0)
	for _, job := range renovateJobs {
		projects, err := s.manager.GetProjectsForRenovateJob(r.Context(), job)
		if err != nil {
			http.Error(w, "failed to load projects", http.StatusBadRequest)
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
		http.Error(w, "failed to get logs for project", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(logs))
}

func (s *Server) runRenovateForProject(w http.ResponseWriter, r *http.Request) {
	// Expect application/json or form values
	var renovateJob, namespace, project string
	if r.Header.Get("Content-Type") == "application/json" {
		var params struct {
			RenovateJob string `json:"renovateJob"`
			Namespace   string `json:"namespace"`
			Project     string `json:"project"`
		}
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}
		renovateJob = params.RenovateJob
		namespace = params.Namespace
		project = params.Project
	} else {
		// fallback to form values
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form body", http.StatusBadRequest)
			return
		}
		renovateJob = r.FormValue("renovateJob")
		namespace = r.FormValue("namespace")
		project = r.FormValue("project")
	}

	if renovateJob == "" || namespace == "" || project == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	err := s.manager.UpdateProjectStatus(
		r.Context(),
		project,
		crdmanager.RenovateJobIdentifier{
			Name:      renovateJob,
			Namespace: namespace,
		},
		api.JobStatusScheduled,
	)
	if err != nil {
		s.logger.Error(err, "Failed to run Renovate for project", "project", project, "renovateJob", renovateJob, "namespace", namespace)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	s.logger.Info("Successfully triggered Renovate for project", "project", project, "renovateJob", renovateJob, "namespace", namespace)
}
