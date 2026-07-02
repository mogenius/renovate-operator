package gitProviderClients

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
)

// FilterStats reports how many repositories were dropped by each criterion so
// callers (which know the namespace/job labels) can record the
// repositories_filtered metric.
type FilterStats struct {
	ForksRemoved   int
	PendingRemoved int
}

// FilterProjects filters out repositories that should be skipped during
// discovery according to the enabled criteria:
//   - skipForks excludes forked repositories
//   - skipPendingDeletion excludes repositories marked for delayed deletion
//     (only GitLab reports this state)
//
// Repository metadata is fetched once per project (a single platform API call),
// so enabling both criteria does not double the number of requests. On API
// errors for individual repos, the repo is kept (fail-open) to avoid
// accidentally excluding valid projects.
func FilterProjects(ctx context.Context, providerClient GitProviderClient, logger logr.Logger, projects []string, skipForks, skipPendingDeletion bool) ([]string, FilterStats, error) {
	if len(projects) == 0 || (!skipForks && !skipPendingDeletion) {
		return projects, FilterStats{}, nil
	}

	const maxConcurrency = 10

	type result struct {
		project string
		info    RepositoryInfo
		err     error
	}

	results := make([]result, len(projects))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, project := range projects {
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

	filtered := make([]string, 0, len(projects))
	var forksRemoved, pendingRemoved int
	for _, r := range results {
		if r.err != nil {
			// Fail-open: keep the repo when its metadata could not be fetched,
			// rather than risk excluding a valid project.
			logger.V(1).Info("Failed to fetch repository info, keeping it", "project", r.project, "error", r.err)
			filtered = append(filtered, r.project)
			continue
		}
		if skipForks && r.info.Fork {
			forksRemoved++
			logger.V(2).Info("Excluding forked repository", "project", r.project)
			continue
		}
		if skipPendingDeletion && r.info.PendingDeletion {
			pendingRemoved++
			logger.V(2).Info("Excluding pending-deletion repository", "project", r.project)
			continue
		}
		filtered = append(filtered, r.project)
	}

	if forksRemoved > 0 || pendingRemoved > 0 {
		logger.Info("Filtered discovered repositories", "forks", forksRemoved, "pendingDeletion", pendingRemoved, "remaining", len(filtered))
	}
	return filtered, FilterStats{ForksRemoved: forksRemoved, PendingRemoved: pendingRemoved}, nil
}
