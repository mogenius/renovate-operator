package gitProviderClients

import (
	"context"
	"path"
	"strings"
	"sync"

	"github.com/go-logr/logr"
)

// FilterOptions selects which discovered repositories are excluded.
type FilterOptions struct {
	// SkipForks excludes forked repositories.
	SkipForks bool
	// SkipPendingDeletion excludes repositories marked for delayed deletion
	// (only GitLab reports this state).
	SkipPendingDeletion bool
	// IgnoreRepositories excludes repositories whose project path matches any
	// of the given case-insensitive minimatch-style globs.
	IgnoreRepositories []string
	// IgnoreTopics excludes repositories carrying any of the given topics
	// (case-insensitive).
	IgnoreTopics []string
}

// FilterStats reports how many repositories were dropped by each criterion so
// callers (which know the namespace/job labels) can record the
// repositories_filtered metric.
type FilterStats struct {
	ForksRemoved               int
	PendingRemoved             int
	IgnoredRepositoriesRemoved int
	IgnoredTopicsRemoved       int
}

// FilterProjects filters out repositories that should be skipped during
// discovery according to the enabled criteria:
//   - ignoreRepositories excludes projects matching a name/glob, without any
//     platform API call
//   - ignoreTopics excludes repositories carrying an ignored topic
//   - skipForks excludes forked repositories
//   - skipPendingDeletion excludes repositories marked for delayed deletion
//     (only GitLab reports this state)
//
// Repository metadata is fetched once per remaining project (a single platform
// API call), so enabling several metadata criteria does not multiply requests.
// On API errors for individual repos, the repo is kept (fail-open) to avoid
// accidentally excluding valid projects. providerClient may be nil when no
// metadata-based criterion is enabled; metadata criteria then fail open.
func FilterProjects(ctx context.Context, providerClient GitProviderClient, logger logr.Logger, projects []string, opts FilterOptions) ([]string, FilterStats, error) {
	var stats FilterStats
	if len(projects) == 0 {
		return projects, stats, nil
	}

	// Name/glob filtering needs no platform metadata, so it runs first.
	remaining := make([]string, 0, len(projects))
	for _, project := range projects {
		if matchesAnyProjectPattern(project, opts.IgnoreRepositories) {
			stats.IgnoredRepositoriesRemoved++
			logger.V(2).Info("Excluding ignored repository", "project", project)
			continue
		}
		remaining = append(remaining, project)
	}

	needsRepoInfo := opts.SkipForks || opts.SkipPendingDeletion || len(opts.IgnoreTopics) > 0
	if len(remaining) == 0 || !needsRepoInfo {
		logFilterSummary(logger, stats, len(remaining))
		return remaining, stats, nil
	}

	if providerClient == nil {
		// Fail-open: without a provider client we cannot inspect topics or repo
		// state, so keep everything rather than drop valid projects.
		logger.Info("Git provider client unavailable, keeping repositories that require platform metadata",
			"remaining", len(remaining))
		logFilterSummary(logger, stats, len(remaining))
		return remaining, stats, nil
	}

	const maxConcurrency = 10

	type result struct {
		project string
		info    RepositoryInfo
		err     error
	}

	results := make([]result, len(remaining))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, project := range remaining {
		wg.Add(1)
		go func(idx int, proj string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			info, err := providerClient.GetRepositoryInfo(ctx, proj)
			results[idx] = result{project: proj, info: info, err: err}
		}(i, project)
	}
	wg.Wait()

	filtered := make([]string, 0, len(remaining))
	for _, r := range results {
		if r.err != nil {
			// Fail-open: keep the repo when its metadata could not be fetched,
			// rather than risk excluding a valid project.
			logger.V(1).Info("Failed to fetch repository info, keeping it", "project", r.project, "error", r.err)
			filtered = append(filtered, r.project)
			continue
		}
		if len(opts.IgnoreTopics) > 0 && matchesAnyTopic(r.info.Topics, opts.IgnoreTopics) {
			stats.IgnoredTopicsRemoved++
			logger.V(2).Info("Excluding repository with ignored topic", "project", r.project)
			continue
		}
		if opts.SkipForks && r.info.Fork {
			stats.ForksRemoved++
			logger.V(2).Info("Excluding forked repository", "project", r.project)
			continue
		}
		if opts.SkipPendingDeletion && r.info.PendingDeletion {
			stats.PendingRemoved++
			logger.V(2).Info("Excluding pending-deletion repository", "project", r.project)
			continue
		}
		filtered = append(filtered, r.project)
	}

	logFilterSummary(logger, stats, len(filtered))
	return filtered, stats, nil
}

func logFilterSummary(logger logr.Logger, stats FilterStats, remaining int) {
	if stats.ForksRemoved == 0 && stats.PendingRemoved == 0 &&
		stats.IgnoredRepositoriesRemoved == 0 && stats.IgnoredTopicsRemoved == 0 {
		return
	}
	logger.Info("Filtered discovered repositories",
		"ignoredRepositories", stats.IgnoredRepositoriesRemoved,
		"ignoredTopics", stats.IgnoredTopicsRemoved,
		"forks", stats.ForksRemoved,
		"pendingDeletion", stats.PendingRemoved,
		"remaining", remaining)
}

// matchesAnyProjectPattern reports whether the project path matches at least
// one of the configured ignore patterns.
func matchesAnyProjectPattern(project string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchesProjectPattern(project, pattern) {
			return true
		}
	}
	return false
}

// matchesProjectPattern matches a project path against a case-insensitive
// minimatch-style glob, including "**" for any number of path segments. A
// pattern without "/" also matches the repository name alone, so "legacy-*"
// excludes "org/legacy-api".
func matchesProjectPattern(project, pattern string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	project = strings.ToLower(project)

	if globMatch(pattern, project) {
		return true
	}
	if !strings.Contains(pattern, "/") {
		if idx := strings.LastIndex(project, "/"); idx >= 0 {
			return globMatch(pattern, project[idx+1:])
		}
	}
	return false
}

// globMatch matches a "/"-separated glob against a path, supporting "**" as a
// wildcard for any number of segments. Each other segment is matched with
// path.Match, so "*", "?" and character classes work as usual.
func globMatch(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pattern, name []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			for len(pattern) > 0 && pattern[0] == "**" {
				pattern = pattern[1:]
			}
			if len(pattern) == 0 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if matchSegments(pattern, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		matched, err := path.Match(pattern[0], name[0])
		if err != nil {
			// An invalid segment pattern can only match literally.
			matched = pattern[0] == name[0]
		}
		if !matched {
			return false
		}
		pattern, name = pattern[1:], name[1:]
	}
	return len(name) == 0
}

// matchesAnyTopic reports whether one of the repository topics appears in the
// ignore list, comparing case-insensitively.
func matchesAnyTopic(topics, ignored []string) bool {
	for _, topic := range topics {
		for _, ignoredTopic := range ignored {
			if strings.EqualFold(strings.TrimSpace(topic), strings.TrimSpace(ignoredTopic)) {
				return true
			}
		}
	}
	return false
}
