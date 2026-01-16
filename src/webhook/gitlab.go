package webhook

import (
	"encoding/json"
	"net/http"
	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
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
	var payload GitLabEvent
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
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
	job := r.URL.Query().Get("job")
	if namespace == "" || job == "" {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing namespace or job query parameter"})
		return
	}

	// Process the webhook payload
	s.logger.Info("received GitLab event", "repository", payload.Project.PathWithNamespace, "action", payload.ObjectAttributes.Action)
	err = s.manager.UpdateProjectStatus(
		r.Context(),
		payload.Project.PathWithNamespace,
		crdmanager.RenovateJobIdentifier{
			Name:      job,
			Namespace: namespace,
		},
		api.JobStatusScheduled,
	)
	if err != nil {
		s.logger.Error(err, "Failed to process GitLab webhook for project", "project", payload.Project.PathWithNamespace)
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to process webhook"})
		return
	}

	s.writeJSON(w, http.StatusAccepted, map[string]string{"message": "renovate job scheduled", "project": payload.Project.PathWithNamespace})

}

func isValidGitLabEvent(payload *GitLabEvent) (bool, string) {
	if payload.ObjectKind != "merge_request" && payload.ObjectKind != "issue" {
		return false, "object kind is not merge_request or issue"
	}

	if payload.ObjectAttributes.Action != "update" {
		return false, "event action is not update"
	}

	if payload.Changes.Description.Current == "" && payload.Changes.Description.Previous == "" {
		return false, "no description change detected"
	}

	// Verify that this is a Renovate event and a checkbox was checked
	if !verifyRenovateDescriptionChange(payload.Changes.Description.Current) {
		return false, "not a valid renovate checkbox change"
	}
	return true, ""
}
