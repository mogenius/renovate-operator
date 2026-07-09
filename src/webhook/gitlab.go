package webhook

import (
	"encoding/json"
	"io"
	"net/http"
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

	var raw GitLabEvent
	if err := json.Unmarshal(body, &raw); err != nil {
		s.logger.Error(err, "failed to decode gitlab webhook payload. Not processing.")
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to decode payload"})
		return
	}

	payload, valid := parseGitLabPayload(&raw)
	if !valid {
		s.logger.V(5).Info("failed to parse webhook payload")
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored"})
		return
	}

	s.handleWebhookTrigger(w, r, body, payload)
}

// parseGitLabPayload normalizes a raw GitLab event into a WebhookPayload and
// validates it. GitLab uses "update" as the action name and sends both the
// previous and current description in Changes. Returns false when the event
// should be ignored.
func parseGitLabPayload(raw *GitLabEvent) (WebhookPayload, bool) {
	var eventType string
	switch raw.ObjectKind {
	case "merge_request":
		eventType = "pull_request"
	case "issue":
		eventType = "issue"
	default:
		return WebhookPayload{}, false
	}

	if raw.ObjectAttributes.Action != "update" {
		return WebhookPayload{}, false
	}

	current := raw.Changes.Description.Current
	previous := raw.Changes.Description.Previous
	if current == "" && previous == "" {
		return WebhookPayload{}, false
	}

	return WebhookPayload{
		Provider:  "GitLab",
		Project:   raw.Project.PathWithNamespace,
		Action:    "edited",
		EventType: eventType,
		Body:      WebhookBody{Current: current, Previous: previous},
	}, true
}
