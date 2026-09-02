package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	api "renovate-operator/api/v1alpha1"
	gitProviderClientFactory "renovate-operator/gitProviderClients/factory"
	crdmanager "renovate-operator/internal/crdManager"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SetupEnvironment carries the install-wide facts the setup guide reports.
// The booleans are fixed at startup. SecretReader is used to verify the
// credential secrets RenovateJobs reference and may be nil, which skips that
// verification; the reads are live because main.go disables the informer
// cache for Secrets.
type SetupEnvironment struct {
	PolicyEnabled        bool
	LogStorageConfigured bool
	// OwnNamespace is the operator's own namespace (POD_NAMESPACE). The guide
	// prefills it and the secret probe is limited to it, so the probe cannot
	// be pointed at arbitrary namespaces.
	OwnNamespace string
	SecretReader client.Reader
}

// Setup step states. "blocked" means the operator found the step attempted but
// broken (missing secret, refused job), which outranks "pending" (not started).
const (
	setupStateDone    = "done"
	setupStatePending = "pending"
	setupStateBlocked = "blocked"
)

// Step and hint IDs are the contract with the frontend, which owns the copy
// for each of them. Details carry the dynamic part (names, refusal reasons).
type SetupStep struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// SetupHint marks an optional subsystem the admin should consider next.
type SetupHint struct {
	ID   string `json:"id"`
	Done bool   `json:"done"`
}

// SetupStatus is the response of /api/v1/setup/status. Visible is false when
// the guide is disabled (an auth provider is configured) or the state cannot
// be computed, in which case every other field is empty so nothing about the
// install leaks.
type SetupStatus struct {
	Visible  bool        `json:"visible"`
	Complete bool        `json:"complete"`
	Steps    []SetupStep `json:"steps,omitempty"`
	Hints    []SetupHint `json:"hints,omitempty"`
	// Namespace is the operator's own namespace, which the guide prefills as
	// the target for its generated manifests — a guessed default would send
	// them to a namespace the operator may not even watch.
	Namespace string `json:"namespace,omitempty"`
}

// setupStatusCache reuses the accessCheckTTL trade-off: the dashboard polls,
// and computing the status lists every RenovateJob, its projects and its
// credential secrets.
type setupStatusCache struct {
	mu       sync.Mutex
	status   *SetupStatus
	computed time.Time
}

func (c *setupStatusCache) load() (*SetupStatus, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.computed.IsZero() || time.Since(c.computed) > accessCheckTTL {
		return nil, false
	}
	return c.status, true
}

func (c *setupStatusCache) store(status *SetupStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = status
	c.computed = time.Now()
}

// setupGuideAvailable reports whether the setup guide exists on this install
// at all. It is a first-run aid: an install with a configured auth provider
// is past first-run setup, so the guide (and its secret probing) is disabled
// entirely rather than gated per session. Without a provider every request
// is an admin anyway, matching decideJobAccess.
func (s *Server) setupGuideAvailable() bool {
	return s.auth == nil
}

// getSetupStatus serves the first-run checklist. When the guide is disabled
// the response is {"visible": false} rather than an error so the frontend
// can probe it unconditionally.
func (s *Server) getSetupStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !s.setupGuideAvailable() {
		_ = json.NewEncoder(w).Encode(SetupStatus{})
		return
	}

	status := s.setupStatus(r.Context())
	_ = json.NewEncoder(w).Encode(&status)
}

// setupStatus returns the current status, computing it when the cache is cold
// so callers other than the status endpoint can depend on it too.
func (s *Server) setupStatus(ctx context.Context) SetupStatus {
	if cached, ok := s.setupCheck.load(); ok && cached != nil {
		return *cached
	}
	status := s.computeSetupStatus(ctx)
	s.setupCheck.store(&status)
	return status
}

// getSetupSecretCheck reports whether the secret the setup guide's first step
// generates exists yet and holds usable keys, so the guide can advance before
// any RenovateJob references it. Only booleans leave the server, never secret
// values, and only for as long as the guide is actually on screen: an auth
// provider, a finished setup or an unreadable state all answer 404. A
// finished first run closes the endpoint for the life of the install — it
// must not linger as a secret existence oracle once nothing consults it.
func (s *Server) getSetupSecretCheck(w http.ResponseWriter, r *http.Request) {
	if !s.setupGuideAvailable() {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Visible and unfinished is exactly the window the guide polls in. An
	// unreadable state reports neither, and closes the probe with it.
	if status := s.setupStatus(r.Context()); !status.Visible || status.Complete {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	name := r.URL.Query().Get("name")
	if namespace == "" || name == "" {
		badRequestError(w, nil, "missing parameters")
		return
	}

	if !s.setupSecretNamespaceAllowed(r.Context(), namespace) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	result := struct {
		Found bool `json:"found"`
		// HasToken: a value at one of Renovate's well-known token keys.
		HasToken bool `json:"hasToken"`
		// HasGithubAppKeys: the key names the guide's GitHub App template uses.
		HasGithubAppKeys bool `json:"hasGithubAppKeys"`
		HasAllowRefLabel bool `json:"hasAllowRefLabel"`
	}{}

	if s.setup.SecretReader != nil {
		secret := &corev1.Secret{}
		if err := s.setup.SecretReader.Get(r.Context(), client.ObjectKey{Name: name, Namespace: namespace}, secret); err == nil {
			result.Found = true
			for _, key := range gitProviderClientFactory.WellKnownTokenKeys {
				if len(secret.Data[key]) > 0 {
					result.HasToken = true
					break
				}
			}
			result.HasGithubAppKeys = len(secret.Data["APP_ID"]) > 0 &&
				len(secret.Data["INSTALL_ID"]) > 0 &&
				len(secret.Data["PEM"]) > 0
			result.HasAllowRefLabel = secret.Labels[api.LabelAllowRef] == "true"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// setupSecretNamespaceAllowed limits the probe to the namespaces the guide has
// a reason to look in: the operator's own, and any that already holds a
// RenovateJob. This endpoint answers without a session (it only exists while
// no auth provider is configured) and the operator's ClusterRole can read
// secrets in every namespace, so an unrestricted namespace parameter would
// turn it into a cluster-wide "does secret X exist" oracle. Names outside
// those namespaces answer 404 — indistinguishable from a disabled guide.
func (s *Server) setupSecretNamespaceAllowed(ctx context.Context, namespace string) bool {
	if s.setup.OwnNamespace != "" && namespace == s.setup.OwnNamespace {
		return true
	}

	jobs, err := s.manager.ListRenovateJobsFull(ctx)
	if err != nil {
		s.logger.Error(err, "failed to list renovatejobs to check the setup secret namespace")
		return false
	}
	for i := range jobs {
		if jobs[i].Namespace == namespace {
			return true
		}
	}

	if s.setup.OwnNamespace == "" {
		// Neither POD_NAMESPACE nor a service account namespace: the operator
		// does not run as a pod, so there is no own namespace to compare
		// against and the check stays closed rather than opening up.
		s.logger.Info("the setup guide's secret check is disabled: the operator's own namespace is unknown "+
			"(no POD_NAMESPACE and no service account namespace — set POD_NAMESPACE for a local run)",
			"namespace", namespace)
		return false
	}

	s.logger.Info("setup guide asked about a secret outside the operator's namespace, refused",
		"namespace", namespace, "ownNamespace", s.setup.OwnNamespace)
	return false
}

func (s *Server) computeSetupStatus(ctx context.Context) SetupStatus {
	jobs, err := s.manager.ListRenovateJobsFull(ctx)
	if err != nil {
		// Without the list every claim below would be a guess. Hide the guide
		// for this cache period; the dashboard's own job fetch surfaces the error.
		s.logger.Error(err, "failed to list renovatejobs for setup status")
		return SetupStatus{}
	}

	// A finished setup stays finished while any job exists: the guide is
	// hidden then, so checking every project and credential secret per cache
	// period would be pure overhead on the steady state. Deleting every job
	// is the admin starting over, and resets the latch.
	if s.setupComplete.Load() {
		if len(jobs) > 0 {
			return SetupStatus{Visible: true, Complete: true, Hints: s.setupHints(), Namespace: s.setup.OwnNamespace}
		}
		s.setupComplete.Store(false)
	}

	credentialsStep := SetupStep{ID: "credentials", State: setupStatePending}
	jobStep := SetupStep{ID: "renovatejob", State: setupStatePending}
	acceptedStep := SetupStep{ID: "accepted", State: setupStatePending}
	discoveryStep := SetupStep{ID: "discovery", State: setupStatePending}

	if len(jobs) > 0 {
		jobStep.State = setupStateDone
		credentialsStep = s.credentialsSetupStep(ctx, jobs)
		acceptedStep = acceptedSetupStep(jobs)
		discoveryStep = s.discoverySetupStep(ctx, jobs)
	}

	// Discovery succeeding is the proof the guide is after: credentials work,
	// the job is accepted and repositories are found. The runs themselves are
	// the dashboard's business, so they are deliberately not a setup step.
	steps := []SetupStep{credentialsStep, jobStep, acceptedStep, discoveryStep}

	complete := true
	for _, step := range steps {
		if step.State != setupStateDone {
			complete = false
			break
		}
	}
	if complete {
		s.setupComplete.Store(true)
	}

	return SetupStatus{
		Visible:   true,
		Complete:  complete,
		Steps:     steps,
		Hints:     s.setupHints(),
		Namespace: s.setup.OwnNamespace,
	}
}

func (s *Server) setupHints() []SetupHint {
	return []SetupHint{
		{ID: "auth", Done: s.auth != nil},
		{ID: "policy", Done: s.setup.PolicyEnabled},
		{ID: "logStorage", Done: s.setup.LogStorageConfigured},
	}
}

// credentialsSetupStep verifies each job's credential reference: the reference
// exists, the secret exists, and it holds a token at a key Renovate reads.
// Verification needs a job that references the secret, so with no problems and
// at least one verified job the step is done.
func (s *Server) credentialsSetupStep(ctx context.Context, jobs []api.RenovateJob) SetupStep {
	var problems []string

	for i := range jobs {
		job := &jobs[i]
		name := job.Namespace + "/" + job.Name

		switch {
		case job.Spec.GithubAppReference != nil:
			if problem := s.checkGithubAppSecret(ctx, job); problem != "" {
				problems = append(problems, name+": "+problem)
			}
		case job.Spec.SecretRef != "":
			if problem := s.checkCredentialSecret(ctx, job); problem != "" {
				problems = append(problems, name+": "+problem)
			}
		default:
			problems = append(problems, name+": references no credentials (set spec.secretRef or spec.githubAppReference)")
		}
	}

	if len(problems) > 0 {
		// The detail is rendered to the admin; keep it short when many jobs
		// share the same misconfiguration.
		const maxProblems = 3
		if len(problems) > maxProblems {
			problems = append(problems[:maxProblems], fmt.Sprintf("and %d more", len(problems)-maxProblems))
		}
		return SetupStep{ID: "credentials", State: setupStateBlocked, Detail: strings.Join(problems, "; ")}
	}

	return SetupStep{ID: "credentials", State: setupStateDone}
}

// checkCredentialSecret reports why a job's spec.secretRef is unusable, or ""
// when it looks fine. A nil SecretReader accepts the reference as-is.
func (s *Server) checkCredentialSecret(ctx context.Context, job *api.RenovateJob) string {
	if s.setup.SecretReader == nil {
		return ""
	}

	secret := &corev1.Secret{}
	if err := s.setup.SecretReader.Get(ctx, client.ObjectKey{Name: job.Spec.SecretRef, Namespace: job.Namespace}, secret); err != nil {
		return fmt.Sprintf("secret %q not found in namespace %q", job.Spec.SecretRef, job.Namespace)
	}

	for _, key := range gitProviderClientFactory.WellKnownTokenKeys {
		if len(secret.Data[key]) > 0 {
			return ""
		}
	}
	return fmt.Sprintf("secret %q holds no platform token (expected one of: %s)",
		job.Spec.SecretRef, strings.Join(gitProviderClientFactory.WellKnownTokenKeys, ", "))
}

// checkGithubAppSecret reports why a job's spec.githubAppReference is
// unusable, or "" when it looks fine. Only the referenced secret is checked;
// the operator-managed token secret is its own reconcile concern.
func (s *Server) checkGithubAppSecret(ctx context.Context, job *api.RenovateJob) string {
	if s.setup.SecretReader == nil {
		return ""
	}

	ref := job.Spec.GithubAppReference
	secret := &corev1.Secret{}
	if err := s.setup.SecretReader.Get(ctx, client.ObjectKey{Name: ref.SecretName, Namespace: job.Namespace}, secret); err != nil {
		return fmt.Sprintf("GitHub App secret %q not found in namespace %q", ref.SecretName, job.Namespace)
	}

	var missing []string
	for _, key := range []string{ref.AppIdSecretKey, ref.InstallationIdSecretKey, ref.PemSecretKey} {
		if len(secret.Data[key]) == 0 {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Sprintf("GitHub App secret %q is missing key(s): %s", ref.SecretName, strings.Join(missing, ", "))
	}
	return ""
}

// acceptedSetupStep folds the jobs' Accepted conditions into one step. A job
// without the condition counts as accepted, matching acceptedState.
func acceptedSetupStep(jobs []api.RenovateJob) SetupStep {
	for i := range jobs {
		if accepted, message := acceptedState(&jobs[i]); !accepted {
			detail := jobs[i].Namespace + "/" + jobs[i].Name + ": halted by the operator's policy"
			if message != "" {
				detail += " — " + message
			}
			return SetupStep{ID: "accepted", State: setupStateBlocked, Detail: detail}
		}
	}
	return SetupStep{ID: "accepted", State: setupStateDone}
}

// discoverySetupStep is done as soon as any project exists — repositories
// were found, so the whole chain provably works.
func (s *Server) discoverySetupStep(ctx context.Context, jobs []api.RenovateJob) SetupStep {
	step := SetupStep{ID: "discovery", State: setupStatePending}

	totalProjects := 0
	for i := range jobs {
		jobId := crdmanager.RenovateJobIdentifier{Name: jobs[i].Name, Namespace: jobs[i].Namespace}
		projects, err := s.manager.GetProjectsForRenovateJob(ctx, jobId)
		if err != nil {
			s.logger.Error(err, "failed to get projects for setup status", "job", jobs[i].Name, "namespace", jobs[i].Namespace)
			continue
		}
		totalProjects += len(projects)
	}

	if totalProjects > 0 {
		step.State = setupStateDone
		step.Detail = fmt.Sprintf("%d project(s) discovered", totalProjects)
	} else if s.anyDiscoveryRunning(ctx, jobs) {
		step.Detail = "discovery is running"
	}

	return step
}

func (s *Server) anyDiscoveryRunning(ctx context.Context, jobs []api.RenovateJob) bool {
	for i := range jobs {
		if status, err := s.discovery.GetDiscoveryJobStatus(ctx, &jobs[i]); err == nil && status == api.JobStatusRunning {
			return true
		}
	}
	return false
}
