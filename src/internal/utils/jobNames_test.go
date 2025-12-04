package utils

import (
	api "renovate-operator/api/v1alpha1"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRenovateJob_JobNames(t *testing.T) {
	tests := []struct {
		name              string
		jobName           string
		project           string
		expectedExecutor  string
		expectedDiscovery string
	}{
		{
			name:              "simple project name",
			jobName:           "my-job",
			project:           "frontend",
			expectedExecutor:  "my-job-frontend-2849ae02",
			expectedDiscovery: "my-job-discovery-065009c5",
		},
		{
			name:              "project with slash",
			jobName:           "my-job",
			project:           "org/repo",
			expectedExecutor:  "my-job-org-repo-8b486ffc",
			expectedDiscovery: "my-job-discovery-065009c5",
		},
		{
			name:              "project with underscore",
			jobName:           "my-job",
			project:           "my_project",
			expectedExecutor:  "my-job-my-project-e4c89720",
			expectedDiscovery: "my-job-discovery-065009c5",
		},
		{
			name:              "project with uppercase",
			jobName:           "my-job",
			project:           "MyProject",
			expectedExecutor:  "my-job-myproject-339414df",
			expectedDiscovery: "my-job-discovery-065009c5",
		},
		{
			name:              "complex project name",
			jobName:           "renovate",
			project:           "Org/My_Repo/SubPath",
			expectedExecutor:  "renovate-org-my-repo-subpath-4130e54c",
			expectedDiscovery: "renovate-discovery-df3ed160",
		},
		{
			name:              "project with multiple slashes and underscores",
			jobName:           "job",
			project:           "org/repo_name",
			expectedExecutor:  "job-org-repo-name-f7f46333",
			expectedDiscovery: "job-discovery-c0002b40",
		},
		{
			name:              "complex project name",
			jobName:           "renovate",
			project:           "Org/.github",
			expectedExecutor:  "renovate-org--github-3ff30a25",
			expectedDiscovery: "renovate-discovery-df3ed160",
		},
		{
			name:              "long project name",
			jobName:           "renovate",
			project:           "Your-very-long-org-name/with.a.lot_of.parts_and--symbols",
			expectedExecutor:  "renovate-your-very-long-org-name-with-a-lot-of-parts-a-2648694c",
			expectedDiscovery: "renovate-discovery-df3ed160",
		},
		{
			name:              "long renovate name",
			jobName:           "this-is-a-very-long-renovate-job-name-to-test-the-trimming-functionality",
			project:           "projcect",
			expectedExecutor:  "this-is-a-very-long-renovate-job-name-to-test-the-trim-5a3f39f4",
			expectedDiscovery: "this-is-a-very-long-renovate-job-name-to-tes-discovery-03ec2833",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rj := &api.RenovateJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.jobName,
				},
			}

			gotExecutor := ExecutorJobName(rj, tt.project)
			if gotExecutor != tt.expectedExecutor {
				t.Errorf("ExecutorJobName() = %v, want %v", gotExecutor, tt.expectedExecutor)
			}

			gotDiscovery := DiscoveryJobName(rj)
			if gotDiscovery != tt.expectedDiscovery {
				t.Errorf("DiscoveryJobName() = %v, want %v", gotDiscovery, tt.expectedDiscovery)
			}
		})
	}
}
