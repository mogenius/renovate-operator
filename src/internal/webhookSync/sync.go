package webhookSync

import (
	"context"
	"fmt"
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
	for k, v := range s.managedRepos {
		out[k] = v
	}
	return out
}

// SetManagedRepos replaces the managed repos state (used to restore persisted state).
func (s *WebhookSyncer) SetManagedRepos(m map[string]int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.managedRepos = make(map[string]int64, len(m))
	for k, v := range m {
		s.managedRepos[k] = v
	}
}

// RunOnce executes one full sync cycle: ensures webhooks exist on topic repos and removes them from opted-out repos.
// It returns a consistent snapshot of the managed repos state as it was at the end of the sync cycle.
func (s *WebhookSyncer) RunOnce(ctx context.Context) (map[string]int64, error) {
	// Step 1: Search repos by topic (no lock needed — pure API call)
	repos, err := s.client.SearchReposByTopic(ctx, s.topic)
	if err != nil {
		return nil, fmt.Errorf("searching repos by topic %q: %w", s.topic, err)
	}

	// Step 2: Partition by admin permission
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
			s.logger.Error(err, "failed to ensure webhook", "repo", fullName)
			continue
		}
	}

	// Step 4: Remove webhooks from repos that are no longer eligible.
	// Snapshot the current managed repos so we can iterate without holding the lock during API calls.
	s.mu.Lock()
	pending := make(map[string]int64, len(s.managedRepos))
	for k, v := range s.managedRepos {
		pending[k] = v
	}
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

	// Return a consistent snapshot of the final state
	s.mu.Lock()
	snapshot := make(map[string]int64, len(s.managedRepos))
	for k, v := range s.managedRepos {
		snapshot[k] = v
	}
	s.mu.Unlock()

	return snapshot, nil
}

func (s *WebhookSyncer) ensureWebhook(ctx context.Context, owner, repo, fullName string) error {
	// API calls without holding the lock
	hooks, err := s.client.ListRepoWebhooks(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("listing webhooks: %w", err)
	}

	// Check if our webhook already exists
	for _, hook := range hooks {
		if hook.Config.URL == s.webhookURL {
			s.mu.Lock()
			s.managedRepos[fullName] = hook.ID
			s.mu.Unlock()
			return nil
		}
	}

	// Create the webhook
	cfg := gitProviderClients.WebhookConfig{
		URL:         s.webhookURL,
		ContentType: "json",
	}
	if s.authToken != "" {
		cfg.AuthorizationHeader = "Bearer " + s.authToken
	}
	opts := gitProviderClients.CreateWebhookOptions{
		Type:   "forgejo",
		Config: cfg,
		Events: s.events,
		Active: true,
	}

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
