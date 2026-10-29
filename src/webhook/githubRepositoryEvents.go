package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/renovate"
	"renovate-operator/metricStore"
)

// GitHubRepositoryEvent is the payload of a GitHub "repository" webhook event
// with action "created" or "deleted".
type GitHubRepositoryEvent struct {
	Action     string           `json:"action"`
	Repository GitHubRepository `json:"repository"`
}

// GitHubInstallationAccount is the account the GitHub App is installed on.
type GitHubInstallationAccount struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

// GitHubInstallation is the installation object embedded in GitHub App
// webhook payloads.
type GitHubInstallation struct {
	ID      int                       `json:"id"`
	Account GitHubInstallationAccount `json:"account"`
}

// GitHubInstallationEvent is the payload of a GitHub "installation" webhook
// event, sent with action "created" when the App is installed.
type GitHubInstallationEvent struct {
	Action       string             `json:"action"`
	Installation GitHubInstallation `json:"installation"`
	Repositories []GitHubRepository `json:"repositories"`
}

// githubRepositoryEvent handles "repository" events. A repository appearing or
// disappearing changes what a job's autodiscovery would return, so it starts a
// discovery run for every matching RenovateJob.
func (s *Server) githubRepositoryEvent(ctx context.Context, w http.ResponseWriter, r *http.Request, body []byte) {
	const provider = "github"

	var payload GitHubRepositoryEvent
	if err := json.Unmarshal(body, &payload); err != nil {
		metricStore.IncWebhookPayloadDecodeFailure(ctx, provider)
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.logger.Error(err, "failed to decode github repository webhook payload. Not processing.")
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to decode payload"})
		return
	}

	if payload.Action != "created" && payload.Action != "deleted" {
		metricStore.IncWebhookRequest(ctx, provider, "ignored")
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored", "reason": "repository action is not created or deleted"})
		return
	}
	if payload.Repository.FullName == "" {
		metricStore.IncWebhookRequest(ctx, provider, "ignored")
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored", "reason": "no repository in payload"})
		return
	}

	s.runDiscoveryForRepositories(ctx, w, r, body, []string{payload.Repository.FullName})
}

// githubInstallationEvent handles "installation" events. Every repository
// embedded in the payload is matched; payloads without a repository list fall
// back to matching the account login as "<login>/*", so filters like "acme/*"
// still trigger the discovery.
func (s *Server) githubInstallationEvent(ctx context.Context, w http.ResponseWriter, r *http.Request, body []byte) {
	const provider = "github"

	var payload GitHubInstallationEvent
	if err := json.Unmarshal(body, &payload); err != nil {
		metricStore.IncWebhookPayloadDecodeFailure(ctx, provider)
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.logger.Error(err, "failed to decode github installation webhook payload. Not processing.")
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to decode payload"})
		return
	}

	if payload.Action != "created" {
		metricStore.IncWebhookRequest(ctx, provider, "ignored")
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored", "reason": "installation action is not created"})
		return
	}

	repositories := installationRepositories(payload)
	if len(repositories) == 0 {
		metricStore.IncWebhookRequest(ctx, provider, "ignored")
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored", "reason": "no repository or account in payload"})
		return
	}

	s.runDiscoveryForRepositories(ctx, w, r, body, repositories)
}

// installationRepositories returns the repositories an installation event is
// matched against, or nil when the payload carries neither repositories nor
// an account.
func installationRepositories(payload GitHubInstallationEvent) []string {
	repositories := make([]string, 0, len(payload.Repositories))
	for _, repository := range payload.Repositories {
		if repository.FullName != "" {
			repositories = append(repositories, repository.FullName)
		}
	}
	if len(repositories) == 0 {
		if payload.Installation.Account.Login == "" {
			return nil
		}
		repositories = append(repositories, payload.Installation.Account.Login+"/*")
	}
	return repositories
}

// discoveryOutcome counts what startDiscoveries did with the matching jobs.
type discoveryOutcome struct {
	matched, authenticated, started, running, suspended, failed int
	startedJobs                                                 []string
}

// runDiscoveryForRepositories starts a discovery for every webhook-enabled
// GitHub job whose discovery filters match one of the repositories. Jobs with
// enabled webhook authentication only proceed when the request authenticates
// against them, mirroring FindAndAuthenticateJob.
func (s *Server) runDiscoveryForRepositories(ctx context.Context, w http.ResponseWriter, r *http.Request, body []byte, repositories []string) {
	const provider = "github"

	if s.discovery == nil {
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.logger.Error(errors.New("discovery agent not configured"), "cannot start discovery from webhook")
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "discovery is not configured"})
		return
	}

	outcome, err := s.startDiscoveries(ctx, r, body, repositories)
	if err != nil {
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.logger.Error(err, "failed to list RenovateJobs for repository event")
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to process webhook"})
		return
	}

	switch {
	case outcome.matched == 0:
		metricStore.IncWebhookRequest(ctx, provider, "ignored")
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored", "reason": "no matching renovate job"})
	case outcome.authenticated == 0:
		s.recordResolverAuthFailure(ctx, provider, ErrAuthenticationFailed, signatureWasUsed(r))
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	case outcome.started == 0:
		metricStore.IncWebhookRequest(ctx, provider, "ignored")
		s.writeJSON(w, http.StatusOK, map[string]any{
			"message":        "no discovery started",
			"alreadyRunning": outcome.running,
			"suspended":      outcome.suspended,
			"failed":         outcome.failed,
		})
	default:
		metricStore.IncWebhookRequest(ctx, provider, "accepted")
		s.logger.Info("started discovery from repository webhook", "repositories", repositories, "renovateJobs", outcome.startedJobs)
		s.writeJSON(w, http.StatusAccepted, map[string]any{"message": "discovery started", "renovateJobs": outcome.startedJobs})
	}
}

// startDiscoveries starts a discovery for every matching job the request
// authenticates against. It requires s.discovery.
func (s *Server) startDiscoveries(ctx context.Context, r *http.Request, body []byte, repositories []string) (discoveryOutcome, error) {
	namespace := r.URL.Query().Get("namespace")
	jobName := r.URL.Query().Get("job")
	checker := buildAuthCheckerFromRequest(r, body, s.manager)

	outcome := discoveryOutcome{startedJobs: make([]string, 0)}
	jobs, err := s.manager.ListEffectiveRenovateJobsFull(ctx)
	if err != nil {
		return outcome, err
	}

	for _, job := range jobs {
		if !jobMatchesRepositoryEvent(job, namespace, jobName, repositories) {
			continue
		}
		outcome.matched++
		id := crdmanager.RenovateJobIdentifier{Name: job.Name, Namespace: job.Namespace}

		if job.Spec.Webhook.Authentication != nil && job.Spec.Webhook.Authentication.Enabled {
			if checker == nil {
				continue
			}
			ok, err := checker(ctx, id)
			if err != nil || !ok {
				continue
			}
		}
		outcome.authenticated++

		status, err := s.discovery.GetDiscoveryJobStatus(ctx, &job)
		if err == nil && status == api.JobStatusRunning {
			outcome.running++
			continue
		}

		if _, err := s.discovery.CreateDiscoveryJob(ctx, job, renovate.DiscoveryJobOptions{TriggerAllProjects: false}); err != nil {
			if errors.Is(err, renovate.ErrRenovateJobSuspended) {
				outcome.suspended++
				continue
			}
			outcome.failed++
			s.logger.Error(err, "failed to start discovery for RenovateJob", "renovateJob", job.Name, "namespace", job.Namespace)
			continue
		}
		outcome.started++
		outcome.startedJobs = append(outcome.startedJobs, job.Fullname())
	}
	return outcome, nil
}

// jobMatchesRepositoryEvent reports whether a repository lifecycle event
// applies to this job: the job must be webhook-enabled, use the GitHub
// provider, and have discovery intent for at least one of the repositories.
func jobMatchesRepositoryEvent(job api.RenovateJob, namespace, jobName string, repositories []string) bool {
	if namespace != "" && job.Namespace != namespace {
		return false
	}
	if jobName != "" && job.Name != jobName {
		return false
	}
	if job.Spec.Webhook == nil || !job.Spec.Webhook.Enabled {
		return false
	}
	if job.Spec.Provider == nil || !strings.EqualFold(job.Spec.Provider.Name, "github") {
		return false
	}
	for _, repository := range repositories {
		if discoveryFiltersMatch(job.Spec.DiscoveryFilters, repository) {
			return true
		}
	}
	return false
}

// discoveryFiltersMatch mirrors the semantics of RENOVATE_AUTODISCOVER_FILTER:
// each filter is a regular expression matched against the repository's full
// name, and any match counts. Without filters, autodiscovery would consider
// every repository the credentials can see, so every repository matches.
func discoveryFiltersMatch(filters []string, repository string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		if ok, err := regexp.MatchString(filter, repository); err == nil && ok {
			return true
		}
	}
	return false
}
