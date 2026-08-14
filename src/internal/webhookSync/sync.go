/*
Package webhookSync keeps repository webhooks on the Git platform in sync with
the projects of a RenovateJob. It is provider-agnostic: all platform access
goes through the gitProviderClients.GitProviderClient interface.

Sync is stateless: the operator's hooks are identified by the platform
endpoint path and the namespace/job parameters of their delivery URL, so no
bookkeeping is persisted between runs. The platform is the source of truth.

The host is deliberately excluded from that identity: it is configuration, not
identity. A hook whose identity matches but whose delivery URL differs is one
of ours that was written under a different base URL, and is reconciled in place
rather than duplicated.
*/
package webhookSync

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"renovate-operator/gitProviderClients"

	"github.com/go-logr/logr"
)

// maxConcurrentRequests caps parallel platform API calls during a sync cycle.
const maxConcurrentRequests = 10

// Options configures a webhook sync run. The events a hook subscribes to are
// not configurable: each provider applies its own fixed, minimal set.
type Options struct {
	// WebhookURL is the full delivery URL (including namespace/job query
	// parameters). It identifies hooks managed by the operator.
	WebhookURL string
	// AuthToken is optional; providers attach it in their platform-specific way.
	AuthToken string
}

// Sync ensures the operator's webhook exists on every project in desired and
// removes the operator's webhook from the removed projects. Failures are
// logged and skipped (fail open): a failed ensure is corrected on the next
// cycle, a removal that fails is not retried: the orphaned hook is logged and
// must be cleaned up manually (its deliveries are rejected by the operator).
func Sync(ctx context.Context, logger logr.Logger, client gitProviderClients.GitProviderClient, opts Options, desired []string, removed []string) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, maxConcurrentRequests)

	run := func(_ string, fn func()) {
		wg.Add(1)
		semaphore <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-semaphore }()
			fn()
		}()
	}

	for _, project := range desired {
		run(project, func() {
			if err := ensureWebhook(ctx, logger, client, opts, project); err != nil {
				logger.Error(err, "failed to ensure webhook", "repo", project)
			}
		})
	}

	for _, project := range removed {
		run(project, func() {
			removeWebhook(ctx, logger, client, opts.WebhookURL, project)
		})
	}

	wg.Wait()
}

// hookIdentity is the host-independent part of a delivery URL that marks a hook
// as belonging to one RenovateJob: the platform endpoint path plus the
// namespace/job parameters the webhook server routes on.
type hookIdentity struct {
	path      string
	job       string
	namespace string
}

// identityOf extracts the identity from a delivery URL. Values are compared
// parsed rather than as raw strings, because query parameter ordering is only
// guaranteed for URLs the operator built itself.
func identityOf(rawURL string) (hookIdentity, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return hookIdentity{}, err
	}
	query := parsed.Query()
	return hookIdentity{
		path:      strings.TrimSuffix(parsed.Path, "/"),
		job:       query.Get("job"),
		namespace: query.Get("namespace"),
	}, nil
}

// ensureWebhook makes sure the operator's hook exists on the project with the
// desired configuration. An existing hook of ours whose delivery URL, event
// subscription or active state drifted is updated in place. The auth token is
// write-only on every platform and cannot be drift-checked.
func ensureWebhook(ctx context.Context, logger logr.Logger, client gitProviderClients.GitProviderClient, opts Options, project string) error {
	desired := gitProviderClients.CreateWebhookOptions{
		URL:       opts.WebhookURL,
		AuthToken: opts.AuthToken,
		Active:    true,
	}

	wanted, err := identityOf(opts.WebhookURL)
	if err != nil {
		return fmt.Errorf("failed to parse webhook URL: %w", err)
	}

	hooks, err := client.ListRepoWebhooks(ctx, project)
	if err != nil {
		return err
	}
	for _, hook := range hooks {
		if found, err := identityOf(hook.URL); err != nil || found != wanted {
			continue
		}
		if hook.URL == opts.WebhookURL && hook.Active && hook.EventsUpToDate {
			return nil
		}
		if _, err := client.UpdateRepoWebhook(ctx, project, hook.ID, desired); err != nil {
			return err
		}
		if hook.URL != opts.WebhookURL {
			logger.Info("updated webhook whose delivery URL changed", "repo", project, "hookID", hook.ID, "from", hook.URL, "to", opts.WebhookURL)
		} else {
			logger.Info("updated webhook whose configuration drifted", "repo", project, "hookID", hook.ID)
		}
		return nil
	}

	hook, err := client.CreateRepoWebhook(ctx, project, desired)
	if err != nil {
		return err
	}
	logger.Info("created webhook on repo", "repo", project, "hookID", hook.ID)
	return nil
}

// removeWebhook deletes the operator's hook from the project, if present. It
// matches the same identity as ensureWebhook, so a hook left behind under an
// earlier base URL is cleaned up rather than orphaned.
func removeWebhook(ctx context.Context, logger logr.Logger, client gitProviderClients.GitProviderClient, webhookURL, project string) {
	wanted, err := identityOf(webhookURL)
	if err != nil {
		logger.Error(err, "failed to parse webhook URL, skipping removal", "repo", project)
		return
	}

	hooks, err := client.ListRepoWebhooks(ctx, project)
	if err != nil {
		// The repo may already be gone together with its hooks; anything else
		// leaves the hook orphaned (harmless: its deliveries are rejected).
		logger.Error(err, "failed to list webhooks for removal, the operator's hook may remain orphaned", "repo", project)
		return
	}

	for _, hook := range hooks {
		if found, err := identityOf(hook.URL); err != nil || found != wanted {
			continue
		}
		if err := client.DeleteRepoWebhook(ctx, project, hook.ID); err != nil {
			logger.Error(err, "failed to remove webhook, it remains orphaned", "repo", project, "hookID", hook.ID)
			return
		}
		logger.Info("removed webhook from repo", "repo", project, "hookID", hook.ID)
		return
	}
}
