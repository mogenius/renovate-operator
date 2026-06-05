package webhookSync

import (
	"context"
	"fmt"
	"maps"
	"renovate-operator/gitProviderClients"
	"strings"
	"sync"

	"github.com/go-logr/logr"
)

// WebhookSyncer manages webhook lifecycle on Forgejo repos tagged with a specific topic.
type WebhookSyncer struct {
	client       gitProviderClients.GitProviderClient
	webhookURL   string
	authToken    string
	events       []string
	topic        string
	logger       logr.Logger
	managedRepos map[string]int64 // repo fullName -> webhook ID
	mu           sync.Mutex
}

// NewWebhookSyncer creates a new WebhookSyncer.
func NewWebhookSyncer(client gitProviderClients.GitProviderClient, webhookURL, authToken, topic string, events []string, logger logr.Logger) *WebhookSyncer {
	if len(events) == 0 {
		events = []string{"issues", "pull_request"}
	}
	return &WebhookSyncer{
		client:       client,
		webhookURL:   webhookURL,
		authToken:    authToken,
		events:       events,
		topic:        topic,
		logger:       logger,
		managedRepos: make(map[string]int64),
	}
}

// ManagedRepos returns a copy of the current managed repos state.
func (s *WebhookSyncer) ManagedRepos() map[string]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int64, len(s.managedRepos))
	maps.Copy(out, s.managedRepos)
	return out
}

// SetManagedRepos replaces the managed repos state (used to restore persisted state).
func (s *WebhookSyncer) SetManagedRepos(m map[string]int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.managedRepos = make(map[string]int64, len(m))
	maps.Copy(s.managedRepos, m)
}

// RunOnce executes one full sync cycle: ensures webhooks exist on eligible repos and removes them from opted-out repos.
// When projects is non-empty, those repos are used directly instead of searching by topic.
// It returns a consistent snapshot of the managed repos state as it was at the end of the sync cycle.
func (s *WebhookSyncer) RunOnce(ctx context.Context, projects ...string) (map[string]int64, error) {
	var repos []gitProviderClients.Repository

	if len(projects) > 0 {
		// Use the provided project list — mark all as admin (the caller already discovered them)
		for _, fullName := range projects {
			repos = append(repos, gitProviderClients.Repository{
				FullName:    fullName,
				Permissions: &gitProviderClients.RepositoryPermissions{Admin: true},
			})
		}
	} else if s.topic != "" {
		// Fall back to topic-based search
		var err error
		repos, err = s.client.SearchReposByTopic(ctx, s.topic)
		if err != nil {
			return nil, fmt.Errorf("searching repos by topic %q: %w", s.topic, err)
		}
	} else {
		s.logger.Info("no projects provided and no topic configured, skipping webhook sync")
		return s.snapshotManagedRepos(), nil
	}

	// Partition by admin permission
	topicRepos := make(map[string]bool, len(repos))
	adminRepos := make(map[string]gitProviderClients.Repository)
	for _, repo := range repos {
		topicRepos[repo.FullName] = true
		if repo.Permissions != nil && repo.Permissions.Admin {
			adminRepos[repo.FullName] = repo
		} else {
			s.logger.Info("skipping repo: no admin permission to manage webhooks", "repo", repo.FullName)
		}
	}

	// Step 3: Ensure webhooks on admin repos
	for fullName := range adminRepos {
		parts := strings.SplitN(fullName, "/", 2)
		if len(parts) != 2 {
			s.logger.Error(fmt.Errorf("invalid repo full name: %s", fullName), "skipping repo")
			continue
		}
		owner, repoName := parts[0], parts[1]

		if err := s.ensureWebhook(ctx, owner, repoName, fullName); err != nil {
			if strings.Contains(err.Error(), "403") {
				s.logger.Info("skipping repo: no admin permission to manage webhooks", "repo", fullName)
			} else {
				s.logger.Error(err, "failed to ensure webhook", "repo", fullName)
			}
			continue
		}
	}

	// Step 4: Remove webhooks from repos that are no longer eligible.
	// Snapshot the current managed repos so we can iterate without holding the lock during API calls.
	s.mu.Lock()
	pending := make(map[string]int64, len(s.managedRepos))
	maps.Copy(pending, s.managedRepos)
	s.mu.Unlock()

	for fullName, hookID := range pending {
		if _, stillActive := adminRepos[fullName]; stillActive {
			continue
		}

		// Repo still has the topic but we lost admin access — we can't delete the webhook
		if topicRepos[fullName] {
			s.logger.Error(fmt.Errorf("lost admin permission on repo that still has topic %q", s.topic),
				"cannot remove webhook: admin permission required, please remove the webhook manually",
				"repo", fullName, "hookID", hookID)
			s.mu.Lock()
			delete(s.managedRepos, fullName)
			s.mu.Unlock()
			continue
		}

		parts := strings.SplitN(fullName, "/", 2)
		if len(parts) != 2 {
			continue
		}
		owner, repoName := parts[0], parts[1]

		if err := s.client.DeleteRepoWebhook(ctx, owner, repoName, hookID); err != nil {
			s.logger.Error(err, "failed to remove webhook", "repo", fullName)
			continue
		}

		s.logger.Info("removed webhook from repo (no longer matches topic)", "repo", fullName)
		s.mu.Lock()
		delete(s.managedRepos, fullName)
		s.mu.Unlock()
	}

	return s.snapshotManagedRepos(), nil
}

func (s *WebhookSyncer) snapshotManagedRepos() map[string]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := make(map[string]int64, len(s.managedRepos))
	maps.Copy(snapshot, s.managedRepos)
	return snapshot
}

func (s *WebhookSyncer) buildWebhookOptions() gitProviderClients.CreateWebhookOptions {
	cfg := gitProviderClients.WebhookConfig{
		URL:         s.webhookURL,
		ContentType: "json",
	}
	opts := gitProviderClients.CreateWebhookOptions{
		Type:   "forgejo",
		Config: cfg,
		Events: s.events,
		Active: true,
	}
	if s.authToken != "" {
		opts.Config.Secret = s.authToken
		opts.AuthorizationHeader = "Bearer " + s.authToken
	}
	return opts
}

func (s *WebhookSyncer) ensureWebhook(ctx context.Context, owner, repo, fullName string) error {
	// API calls without holding the lock
	hooks, err := s.client.ListRepoWebhooks(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("listing webhooks: %w", err)
	}

	opts := s.buildWebhookOptions()

	// Check if our webhook already exists
	for _, hook := range hooks {
		if hook.Config.URL == s.webhookURL {
			// Update the existing webhook to ensure config (secret, auth header, events) stays current
			updated, err := s.client.EditRepoWebhook(ctx, owner, repo, hook.ID, opts)
			if err != nil {
				return fmt.Errorf("updating webhook: %w", err)
			}
			s.mu.Lock()
			s.managedRepos[fullName] = updated.ID
			s.mu.Unlock()
			return nil
		}
	}

	// Create the webhook
	hook, err := s.client.CreateRepoWebhook(ctx, owner, repo, opts)
	if err != nil {
		return fmt.Errorf("creating webhook: %w", err)
	}

	s.mu.Lock()
	s.managedRepos[fullName] = hook.ID
	s.mu.Unlock()
	s.logger.Info("created webhook on repo", "repo", fullName, "hookID", hook.ID)
	return nil
}
