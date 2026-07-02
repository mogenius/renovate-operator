package webhook

import (
	"encoding/json"
	"io"
	"net/http"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/types"
	"renovate-operator/metricStore"
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
	ctx := r.Context()
	const provider = "github"

	body, err := io.ReadAll(r.Body)
	if err != nil {
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read request body"})
		return
	}

	var payload GitHubEvent
	if err := json.Unmarshal(body, &payload); err != nil {
		metricStore.IncWebhookPayloadDecodeFailure(ctx, provider)
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.logger.Error(err, "failed to decode github webhook payload. Not processing.")
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to decode payload"})
		return
	}

	valid, reason := isValidGitHubEvent(&payload)
	if !valid {
		metricStore.IncWebhookRequest(ctx, provider, "ignored")
		s.logger.Info("ignoring github webhook event", "reason", reason)
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored", "reason": reason})
		return
	}

	namespace := r.URL.Query().Get("namespace")
	jobName := r.URL.Query().Get("job")
	project := payload.Repository.FullName

	checker := buildAuthCheckerFromRequest(r, body, s.manager)
	jobId, err := FindAndAuthenticateJob(ctx, s.manager, namespace, jobName, project, checker)
	if err != nil {
		s.recordResolverAuthFailure(ctx, provider, err, signatureWasUsed(r))
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.logger.Info("webhook resolve failed", "project", project, "error", err)
		s.handleResolverError(w, err)
		return
	}

	s.logger.Info("received github event", "repository", project, "action", payload.Action, "priority", 1)
	err = s.manager.UpdateProjectStatus(
		ctx,
		project,
		jobId,
		&types.RenovateStatusUpdate{
			Status:   api.JobStatusScheduled,
			Priority: 1,
		},
	)
	if s.handleUpdateProjectStatusError(w, err, project, jobId.Name, jobId.Namespace) {
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		return
	}

	metricStore.IncWebhookRequest(ctx, provider, "accepted")
	s.writeJSON(w, http.StatusAccepted, map[string]string{"message": "renovate job scheduled", "repository": project})
}

func isValidGitHubEvent(payload *GitHubEvent) (bool, string) {
	if payload.Action != "edited" {
		return false, "event action is not edited"
	}

	if payload.PullRequest == nil && payload.Issue == nil {
		return false, "event is neither pull request nor issue"
	}

	if (payload.PullRequest == nil || payload.PullRequest.Body == "") &&
		(payload.Issue == nil || payload.Issue.Body == "") {
		return false, "no body change detected"
	}

	var currentBody string
	if payload.PullRequest != nil {
		currentBody = payload.PullRequest.Body
	} else if payload.Issue != nil {
		currentBody = payload.Issue.Body
	}

	if !verifyRenovateDescriptionChange(currentBody) {
		return false, "not a valid renovate checkbox change"
	}
	return true, ""
}
