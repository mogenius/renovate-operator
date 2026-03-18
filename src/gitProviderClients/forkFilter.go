package gitProviderClients

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
)

// FilterForks filters forked repositories from a list of discovered projects
// using the given client. On API errors for individual repos, the repo is kept
// (fail-open) to avoid accidentally excluding valid projects.
func FilterForks(ctx context.Context, providerClient GitProviderClient, logger logr.Logger, projects []string) ([]string, error) {
	if len(projects) == 0 {
		return projects, nil
	}

	const maxConcurrency = 10

	type result struct {
		project string
		isFork  bool
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

			isFork, err := providerClient.IsFork(ctx, proj)
			results[idx] = result{project: proj, isFork: isFork, err: err}
		}(i, project)
	}
	wg.Wait()

	filtered := make([]string, 0, len(projects))
	for _, r := range results {
		if r.err != nil {
			logger.V(1).Info("Failed to check if repo is fork, keeping it", "project", r.project, "error", r.err)
			filtered = append(filtered, r.project)
			continue
		}
		if !r.isFork {
			filtered = append(filtered, r.project)
		} else {
			logger.V(2).Info("Excluding forked repository", "project", r.project)
		}
	}

	removed := len(projects) - len(filtered)
	if removed > 0 {
		logger.Info("Filtered forked repositories", "removed", removed, "remaining", len(filtered))
	}
	return filtered, nil
}
