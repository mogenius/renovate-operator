package webhook

import (
	"encoding/json"
	"io"
	"net/http"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/types"
)

type GitLabEvent struct {
	ObjectKind       string           `json:"object_kind"`
	EventType        string           `json:"event_type"`
	Project          Project          `json:"project"`
	ObjectAttributes ObjectAttributes `json:"object_attributes"`
	Changes          Changes          `json:"changes"`
}

type Project struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	PathWithNamespace string `json:"path_with_namespace"`
}

type ObjectAttributes struct {
	ID     int    `json:"id"`
	Action string `json:"action"`
}

type Changes struct {
	Description ChangeDescription `json:"description"`
}

type ChangeDescription struct {
	Previous string `json:"previous"`
	Current  string `json:"current"`
}

func (s *Server) gitLabWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read request body"})
		return
	}

	var payload GitLabEvent
	if err := json.Unmarshal(body, &payload); err != nil {
		s.logger.Error(err, "failed to decode gitlab webhook payload. Not processing.")
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to decode payload"})
		return
	}

	valid, reason := isValidGitLabEvent(&payload)
	if !valid {
		s.logger.Info("ignoring GitLab webhook event", "reason", reason)
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored", "reason": reason})
		return
	}

	namespace := r.URL.Query().Get("namespace")
	jobName := r.URL.Query().Get("job")
	project := payload.Project.PathWithNamespace

	checker := buildAuthCheckerFromRequest(r, body, s.manager)
	jobId, err := FindAndAuthenticateJob(r.Context(), s.manager, namespace, jobName, project, checker)
	if err != nil {
		s.logger.Info("webhook resolve failed", "project", project, "error", err)
		s.handleResolverError(w, err)
		return
	}

	s.logger.Info("received GitLab event", "repository", project, "action", payload.ObjectAttributes.Action, "priority", 1)
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

	s.writeJSON(w, http.StatusAccepted, map[string]string{"message": "renovate job scheduled", "project": project})
}

func isValidGitLabEvent(payload *GitLabEvent) (bool, string) {
	if payload.ObjectKind != "merge_request" && payload.ObjectKind != "issue" {
		return false, "object kind is not merge_request or issue"
	}

	if payload.ObjectKind == "merge_request" && payload.ObjectAttributes.Action == "merge" {
		return true, ""
	}

	if payload.ObjectAttributes.Action != "update" {
		return false, "event action is not update"
	}

	if payload.Changes.Description.Current == "" && payload.Changes.Description.Previous == "" {
		return false, "no description change detected"
	}

	if !verifyRenovateDescriptionChange(payload.Changes.Description.Current) {
		return false, "not a valid renovate checkbox change"
	}
	return true, ""
}
