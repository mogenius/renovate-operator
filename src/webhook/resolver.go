package webhook

import (
	"context"
	"errors"
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
	ListRenovateJobsFull(ctx context.Context) ([]api.RenovateJob, error)
}

type credentialValidator interface {
	IsWebhookTokenValid(ctx context.Context, job crdmanager.RenovateJobIdentifier, token string) (bool, error)
	IsWebhookSignatureValid(ctx context.Context, job crdmanager.RenovateJobIdentifier, signature string, body []byte) (bool, error)
}

// FindAndAuthenticateJob discovers which RenovateJob owns project and authenticates the request.
//
// namespace and jobName are optional filters; pass empty string to skip each filter.
// The function lists all RenovateJobs, applies filters, then tries to authenticate against each
// candidate: jobs with authentication disabled pass automatically; jobs with authentication enabled
// require checker to return true. Returns the first authenticated candidate.
//
// Returns ErrNoMatchingJob when no job matches the filters, ErrAuthenticationFailed when
// candidates exist but none pass authentication.
func FindAndAuthenticateJob(
	ctx context.Context,
	manager jobLister,
	namespace string,
	jobName string,
	project string,
	checker AuthChecker,
) (crdmanager.RenovateJobIdentifier, error) {
	jobs, err := manager.ListRenovateJobsFull(ctx)
	if err != nil {
		return crdmanager.RenovateJobIdentifier{}, err
	}

	candidates := filterCandidates(jobs, namespace, jobName, project)
	if len(candidates) == 0 {
		return crdmanager.RenovateJobIdentifier{}, ErrNoMatchingJob
	}

	for _, job := range candidates {
		id := crdmanager.RenovateJobIdentifier{Name: job.Name, Namespace: job.Namespace}

		if job.Spec.Webhook.Authentication == nil || !job.Spec.Webhook.Authentication.Enabled {
			return id, nil
		}
		if checker == nil {
			continue
		}
		ok, err := checker(ctx, id)
		if err != nil || !ok {
			continue
		}
		return id, nil
	}

	return crdmanager.RenovateJobIdentifier{}, ErrAuthenticationFailed
}

func filterCandidates(jobs []api.RenovateJob, namespace, jobName, project string) []api.RenovateJob {
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
		if !hasProject(job.Status.Projects, project) {
			continue
		}
		out = append(out, job)
	}
	return out
}

func hasProject(projects []api.ProjectStatus, project string) bool {
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
		signature = r.Header.Get("X-Forgejo-Signature")
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
