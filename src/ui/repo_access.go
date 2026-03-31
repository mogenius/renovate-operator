package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	crdmanager "renovate-operator/internal/crdManager"

	"github.com/go-logr/logr"
)

const defaultRepoCacheTTL = 5 * time.Minute

// RepoPermission stores the user's permission level for a repository.
type RepoPermission struct {
	CanWrite bool
}

// RepoCache provides a caching layer for user repository access lookups.
// The in-memory implementation can be swapped for Valkey/Redis.
type RepoCache interface {
	Get(ctx context.Context, key string) (map[string]RepoPermission, bool)
	Set(ctx context.Context, key string, repos map[string]RepoPermission, ttl time.Duration)
}

// --- In-memory implementation ---

type memoryCacheEntry struct {
	repos   map[string]RepoPermission
	expires time.Time
}

type MemoryRepoCache struct {
	mu      sync.RWMutex
	entries map[string]memoryCacheEntry
}

func NewMemoryRepoCache() *MemoryRepoCache {
	c := &MemoryRepoCache{entries: make(map[string]memoryCacheEntry)}
	go c.cleanup()
	return c
}

func (c *MemoryRepoCache) Get(_ context.Context, key string) (map[string]RepoPermission, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	// Return a defensive copy to prevent callers from mutating cached state.
	copied := make(map[string]RepoPermission, len(entry.repos))
	for k, v := range entry.repos {
		copied[k] = v
	}
	return copied, true
}

func (c *MemoryRepoCache) Set(_ context.Context, key string, repos map[string]RepoPermission, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Store a defensive copy so the caller can't mutate cached state.
	stored := make(map[string]RepoPermission, len(repos))
	for k, v := range repos {
		stored[k] = v
	}
	c.entries[key] = memoryCacheEntry{repos: stored, expires: time.Now().Add(ttl)}
}

func (c *MemoryRepoCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for k, v := range c.entries {
			if now.After(v.expires) {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}

// --- Cache key helper ---

// normalizeEndpoint strips path, query, and default ports so that semantically
// equivalent endpoint URLs (e.g. with/without /api/v1, trailing slash) share a cache entry.
func normalizeEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return strings.TrimRight(endpoint, "/")
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	u.Host = strings.ToLower(u.Host)
	return strings.TrimRight(u.String(), "/")
}

func repoCacheKey(token, endpoint string) string {
	normalized := normalizeEndpoint(endpoint)
	h := sha256.Sum256([]byte(token + "\x00" + normalized))
	return hex.EncodeToString(h[:])
}

// --- Platform repo fetchers ---

func fetchUserRepos(ctx context.Context, platform, endpoint, token string, logger logr.Logger) (map[string]RepoPermission, error) {
	switch strings.ToLower(platform) {
	case "forgejo", "gitea":
		return fetchForgejoUserRepos(ctx, endpoint, token, logger)
	case "github":
		return fetchGitHubUserRepos(ctx, endpoint, token, logger)
	default:
		return nil, fmt.Errorf("repo access filtering not supported for platform %q", platform)
	}
}

func fetchForgejoUserRepos(ctx context.Context, endpoint, token string, logger logr.Logger) (map[string]RepoPermission, error) {
	endpoint = strings.TrimSuffix(endpoint, "/")
	endpoint = strings.TrimSuffix(endpoint, "/api/v1")

	repos := make(map[string]RepoPermission)
	client := &http.Client{Timeout: 30 * time.Second}
	page := 1
	limit := 50

	for {
		params := url.Values{}
		params.Set("limit", strconv.Itoa(limit))
		params.Set("page", strconv.Itoa(page))

		reqURL := fmt.Sprintf("%s/api/v1/repos/search?%s", endpoint, params.Encode())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "token "+token)
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("forgejo API request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return nil, fmt.Errorf("forgejo API returned %d: %s", resp.StatusCode, string(body))
		}

		var result struct {
			Data []struct {
				FullName    string `json:"full_name"`
				Permissions *struct {
					Admin bool `json:"admin"`
					Push  bool `json:"push"`
					Pull  bool `json:"pull"`
				} `json:"permissions,omitempty"`
			} `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to decode forgejo API response: %w", err)
		}

		if len(result.Data) == 0 {
			break
		}
		for _, r := range result.Data {
			canWrite := r.Permissions != nil && (r.Permissions.Push || r.Permissions.Admin)
			repos[r.FullName] = RepoPermission{CanWrite: canWrite}
		}
		logger.V(2).Info("fetched user repos from forgejo", "page", page, "count", len(result.Data))
		if len(result.Data) < limit {
			break
		}
		page++
	}

	logger.V(1).Info("fetched all user repos from forgejo", "total", len(repos))
	return repos, nil
}

func fetchGitHubUserRepos(ctx context.Context, endpoint, token string, logger logr.Logger) (map[string]RepoPermission, error) {
	if endpoint == "" {
		endpoint = "https://api.github.com"
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	repos := make(map[string]RepoPermission)
	client := &http.Client{Timeout: 30 * time.Second}
	page := 1

	for {
		reqURL := fmt.Sprintf("%s/user/repos?per_page=100&page=%d", endpoint, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("github API request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return nil, fmt.Errorf("github API returned %d: %s", resp.StatusCode, string(body))
		}

		var result []struct {
			FullName    string `json:"full_name"`
			Permissions *struct {
				Admin    bool `json:"admin"`
				Push     bool `json:"push"`
				Pull     bool `json:"pull"`
				Maintain bool `json:"maintain"`
			} `json:"permissions,omitempty"`
		}
		err = json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to decode github API response: %w", err)
		}

		if len(result) == 0 {
			break
		}
		for _, r := range result {
			canWrite := r.Permissions != nil && (r.Permissions.Push || r.Permissions.Admin || r.Permissions.Maintain)
			repos[r.FullName] = RepoPermission{CanWrite: canWrite}
		}
		logger.V(2).Info("fetched user repos from github", "page", page, "count", len(result))
		if len(result) < 100 {
			break
		}
		page++
	}

	logger.V(1).Info("fetched all user repos from github", "total", len(repos))
	return repos, nil
}

// --- Project filtering ---

// getUserRepos resolves the user's repo permissions from cache or the platform API.
// Returns nil if permissions cannot be determined (no token, unsupported platform, API error).
func getUserRepos(
	ctx context.Context,
	session *sessionData,
	platform, endpoint string,
	cache RepoCache,
	logger logr.Logger,
) map[string]RepoPermission {
	if session == nil || session.AccessToken == "" || cache == nil {
		return nil
	}
	if platform == "" || endpoint == "" {
		return nil
	}

	key := repoCacheKey(session.AccessToken, endpoint)

	userRepos, ok := cache.Get(ctx, key)
	if !ok {
		var err error
		userRepos, err = fetchUserRepos(ctx, platform, endpoint, session.AccessToken, logger)
		if err != nil {
			logger.Info("failed to fetch user repos for access filtering, showing all projects",
				"platform", platform,
				"endpoint", endpoint,
				"user", session.Email,
				"error", err.Error())
			return nil
		}
		cache.Set(ctx, key, userRepos, defaultRepoCacheTTL)
	}

	return userRepos
}

// filterProjectsByAccess filters a job's projects to only those the user can access
// on the git platform. Returns the original list unchanged if filtering is not possible
// (no token, unsupported platform, API error).
func filterProjectsByAccess(
	ctx context.Context,
	session *sessionData,
	platform, endpoint string,
	projects []crdmanager.RenovateProjectStatus,
	cache RepoCache,
	logger logr.Logger,
) []crdmanager.RenovateProjectStatus {
	userRepos := getUserRepos(ctx, session, platform, endpoint, cache, logger)
	if userRepos == nil {
		return projects
	}

	filtered := make([]crdmanager.RenovateProjectStatus, 0, len(projects))
	for _, p := range projects {
		if _, ok := userRepos[p.Name]; ok {
			filtered = append(filtered, p)
		}
	}

	logger.V(1).Info("filtered projects by user access",
		"user", session.Email,
		"platform", platform,
		"total_projects", len(projects),
		"visible_projects", len(filtered))

	return filtered
}

// canUserWriteRepo checks if the user has write access to a specific repo.
// Returns true (fail-open) when permissions cannot be determined — this covers
// unsupported platforms, missing tokens, and transient API errors. Fail-open is
// intentional: this is additive security on top of existing group-based auth,
// and failing closed would block all trigger actions on transient API failures.
func canUserWriteRepo(
	ctx context.Context,
	session *sessionData,
	platform, endpoint, repoName string,
	cache RepoCache,
	logger logr.Logger,
) bool {
	userRepos := getUserRepos(ctx, session, platform, endpoint, cache, logger)
	if userRepos == nil {
		return true // fail-open when we can't determine permissions
	}
	perm, ok := userRepos[repoName]
	if !ok {
		return false // not in user's repo list at all
	}
	return perm.CanWrite
}
