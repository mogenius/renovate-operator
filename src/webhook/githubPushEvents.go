package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/types"
	"renovate-operator/metricStore"
)

// GitHubPushEvent is the payload of a GitHub "push" webhook event.
type GitHubPushEvent struct {
	// Ref is the full ref that was pushed, e.g. "refs/heads/main".
	Ref string `json:"ref"`
	// Deleted is true when the push deleted the ref.
	Deleted    bool                 `json:"deleted"`
	Repository GitHubPushRepository `json:"repository"`
}

// GitHubPushRepository is the repository object of a push payload.
type GitHubPushRepository struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
}

// githubPushEvent handles "push" events. A push to a project's default branch
// schedules a Renovate run for that project, for jobs that opted in with
// spec.webhook.triggerOnPush. Pushes to other branches (including Renovate's
// own branches) and branch deletions are ignored.
func (s *Server) githubPushEvent(ctx context.Context, w http.ResponseWriter, r *http.Request, body []byte) {
	const provider = "github"

	var payload GitHubPushEvent
	if err := json.Unmarshal(body, &payload); err != nil {
		metricStore.IncWebhookPayloadDecodeFailure(ctx, provider)
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.logger.Error(err, "failed to decode github push webhook payload. Not processing.")
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to decode payload"})
		return
	}

	if reason := ignoredPushReason(payload); reason != "" {
		metricStore.IncWebhookRequest(ctx, provider, "ignored")
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored", "reason": reason})
		return
	}

	namespace := r.URL.Query().Get("namespace")
	jobName := r.URL.Query().Get("job")
	project := payload.Repository.FullName

	checker := buildAuthCheckerFromRequest(r, body, s.manager)
	jobId, err := FindAndAuthenticateJob(ctx, pushTriggerJobs{s.manager}, namespace, jobName, project, checker)
	if errors.Is(err, ErrNoMatchingJob) {
		// Push subscriptions are often broader than the jobs that opted in
		// (e.g. a GitHub App delivering every repository's pushes), so an
		// unmatched push is acknowledged rather than marked failed.
		metricStore.IncWebhookRequest(ctx, provider, "ignored")
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored", "reason": "no renovate job with triggerOnPush owns the repository"})
		return
	}
	if err != nil {
		s.recordResolverAuthFailure(ctx, provider, err, signatureWasUsed(r))
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.logger.Info("webhook resolve failed", "project", project, "error", err)
		s.handleResolverError(w, err)
		return
	}

	s.logger.Info("received github push", "repository", project, "ref", payload.Ref, "priority", 1)
	err = s.manager.UpdateProjectStatus(ctx, project, jobId, &types.RenovateStatusUpdate{
		Status:   api.JobStatusScheduled,
		Priority: 1,
	})
	if s.handleUpdateProjectStatusError(w, err, project, jobId.Name, jobId.Namespace) {
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		return
	}

	metricStore.IncWebhookRequest(ctx, provider, "accepted")
	s.writeJSON(w, http.StatusAccepted, map[string]string{"message": "renovate job scheduled", "repository": project})
}

// ignoredPushReason returns why a push does not trigger a run, or "" when it
// does.
func ignoredPushReason(payload GitHubPushEvent) string {
	switch {
	case payload.Repository.FullName == "":
		return "no repository in payload"
	case payload.Deleted:
		return "push deleted the ref"
	case payload.Repository.DefaultBranch == "" ||
		payload.Ref != "refs/heads/"+payload.Repository.DefaultBranch:
		return "push is not to the default branch"
	}
	return ""
}

// pushTriggerJobs narrows job resolution to GitHub jobs that opted into push
// triggers, so no other job ever claims a push.
type pushTriggerJobs struct {
	jobLister
}

func (p pushTriggerJobs) ListEffectiveRenovateJobsFull(ctx context.Context) ([]api.RenovateJob, error) {
	jobs, err := p.jobLister.ListEffectiveRenovateJobsFull(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.RenovateJob, 0, len(jobs))
	for _, job := range jobs {
		if job.Spec.Webhook != nil && job.Spec.Webhook.TriggerOnPush && job.Spec.Provider.Is("github") {
			out = append(out, job)
		}
	}
	return out, nil
}
