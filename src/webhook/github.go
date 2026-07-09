package webhook

import (
	"encoding/json"
	"io"
	"net/http"
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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read request body"})
		return
	}

	var raw GitHubEvent
	if err := json.Unmarshal(body, &raw); err != nil {
		s.logger.Error(err, "failed to decode github webhook payload. Not processing.")
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to decode payload"})
		return
	}

	payload, valid := parseGitHubPayload(&raw)
	if !valid {
		s.logger.V(5).Info("failed to parse webhook payload")
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored"})
		return
	}

	s.handleWebhookTrigger(w, r, body, payload)
}

// parseGitHubPayload normalizes a raw GitHub event into a WebhookPayload and
// validates it. Returns false when the event should be ignored.
func parseGitHubPayload(raw *GitHubEvent) (WebhookPayload, bool) {
	if raw.Action != "edited" {
		return WebhookPayload{}, false
	}

	var eventType, current string
	if raw.PullRequest != nil {
		eventType = "pull_request"
		current = raw.PullRequest.Body
	} else if raw.Issue != nil {
		eventType = "issue"
		current = raw.Issue.Body
	} else {
		return WebhookPayload{}, false
	}

	return WebhookPayload{
		Provider:  "GitHub",
		Project:   raw.Repository.FullName,
		Action:    "edited",
		EventType: eventType,
		Body:      WebhookBody{Current: current},
	}, true
}
