package webhook

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
)

// ErrNoMatchingJob is returned when no RenovateJob matches the namespace/job/project filters.
var ErrNoMatchingJob = errors.New("no matching renovate job found")

// ErrAuthenticationFailed is returned when candidates were found but none passed authentication.
var ErrAuthenticationFailed = errors.New("authentication failed")

// AuthChecker validates a credential against a specific RenovateJob.
type AuthChecker func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier) (bool, error)

type jobLister interface {
	// Effective specs: a job's webhook block may be inherited from a template.
	ListEffectiveRenovateJobsFull(ctx context.Context) ([]api.RenovateJob, error)
	GetProjectsForRenovateJob(ctx context.Context, job crdmanager.RenovateJobIdentifier) ([]crdmanager.RenovateProjectStatus, error)
}

type credentialValidator interface {
	IsWebhookTokenValid(ctx context.Context, job crdmanager.RenovateJobIdentifier, token string) (bool, error)
	IsWebhookSignatureValid(ctx context.Context, job crdmanager.RenovateJobIdentifier, signature string, body []byte) (bool, error)
	IsWebhookStandardSignatureValid(ctx context.Context, job crdmanager.RenovateJobIdentifier, msgID, timestamp, signature string, body []byte) (bool, error)
}

// FindAndAuthenticateJob discovers which RenovateJob owns project and authenticates the request.
//
// namespace and jobName are optional filters; pass empty string to skip each filter.
// The function lists all RenovateJobs, applies filters, then tries to authenticate against each
// candidate: jobs with authentication disabled pass automatically; jobs with authentication enabled
// require checker to return true. Returns the first authenticated candidate.
//
// Returns ErrNoMatchingJob when no job matches the filters, ErrAuthenticationFailed when
// candidates exist but none pass authentication. When a checker failed with an error rather
// than a mismatch (e.g. the webhook secret could not be read), the returned error still
// matches ErrAuthenticationFailed but also wraps that cause, so it shows up in the logs.
func FindAndAuthenticateJob(
	ctx context.Context,
	manager jobLister,
	namespace string,
	jobName string,
	project string,
	checker AuthChecker,
) (crdmanager.RenovateJobIdentifier, error) {
	jobs, err := manager.ListEffectiveRenovateJobsFull(ctx)
	if err != nil {
		return crdmanager.RenovateJobIdentifier{}, err
	}

	candidates := filterCandidates(ctx, manager, jobs, namespace, jobName, project)
	if len(candidates) == 0 {
		return crdmanager.RenovateJobIdentifier{}, ErrNoMatchingJob
	}

	var checkErrs []error
	for _, job := range candidates {
		id := crdmanager.RenovateJobIdentifier{Name: job.Name, Namespace: job.Namespace}

		if job.Spec.Webhook.Authentication == nil || !job.Spec.Webhook.Authentication.Enabled {
			return id, nil
		}
		if checker == nil {
			continue
		}
		ok, err := checker(ctx, id)
		if err != nil {
			checkErrs = append(checkErrs, fmt.Errorf("%s/%s: %w", id.Namespace, id.Name, err))
			continue
		}
		if !ok {
			continue
		}
		return id, nil
	}

	if len(checkErrs) > 0 {
		return crdmanager.RenovateJobIdentifier{}, fmt.Errorf("%w: %w", ErrAuthenticationFailed, errors.Join(checkErrs...))
	}
	return crdmanager.RenovateJobIdentifier{}, ErrAuthenticationFailed
}

// ErrAuthenticationRequired is returned by AuthenticateJobForAllProjects for a job whose
// webhook has authentication disabled. It matches ErrAuthenticationFailed, so callers answer
// it like any other failed credential check.
var ErrAuthenticationRequired = fmt.Errorf("%w: scheduling every project requires webhook authentication on the job", ErrAuthenticationFailed)

// AuthenticateJobForAllProjects resolves the RenovateJob namespace/jobName for a request that
// schedules every project of that job, and authenticates the request against it.
//
// Unlike FindAndAuthenticateJob it does not search: namespace and jobName name exactly one job
// and no project takes part in the lookup. It is also stricter, since one request reaches every
// repository of the job: the job's webhook must have authentication enabled and checker must
// pass. A job without authentication is refused with ErrAuthenticationRequired instead of being
// let through.
//
// Returns ErrNoMatchingJob when the job does not exist or its webhook is disabled, and
// ErrAuthenticationFailed (wrapping the checker's error, if any) when the credential is missing
// or does not match.
func AuthenticateJobForAllProjects(
	ctx context.Context,
	manager jobLister,
	namespace string,
	jobName string,
	checker AuthChecker,
) (crdmanager.RenovateJobIdentifier, error) {
	jobs, err := manager.ListEffectiveRenovateJobsFull(ctx)
	if err != nil {
		return crdmanager.RenovateJobIdentifier{}, err
	}

	for _, job := range jobs {
		if job.Namespace != namespace || job.Name != jobName {
			continue
		}
		if job.Spec.Webhook == nil || !job.Spec.Webhook.Enabled {
			return crdmanager.RenovateJobIdentifier{}, ErrNoMatchingJob
		}
		if job.Spec.Webhook.Authentication == nil || !job.Spec.Webhook.Authentication.Enabled {
			return crdmanager.RenovateJobIdentifier{}, ErrAuthenticationRequired
		}
		if checker == nil {
			return crdmanager.RenovateJobIdentifier{}, ErrAuthenticationFailed
		}

		id := crdmanager.RenovateJobIdentifier{Name: job.Name, Namespace: job.Namespace}
		ok, err := checker(ctx, id)
		if err != nil {
			return crdmanager.RenovateJobIdentifier{}, fmt.Errorf("%w: %s/%s: %w", ErrAuthenticationFailed, id.Namespace, id.Name, err)
		}
		if !ok {
			return crdmanager.RenovateJobIdentifier{}, ErrAuthenticationFailed
		}
		return id, nil
	}

	return crdmanager.RenovateJobIdentifier{}, ErrNoMatchingJob
}

func filterCandidates(ctx context.Context, manager jobLister, jobs []api.RenovateJob, namespace, jobName, project string) []api.RenovateJob {
	out := make([]api.RenovateJob, 0, len(jobs))
	for _, job := range jobs {
		if namespace != "" && job.Namespace != namespace {
			continue
		}
		if jobName != "" && job.Name != jobName {
			continue
		}
		if job.Spec.Webhook == nil || !job.Spec.Webhook.Enabled {
			continue
		}
		jobId := crdmanager.RenovateJobIdentifier{Name: job.Name, Namespace: job.Namespace}
		projects, err := manager.GetProjectsForRenovateJob(ctx, jobId)
		if err != nil || !hasProjectInList(projects, project) {
			continue
		}
		out = append(out, job)
	}
	return out
}

func hasProjectInList(projects []crdmanager.RenovateProjectStatus, project string) bool {
	for _, p := range projects {
		if p.Name == project {
			return true
		}
	}
	return false
}

// signatureWasUsed reports whether an HMAC signature header was the credential that
// buildAuthCheckerFromRequest would actually use for this request. It mirrors that
// function's precedence: a bearer/X-Gitlab-Token token takes priority over a
// signature header, so a signature is only "used" when no token header is present.
// This lets callers attribute an authentication failure to a signature mismatch.
func signatureWasUsed(r *http.Request) bool {
	if r.Header.Get("Authorization") != "" || r.Header.Get("X-Gitlab-Token") != "" {
		return false
	}
	return r.Header.Get("X-Hub-Signature-256") != "" || r.Header.Get("X-Forgejo-Signature") != ""
}

// buildAuthCheckerFromRequest extracts auth credentials from request headers and returns
// an AuthChecker that validates them against a RenovateJob. Returns nil if no credential is present.
func buildAuthCheckerFromRequest(r *http.Request, body []byte, manager credentialValidator) AuthChecker {
	// Standard Webhooks signature (https://www.standardwebhooks.com/) — a vendor-neutral scheme used by
	// GitLab signing tokens among others; prefer the cryptographic signature when present.
	if sig := r.Header.Get("Webhook-Signature"); sig != "" {
		msgID := r.Header.Get("Webhook-Id")
		timestamp := r.Header.Get("Webhook-Timestamp")
		b := body
		return func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier) (bool, error) {
			return manager.IsWebhookStandardSignatureValid(ctx, jobId, msgID, timestamp, sig, b)
		}
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		authHeader = r.Header.Get("X-Gitlab-Token")
	}
	if authHeader != "" {
		token := authHeader
		if strings.HasPrefix(authHeader, "Bearer ") {
			parts := strings.SplitN(authHeader, " ", 2)
			token = strings.TrimSpace(parts[1])
		}
		t := token
		return func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier) (bool, error) {
			return manager.IsWebhookTokenValid(ctx, jobId, t)
		}
	}

	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		// Bitbucket signs with sha256 but sends the WebSub-style header name
		signature = r.Header.Get("X-Hub-Signature")
	}
	if signature == "" {
		signature = r.Header.Get("X-Forgejo-Signature")
	}
	if signature == "" {
		signature = r.Header.Get("X-Gitea-Signature")
	}
	if signature != "" {
		sig := signature
		b := body
		return func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier) (bool, error) {
			return manager.IsWebhookSignatureValid(ctx, jobId, sig, b)
		}
	}

	return nil
}
