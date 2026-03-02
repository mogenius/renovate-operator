package webhook

import "testing"

func TestRenovateCheckbox(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		expected bool
	}{
		{
			name:     "valid renovate description with checked checkbox",
			current:  "Some update\n- [x] <!-- rebase-check -->If you want to rebase/retry this MR",
			expected: true,
		},
		{
			name:     "valid renovate description with checked checkbox uppercase X",
			current:  "Some update\n- [X] <!-- rebase-check -->If you want to rebase/retry this MR",
			expected: true,
		},
		{
			name:     "renovate description but no checkbox checked",
			current:  "Some update\n- [ ] <!-- rebase-check -->If you want to rebase/retry this MR",
			expected: false,
		},
		{
			name:     "non-renovate description with checked checkbox",
			current:  "Some update\n- [x] This is not renovate",
			expected: false,
		},
		{
			name:     "valid dependency dashboard with approve-all-pending-prs checkbox",
			current:  "Some update\n- [x] <!-- approve-all-pending-prs -->🔐 **Create all pending approval PRs at once** 🔐",
			expected: true,
		},
		{
			name:     "valid dependency dashboard with approvePr-branch checkbox",
			current:  "\n- [x] <!-- approvePr-branch=renovate/renovate-skopeo-43.x -->chore(deps): update dependency renovate-skopeo to v43.29.2",
			expected: true,
		},
		{
			name:     "empty description",
			current:  "",
			expected: false,
		},
		{
			name: "description without dependencies and no checkbox checked",
			current: `This issue lists Renovate updates and detected dependencies. Read the [Dependency Dashboard](https://docs.renovatebot.com/key-concepts/dashboard/) docs to learn more.
								 This repository currently has no open or pending branches.
								 ## Detected Dependencies
								 None detected`,
			expected: false,
		},
		{
			name: "config-migration checked",
			current: `This issue lists Renovate updates and detected dependencies. Read the [Dependency Dashboard](https://docs.renovatebot.com/key-concepts/dashboard/) docs to learn more.
								## Config Migration Needed
 								- [x] <!-- create-config-migration-pr --> Select this checkbox to let Renovate create an automated Config Migration PR.`,
			expected: true,
		},
		{
			name: "pending approval with approve-branch checked",
			current: `## Pending Approval
								The following branches are pending approval. To create them, click on a checkbox below.
 								- [x] <!-- approve-branch=renovate/python-3.x -->chore: update python docker tag to v3.14`,
			expected: true,
		},
		{
			name: "rebase branch",
			current: `## Open
								The following updates have all been created. To force a retry/rebase of any, click on a checkbox below.
 								- [x] <!-- rebase-branch=renovate/python-reqs -->[chore: update python reqs](../pull/255)`,
			expected: true,
		},
		{
			name: "manual job",
			current: `\n- [x] <!-- manual job -->Check this box to trigger a request for Renovate to run again on this repository\n`,
			expected: true,
		},
		{
			name: "unschedule-branch checked",
			current: `\n- [x] <!-- unschedule-branch=renovate/lock-file-maintenance -->chore(deps): lock file maintenance\n`,
			expected: true,
		},
		{
			name: "create-all-awaiting-schedule-prs checked",
			current: `\n- [x] <!-- create-all-awaiting-schedule-prs -->🔐 **Create all awaiting schedule PRs at once** 🔐\n`,
			expected: true,
		},
		{
			name: "recreate-branch checked",
			current: `## PR Closed (Blocked)
						The following updates are blocked by an existing closed PR. To recreate the PR, click on a checkbox below.
						- [x] <!-- recreate-branch=renovate/harbor-1-18-x -->[fix(deps): update helm release harbor to v1.18.2](pulls/98)`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := verifyRenovateDescriptionChange(tt.current)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
