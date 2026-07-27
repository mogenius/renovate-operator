package webhook

import (
	"context"
	"errors"
	"testing"

	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type mockJobLister struct {
	jobs []api.RenovateJob
	err  error
}

func (m *mockJobLister) ListRenovateJobsFull(_ context.Context) ([]api.RenovateJob, error) {
	return m.jobs, m.err
}

func makeJob(name, namespace string, webhookEnabled, authEnabled bool, projects ...string) api.RenovateJob {
	var webhook *api.RenovateWebhook
	if webhookEnabled {
		webhook = &api.RenovateWebhook{Enabled: true}
		if authEnabled {
			webhook.Authentication = &api.RenovateWebhookAuth{Enabled: true}
		}
	}
	statuses := make([]api.ProjectStatus, len(projects))
	for i, p := range projects {
		statuses[i] = api.ProjectStatus{Name: p}
	}
	return api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       api.RenovateJobSpec{Webhook: webhook},
		Status:     api.RenovateJobStatus{Projects: statuses},
	}
}

func passingChecker(_ context.Context, _ crdmanager.RenovateJobIdentifier) (bool, error) {
	return true, nil
}

func failingChecker(_ context.Context, _ crdmanager.RenovateJobIdentifier) (bool, error) {
	return false, nil
}

func errorChecker(_ context.Context, _ crdmanager.RenovateJobIdentifier) (bool, error) {
	return false, errors.New("auth error")
}

func TestFindAndAuthenticateJob(t *testing.T) {
	tests := []struct {
		name      string
		jobs      []api.RenovateJob
		listerErr error
		namespace string
		jobName   string
		project   string
		checker   AuthChecker
		wantId    crdmanager.RenovateJobIdentifier
		wantErr   error // nil = success expected
	}{
		{
			name:    "single match auth disabled returns job",
			jobs:    []api.RenovateJob{makeJob("job1", "ns1", true, false, "org/repo")},
			project: "org/repo",
			wantId:  crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"},
		},
		{
			name:    "single match auth enabled checker passes",
			jobs:    []api.RenovateJob{makeJob("job1", "ns1", true, true, "org/repo")},
			project: "org/repo",
			checker: passingChecker,
			wantId:  crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"},
		},
		{
			name:    "single match auth enabled checker fails",
			jobs:    []api.RenovateJob{makeJob("job1", "ns1", true, true, "org/repo")},
			project: "org/repo",
			checker: failingChecker,
			wantErr: ErrAuthenticationFailed,
		},
		{
			name:    "single match auth enabled checker returns error",
			jobs:    []api.RenovateJob{makeJob("job1", "ns1", true, true, "org/repo")},
			project: "org/repo",
			checker: errorChecker,
			wantErr: ErrAuthenticationFailed,
		},
		{
			name:    "single match auth enabled no checker",
			jobs:    []api.RenovateJob{makeJob("job1", "ns1", true, true, "org/repo")},
			project: "org/repo",
			checker: nil,
			wantErr: ErrAuthenticationFailed,
		},
		{
			name: "multiple matches only one authenticates returns authenticated",
			jobs: []api.RenovateJob{
				makeJob("job1", "ns1", true, true, "org/repo"),
				makeJob("job2", "ns2", true, false, "org/repo"),
			},
			project: "org/repo",
			checker: failingChecker,
			wantId:  crdmanager.RenovateJobIdentifier{Name: "job2", Namespace: "ns2"},
		},
		{
			name: "multiple matches all authenticate returns first",
			jobs: []api.RenovateJob{
				makeJob("job1", "ns1", true, false, "org/repo"),
				makeJob("job2", "ns2", true, false, "org/repo"),
			},
			project: "org/repo",
			wantId:  crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"},
		},
		{
			name: "multiple matches none authenticate",
			jobs: []api.RenovateJob{
				makeJob("job1", "ns1", true, true, "org/repo"),
				makeJob("job2", "ns2", true, true, "org/repo"),
			},
			project: "org/repo",
			checker: failingChecker,
			wantErr: ErrAuthenticationFailed,
		},
		{
			name: "filter by namespace reduces to correct job",
			jobs: []api.RenovateJob{
				makeJob("job1", "ns1", true, false, "org/repo"),
				makeJob("job2", "ns2", true, false, "org/repo"),
			},
			namespace: "ns2",
			project:   "org/repo",
			wantId:    crdmanager.RenovateJobIdentifier{Name: "job2", Namespace: "ns2"},
		},
		{
			name: "filter by job name reduces to correct job",
			jobs: []api.RenovateJob{
				makeJob("job1", "ns1", true, false, "org/repo"),
				makeJob("job2", "ns2", true, false, "org/repo"),
			},
			jobName: "job1",
			project: "org/repo",
			wantId:  crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"},
		},
		{
			name: "filter by namespace and job name",
			jobs: []api.RenovateJob{
				makeJob("job1", "ns1", true, false, "org/repo"),
				makeJob("job1", "ns2", true, false, "org/repo"),
			},
			namespace: "ns2",
			jobName:   "job1",
			project:   "org/repo",
			wantId:    crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns2"},
		},
		{
			name:    "webhook not enabled excluded",
			jobs:    []api.RenovateJob{makeJob("job1", "ns1", false, false, "org/repo")},
			project: "org/repo",
			wantErr: ErrNoMatchingJob,
		},
		{
			name:    "project not in any job",
			jobs:    []api.RenovateJob{makeJob("job1", "ns1", true, false, "other/repo")},
			project: "org/repo",
			wantErr: ErrNoMatchingJob,
		},
		{
			name:    "no jobs at all",
			jobs:    nil,
			project: "org/repo",
			wantErr: ErrNoMatchingJob,
		},
		{
			name:      "lister returns error",
			listerErr: errors.New("k8s api error"),
			project:   "org/repo",
			wantErr:   errors.New("k8s api error"), // any non-nil error
		},
		{
			name:    "job with multiple projects matches correct one",
			jobs:    []api.RenovateJob{makeJob("job1", "ns1", true, false, "other/repo", "org/repo", "another/repo")},
			project: "org/repo",
			wantId:  crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lister := &mockJobLister{jobs: tt.jobs, err: tt.listerErr}

			id, err := FindAndAuthenticateJob(context.Background(), lister, tt.namespace, tt.jobName, tt.project, tt.checker)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("expected error, got nil (id=%+v)", id)
					return
				}
				// For sentinel errors, check with errors.Is; for generic errors just check non-nil.
				if errors.Is(tt.wantErr, ErrNoMatchingJob) || errors.Is(tt.wantErr, ErrAuthenticationFailed) {
					if !errors.Is(err, tt.wantErr) {
						t.Errorf("expected error %v, got %v", tt.wantErr, err)
					}
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if id != tt.wantId {
				t.Errorf("expected id %+v, got %+v", tt.wantId, id)
			}
		})
	}
}
