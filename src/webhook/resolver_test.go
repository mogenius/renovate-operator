package webhook

import (
	"context"
	"errors"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// testJobEntry bundles a RenovateJob with the project names it owns (stored in
// separate RenovateProject CRDs in production, but kept inline here for test convenience).
type testJobEntry struct {
	job      api.RenovateJob
	projects []string
}

type mockJobLister struct {
	entries []testJobEntry
	err     error
}

func (m *mockJobLister) ListEffectiveRenovateJobsFull(_ context.Context) ([]api.RenovateJob, error) {
	if m.err != nil {
		return nil, m.err
	}
	jobs := make([]api.RenovateJob, len(m.entries))
	for i, e := range m.entries {
		jobs[i] = e.job
	}
	return jobs, nil
}

func (m *mockJobLister) GetProjectsForRenovateJob(_ context.Context, job crdmanager.RenovateJobIdentifier) ([]crdmanager.RenovateProjectStatus, error) {
	for _, e := range m.entries {
		if e.job.Name == job.Name && e.job.Namespace == job.Namespace {
			result := make([]crdmanager.RenovateProjectStatus, len(e.projects))
			for i, p := range e.projects {
				result[i] = crdmanager.RenovateProjectStatus{Name: p}
			}
			return result, nil
		}
	}
	return nil, nil
}

func (m *mockJobLister) GetRenovateJob(_ context.Context, name, namespace string) (*api.RenovateJob, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, e := range m.entries {
		if e.job.Name == name && e.job.Namespace == namespace {
			job := e.job
			return &job, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "renovatejobs"}, name)
}

func makeEntry(name, namespace string, webhookEnabled, authEnabled bool, projects ...string) testJobEntry {
	var webhook *api.RenovateWebhook
	if webhookEnabled {
		webhook = &api.RenovateWebhook{Enabled: true}
		if authEnabled {
			webhook.Authentication = &api.RenovateWebhookAuth{Enabled: true}
		}
	}
	return testJobEntry{
		job: api.RenovateJob{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       api.RenovateJobSpec{Webhook: webhook},
		},
		projects: projects,
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
		entries   []testJobEntry
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
			entries: []testJobEntry{makeEntry("job1", "ns1", true, false, "org/repo")},
			project: "org/repo",
			wantId:  crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"},
		},
		{
			name:    "single match auth enabled checker passes",
			entries: []testJobEntry{makeEntry("job1", "ns1", true, true, "org/repo")},
			project: "org/repo",
			checker: passingChecker,
			wantId:  crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"},
		},
		{
			name:    "single match auth enabled checker fails",
			entries: []testJobEntry{makeEntry("job1", "ns1", true, true, "org/repo")},
			project: "org/repo",
			checker: failingChecker,
			wantErr: ErrAuthenticationFailed,
		},
		{
			name:    "single match auth enabled checker returns error",
			entries: []testJobEntry{makeEntry("job1", "ns1", true, true, "org/repo")},
			project: "org/repo",
			checker: errorChecker,
			wantErr: ErrAuthenticationFailed,
		},
		{
			name:    "single match auth enabled no checker",
			entries: []testJobEntry{makeEntry("job1", "ns1", true, true, "org/repo")},
			project: "org/repo",
			checker: nil,
			wantErr: ErrAuthenticationFailed,
		},
		{
			name: "multiple matches only one authenticates returns authenticated",
			entries: []testJobEntry{
				makeEntry("job1", "ns1", true, true, "org/repo"),
				makeEntry("job2", "ns2", true, false, "org/repo"),
			},
			project: "org/repo",
			checker: failingChecker,
			wantId:  crdmanager.RenovateJobIdentifier{Name: "job2", Namespace: "ns2"},
		},
		{
			name: "multiple matches all authenticate returns first",
			entries: []testJobEntry{
				makeEntry("job1", "ns1", true, false, "org/repo"),
				makeEntry("job2", "ns2", true, false, "org/repo"),
			},
			project: "org/repo",
			wantId:  crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"},
		},
		{
			name: "multiple matches none authenticate",
			entries: []testJobEntry{
				makeEntry("job1", "ns1", true, true, "org/repo"),
				makeEntry("job2", "ns2", true, true, "org/repo"),
			},
			project: "org/repo",
			checker: failingChecker,
			wantErr: ErrAuthenticationFailed,
		},
		{
			name: "filter by namespace reduces to correct job",
			entries: []testJobEntry{
				makeEntry("job1", "ns1", true, false, "org/repo"),
				makeEntry("job2", "ns2", true, false, "org/repo"),
			},
			namespace: "ns2",
			project:   "org/repo",
			wantId:    crdmanager.RenovateJobIdentifier{Name: "job2", Namespace: "ns2"},
		},
		{
			name: "filter by job name reduces to correct job",
			entries: []testJobEntry{
				makeEntry("job1", "ns1", true, false, "org/repo"),
				makeEntry("job2", "ns2", true, false, "org/repo"),
			},
			jobName: "job1",
			project: "org/repo",
			wantId:  crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"},
		},
		{
			name: "filter by namespace and job name",
			entries: []testJobEntry{
				makeEntry("job1", "ns1", true, false, "org/repo"),
				makeEntry("job1", "ns2", true, false, "org/repo"),
			},
			namespace: "ns2",
			jobName:   "job1",
			project:   "org/repo",
			wantId:    crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns2"},
		},
		{
			name:    "webhook not enabled excluded",
			entries: []testJobEntry{makeEntry("job1", "ns1", false, false, "org/repo")},
			project: "org/repo",
			wantErr: ErrNoMatchingJob,
		},
		{
			name:    "project not in any job",
			entries: []testJobEntry{makeEntry("job1", "ns1", true, false, "other/repo")},
			project: "org/repo",
			wantErr: ErrNoMatchingJob,
		},
		{
			name:    "no jobs at all",
			entries: nil,
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
			entries: []testJobEntry{makeEntry("job1", "ns1", true, false, "other/repo", "org/repo", "another/repo")},
			project: "org/repo",
			wantId:  crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lister := &mockJobLister{entries: tt.entries, err: tt.listerErr}

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

// A checker that errors (the webhook secret could not be read) is still reported as an
// authentication failure, but the cause must survive so the "webhook resolve failed" log
// line says why instead of looking like a credential mismatch.
func TestFindAndAuthenticateJobKeepsCheckerErrorCause(t *testing.T) {
	lister := &mockJobLister{entries: []testJobEntry{
		makeEntry("job1", "ns1", true, true, "org/repo"),
		makeEntry("job2", "ns1", true, true, "org/repo"),
	}}

	_, err := FindAndAuthenticateJob(context.Background(), lister, "", "", "org/repo", errorChecker)
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("expected ErrAuthenticationFailed, got %v", err)
	}
	for _, want := range []string{"ns1/job1: auth error", "ns1/job2: auth error"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected %q in %q", want, err.Error())
		}
	}

	_, err = FindAndAuthenticateJob(context.Background(), lister, "", "", "org/repo", failingChecker)
	if err != ErrAuthenticationFailed {
		t.Errorf("a plain mismatch should return the bare sentinel, got %v", err)
	}
}

func TestAuthenticateJobForAllProjects(t *testing.T) {
	tests := []struct {
		name      string
		entries   []testJobEntry
		listerErr error
		namespace string
		jobName   string
		checker   AuthChecker
		wantId    crdmanager.RenovateJobIdentifier
		wantErr   error // nil = success expected
		wantMsg   string
	}{
		{
			name:      "auth enabled checker passes",
			entries:   []testJobEntry{makeEntry("job1", "ns1", true, true, "org/repo")},
			namespace: "ns1",
			jobName:   "job1",
			checker:   passingChecker,
			wantId:    crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"},
		},
		{
			name:      "job without discovered projects still resolves",
			entries:   []testJobEntry{makeEntry("job1", "ns1", true, true)},
			namespace: "ns1",
			jobName:   "job1",
			checker:   passingChecker,
			wantId:    crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"},
		},
		{
			name: "only the named job in the named namespace is used",
			entries: []testJobEntry{
				makeEntry("job1", "ns2", true, false, "org/repo"),
				makeEntry("job2", "ns1", true, false, "org/repo"),
				makeEntry("job1", "ns1", true, true, "org/repo"),
			},
			namespace: "ns1",
			jobName:   "job1",
			checker:   passingChecker,
			wantId:    crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"},
		},
		{
			name:      "unknown job",
			entries:   []testJobEntry{makeEntry("job1", "ns1", true, true, "org/repo")},
			namespace: "ns1",
			jobName:   "job2",
			checker:   passingChecker,
			wantErr:   ErrNoMatchingJob,
		},
		{
			name:      "job name matches in another namespace only",
			entries:   []testJobEntry{makeEntry("job1", "ns2", true, true, "org/repo")},
			namespace: "ns1",
			jobName:   "job1",
			checker:   passingChecker,
			wantErr:   ErrNoMatchingJob,
		},
		{
			name:      "webhook disabled",
			entries:   []testJobEntry{makeEntry("job1", "ns1", false, false, "org/repo")},
			namespace: "ns1",
			jobName:   "job1",
			checker:   passingChecker,
			wantErr:   ErrNoMatchingJob,
		},
		{
			name:      "auth disabled is refused even with a credential",
			entries:   []testJobEntry{makeEntry("job1", "ns1", true, false, "org/repo")},
			namespace: "ns1",
			jobName:   "job1",
			checker:   passingChecker,
			wantErr:   ErrAuthenticationFailed,
			wantMsg:   "ns1/job1 has webhook authentication disabled",
		},
		{
			name:      "auth enabled no credential",
			entries:   []testJobEntry{makeEntry("job1", "ns1", true, true, "org/repo")},
			namespace: "ns1",
			jobName:   "job1",
			checker:   nil,
			wantErr:   ErrAuthenticationFailed,
		},
		{
			name:      "auth enabled checker fails",
			entries:   []testJobEntry{makeEntry("job1", "ns1", true, true, "org/repo")},
			namespace: "ns1",
			jobName:   "job1",
			checker:   failingChecker,
			wantErr:   ErrAuthenticationFailed,
		},
		{
			name:      "auth enabled checker returns error",
			entries:   []testJobEntry{makeEntry("job1", "ns1", true, true, "org/repo")},
			namespace: "ns1",
			jobName:   "job1",
			checker:   errorChecker,
			wantErr:   ErrAuthenticationFailed,
			wantMsg:   "ns1/job1: auth error",
		},
		{
			name:      "lookup error is returned",
			listerErr: errors.New("get failed"),
			namespace: "ns1",
			jobName:   "job1",
			checker:   passingChecker,
			wantErr:   errors.New("get failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lister := &mockJobLister{entries: tt.entries, err: tt.listerErr}
			id, err := AuthenticateJobForAllProjects(context.Background(), lister, tt.namespace, tt.jobName, tt.checker)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) && err.Error() != tt.wantErr.Error() {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				if !strings.Contains(err.Error(), tt.wantMsg) {
					t.Errorf("expected %q in %q", tt.wantMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tt.wantId {
				t.Errorf("expected %+v, got %+v", tt.wantId, id)
			}
		})
	}
}
