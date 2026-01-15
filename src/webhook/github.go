package webhook

import (
	"encoding/json"
	"net/http"
	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
)

type GitHubEvent struct {
	Action      string             `json:"action"`
	PullRequest *GitHubPullRequest `json:"pull_request,omitempty"`
	Issue       *GitHubIssue       `json:"issue,omitempty"`
	Repository  GitHubRepository   `json:"repository"`
}

type GitHubPullRequest struct {
	ID     int    `json:"id"`
	Number int    `json:"number"`
	Body   string `json:"body"`
}

type GitHubIssue struct {
	ID     int    `json:"id"`
	Number int    `json:"number"`
	Body   string `json:"body"`
}

type GitHubRepository struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
}

func (s *Server) githubWebhook(w http.ResponseWriter, r *http.Request) {
	var payload GitHubEvent
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		s.logger.Error(err, "failed to decode github webhook payload. Not processing.")
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to decode payload"})
		return
	}

	valid, reason := isValidGitHubEvent(&payload)
	if !valid {
		s.logger.Info("ignoring github webhook event", "reason", reason)
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
	s.logger.Info("received github event", "repository", payload.Repository.FullName, "action", payload.Action)
	err = s.manager.UpdateProjectStatus(
		r.Context(),
		payload.Repository.FullName,
		crdmanager.RenovateJobIdentifier{
			Name:      job,
			Namespace: namespace,
		},
		api.JobStatusScheduled,
	)
	if err != nil {
		s.logger.Error(err, "Failed to process GitHub webhook for repo", "repo", payload.Repository.FullName)
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to process webhook"})
		return
	}

	s.writeJSON(w, http.StatusAccepted, map[string]string{"message": "renovate job scheduled", "repository": payload.Repository.FullName})
}

func isValidGitHubEvent(payload *GitHubEvent) (bool, string) {
	// Only process pull request or issue edited events
	if payload.Action != "edited" {
		return false, "event action is not edited"
	}

	// Check if it's a pull request or issue event
	if payload.PullRequest == nil && payload.Issue == nil {
		return false, "event is neither pull request nor issue"
	}

	// Check if body was changed
	if (payload.PullRequest == nil || payload.PullRequest.Body == "") &&
		(payload.Issue == nil || payload.Issue.Body == "") {
		return false, "no body change detected"
	}

	// Get the current body (either from PR or Issue)
	var currentBody string
	if payload.PullRequest != nil {
		currentBody = payload.PullRequest.Body
	} else if payload.Issue != nil {
		currentBody = payload.Issue.Body
	}

	// Verify that this is a Renovate event and a checkbox was checked
	if !verifyRenovateDescriptionChange(currentBody) {
		return false, "not a valid renovate checkbox change"
	}
	return true, ""
}
