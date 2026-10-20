package controllers

import (
	"cmp"
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/github"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/policy"
	"renovate-operator/internal/utils"
	"renovate-operator/metricStore"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Reasons of a GitHubAppInstallationSet's Ready condition, beside the policy
// package's refusal reasons.
const (
	ReasonInstallationsSynced    = "Synced"
	ReasonPartiallySynced        = "PartiallySynced"
	ReasonCredentialsUnavailable = "CredentialsUnavailable"
	ReasonListInstallationsError = "ListInstallationsFailed"
)

// Keys of the generated credentials Secret, referenced by the generated
// RenovateJob's githubAppReference.
const (
	credentialsAppIDKey          = "appId"
	credentialsInstallationIDKey = "installationId"
	credentialsPemKey            = "pem"
)

const (
	defaultInstallationResyncInterval = 10 * time.Minute
	// installationRetryInterval paces retries after a failed sync, so a broken
	// credential does not hammer the GitHub API.
	installationRetryInterval = 1 * time.Minute
	// maxLabelValueLength bounds generated RenovateJob names: the name is used
	// as the value of api.LabelRenovateJob on every Job it runs.
	maxLabelValueLength = 63
)

// GitHubAppInstallationSetReconciler generates one RenovateJob, plus a Secret
// holding its App credentials, per installation of a GitHub App.
type GitHubAppInstallationSetReconciler struct {
	K8sClient client.Client
	// APIReader reads generated RenovateJobs uncached: a job created by the
	// previous reconcile may not be in the informer cache yet, and missing it
	// would create a duplicate.
	APIReader               client.Reader
	Installations           github.InstallationLister
	Policy                  policy.Policy
	ClusterTemplatesEnabled bool
}

type appCredentials struct {
	appID string
	pem   string
}

func (r *GitHubAppInstallationSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	set := &api.GitHubAppInstallationSet{}
	if err := r.K8sClient.Get(ctx, req.NamespacedName, set); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !set.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	if reason, msg := r.validateSet(set); reason != "" {
		return r.markNotReady(ctx, set, reason, msg)
	}

	creds, reason, err := r.readCredentials(ctx, set)
	if err != nil {
		return r.markNotReady(ctx, set, reason, err.Error())
	}

	statusBefore := &api.GitHubAppInstallationSetStatus{}
	set.Status.DeepCopyInto(statusBefore)
	interval := resyncInterval(set)
	installations := slices.Clone(set.Status.Installations)
	synced := needsInstallationSync(set, interval, time.Now())
	if synced {
		_, endpoint := utils.GetPlatformAndEndpoint(set.Spec.RenovateProvider())
		listed, err := r.Installations.ListInstallations(ctx, creds.appID, creds.pem, endpoint)
		if err != nil {
			// Generated RenovateJobs are kept: a GitHub outage must not wipe
			// every installation's project history.
			return r.markNotReady(ctx, set, ReasonListInstallationsError, err.Error())
		}
		installations = filterInstallations(listed, set.Spec.Installations)
	}

	failures := r.ensureRenovateJobs(ctx, set, creds, installations)

	set.Status.Installations = installations
	if synced {
		now := metav1.Now()
		set.Status.LastSyncTime = &now
		set.Status.ObservedGeneration = set.Generation
	}
	if len(failures) == 0 {
		setReadyCondition(set, metav1.ConditionTrue, ReasonInstallationsSynced,
			fmt.Sprintf("%d installation(s) have a RenovateJob", len(installations)))
	} else {
		setReadyCondition(set, metav1.ConditionFalse, ReasonPartiallySynced, strings.Join(failures, "; "))
	}
	if !equality.Semantic.DeepEqual(statusBefore, &set.Status) {
		if err := r.K8sClient.Status().Update(ctx, set); err != nil {
			return ctrl.Result{}, fmt.Errorf("updating GitHubAppInstallationSet status: %w", err)
		}
	}

	if err := crdManager.RemoveAnnotation(ctx, r.K8sClient, set, api.TriggerResyncAnnotationKey); err != nil {
		logger.Error(err, "failed to remove resync annotation from GitHubAppInstallationSet")
	}

	if len(failures) > 0 {
		return ctrl.Result{RequeueAfter: installationRetryInterval}, nil
	}
	return ctrl.Result{RequeueAfter: time.Until(set.Status.LastSyncTime.Add(interval))}, nil
}

// validateSet applies the checks a RenovateJob's acceptance gate would apply to
// the fields the set itself uses, before any credential is read.
func (r *GitHubAppInstallationSetReconciler) validateSet(set *api.GitHubAppInstallationSet) (string, string) {
	if !r.ClusterTemplatesEnabled && set.Spec.TemplateRef.IsCluster() {
		return policy.ReasonClusterTemplatesDisabled,
			"ClusterRenovateJobTemplate support is disabled on this operator; use a namespaced RenovateJobTemplate instead"
	}

	provider := set.Spec.RenovateProvider()
	_, endpoint := utils.GetPlatformAndEndpoint(provider)
	if err := r.Policy.ValidateDestination(endpoint, "spec.provider.endpoint"); err != nil {
		return reasonOr(err, policy.ReasonDestinationNotAllowed), err.Error()
	}
	if provider.PublicEndpoint != "" {
		if err := r.Policy.ValidateDestination(provider.PublicEndpoint, "spec.provider.publicEndpoint"); err != nil {
			return reasonOr(err, policy.ReasonDestinationNotAllowed), err.Error()
		}
	}
	return "", ""
}

func (r *GitHubAppInstallationSetReconciler) readCredentials(ctx context.Context, set *api.GitHubAppInstallationSet) (*appCredentials, string, error) {
	ref := set.Spec.App
	secret := &corev1.Secret{}
	if err := r.K8sClient.Get(ctx, client.ObjectKey{Name: ref.SecretName, Namespace: set.Namespace}, secret); err != nil {
		return nil, ReasonCredentialsUnavailable, fmt.Errorf("failed to get github app secret %s: %w", ref.SecretName, err)
	}
	if err := r.Policy.ValidateReferencedSecret(secret); err != nil {
		metricStore.IncPolicyDenial(ctx, "secret_ref")
		return nil, policy.ReasonSecretRefNotOptedIn, err
	}

	appID, ok := secret.Data[ref.AppIdSecretKey]
	if !ok {
		return nil, ReasonCredentialsUnavailable, fmt.Errorf("the given key does not exist in the secret: key=%s, secret=%s", ref.AppIdSecretKey, ref.SecretName)
	}
	pem, ok := secret.Data[ref.PemSecretKey]
	if !ok {
		return nil, ReasonCredentialsUnavailable, fmt.Errorf("the given key does not exist in the secret: key=%s, secret=%s", ref.PemSecretKey, ref.SecretName)
	}
	return &appCredentials{appID: string(appID), pem: string(pem)}, "", nil
}

// ensureRenovateJobs converges the generated RenovateJobs on installations and
// records each one's name in place. It returns one message per installation it
// could not converge.
func (r *GitHubAppInstallationSetReconciler) ensureRenovateJobs(ctx context.Context, set *api.GitHubAppInstallationSet, creds *appCredentials, installations []api.GitHubAppInstallationStatus) []string {
	logger := log.FromContext(ctx)
	var failures []string

	owned, err := r.ownedRenovateJobs(ctx, set)
	if err != nil {
		return []string{err.Error()}
	}

	previouslySuspended := make(map[int64]bool, len(set.Status.Installations))
	for _, inst := range set.Status.Installations {
		previouslySuspended[inst.ID] = inst.Suspended
	}

	reserved := make(map[string]bool, len(owned))
	for _, job := range owned {
		reserved[job.Name] = true
	}

	for i := range installations {
		inst := &installations[i]
		fail := func(err error) {
			failures = append(failures, fmt.Sprintf("installation %d (%s): %s", inst.ID, inst.Account, err))
		}

		existing, ok := owned[inst.ID]
		delete(owned, inst.ID)
		if !ok {
			name, err := r.chooseJobName(ctx, set, inst, reserved)
			if err != nil {
				fail(err)
				continue
			}
			reserved[name] = true
			inst.RenovateJob = name
		} else {
			inst.RenovateJob = existing.Name
		}

		job, err := r.ensureRenovateJob(ctx, set, existing, inst, previouslySuspended[inst.ID])
		if err != nil {
			if existing == nil {
				inst.RenovateJob = ""
			}
			fail(err)
			continue
		}
		if err := r.ensureCredentialsSecret(ctx, set, job, inst.ID, creds); err != nil {
			fail(err)
		}
	}

	for id, job := range owned {
		logger.Info("deleting RenovateJob of a removed GitHub App installation", "renovateJob", job.Name, "installationId", id)
		if err := r.K8sClient.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
			failures = append(failures, fmt.Sprintf("deleting RenovateJob %s: %s", job.Name, err))
		}
	}

	return failures
}

// ownedRenovateJobs returns the RenovateJobs this set controls, keyed by
// installation id.
func (r *GitHubAppInstallationSetReconciler) ownedRenovateJobs(ctx context.Context, set *api.GitHubAppInstallationSet) (map[int64]*api.RenovateJob, error) {
	var jobs api.RenovateJobList
	if err := r.APIReader.List(ctx, &jobs,
		client.InNamespace(set.Namespace),
		client.MatchingLabels{api.LabelInstallationSet: set.Name},
	); err != nil {
		return nil, fmt.Errorf("listing generated RenovateJobs: %w", err)
	}

	owned := make(map[int64]*api.RenovateJob, len(jobs.Items))
	for i := range jobs.Items {
		job := &jobs.Items[i]
		if !metav1.IsControlledBy(job, set) {
			continue
		}
		id, err := strconv.ParseInt(job.Labels[api.LabelInstallationID], 10, 64)
		if err != nil {
			continue
		}
		owned[id] = job
	}
	return owned, nil
}

// chooseJobName names a new RenovateJob after the installation's account, and
// after its id when that name is taken by a RenovateJob this set does not own.
// The name is fixed at creation, so renaming the account keeps the RenovateJob
// and its project history.
func (r *GitHubAppInstallationSetReconciler) chooseJobName(ctx context.Context, set *api.GitHubAppInstallationSet, inst *api.GitHubAppInstallationStatus, reserved map[string]bool) (string, error) {
	candidates := []string{
		generatedJobName(set.Name, inst.Account),
		generatedJobName(set.Name, strconv.FormatInt(inst.ID, 10)),
	}
	for _, name := range candidates {
		if reserved[name] {
			continue
		}
		existing := &api.RenovateJob{}
		err := r.APIReader.Get(ctx, client.ObjectKey{Name: name, Namespace: set.Namespace}, existing)
		if k8serrors.IsNotFound(err) {
			return name, nil
		}
		if err != nil {
			return "", fmt.Errorf("checking RenovateJob name %s: %w", name, err)
		}
	}
	return "", fmt.Errorf("RenovateJobs named %s already exist and are not owned by this GitHubAppInstallationSet", strings.Join(candidates, " and "))
}

func generatedJobName(setName, suffix string) string {
	name := utils.KubernetesCompatibleName(setName + "-" + suffix)
	if len(name) <= maxLabelValueLength {
		return name
	}
	hash := sha256.Sum256([]byte(name))
	return fmt.Sprintf("%s-%x", strings.TrimRight(name[:54], "-"), hash[:4])
}

// ensureRenovateJob creates the installation's RenovateJob named
// inst.RenovateJob when existing is nil, and otherwise patches the fields the
// set manages back onto existing. Fields the set does not manage are left to
// the user.
func (r *GitHubAppInstallationSetReconciler) ensureRenovateJob(ctx context.Context, set *api.GitHubAppInstallationSet, existing *api.RenovateJob, inst *api.GitHubAppInstallationStatus, wasSuspended bool) (*api.RenovateJob, error) {
	job := existing
	if job == nil {
		job = &api.RenovateJob{}
		job.Name = inst.RenovateJob
		job.Namespace = set.Namespace
	}
	original := job.DeepCopy()

	if job.Labels == nil {
		job.Labels = map[string]string{}
	}
	job.Labels[api.LabelAppManagedBy] = api.LabelValueManagedBy
	job.Labels[api.LabelInstallationSet] = set.Name
	job.Labels[api.LabelInstallationID] = strconv.FormatInt(inst.ID, 10)

	templateRef := set.Spec.TemplateRef
	job.Spec.TemplateRef = &templateRef
	job.Spec.Provider = set.Spec.RenovateProvider()
	job.Spec.GithubAppReference = &api.GithubAppReference{
		SecretName:              github.GetNameForGithubAppCredentialsSecret(job.Name),
		AppIdSecretKey:          credentialsAppIDKey,
		InstallationIdSecretKey: credentialsInstallationIDKey,
		PemSecretKey:            credentialsPemKey,
	}
	// A suspended installation cannot mint tokens. Suspend is released only on
	// the transition back, so a suspend set by a user stays untouched.
	if inst.Suspended {
		suspend := true
		job.Spec.Suspend = &suspend
	} else if wasSuspended {
		job.Spec.Suspend = nil
	}

	if err := controllerutil.SetControllerReference(set, job, r.K8sClient.Scheme()); err != nil {
		return nil, fmt.Errorf("setting owner of RenovateJob %s: %w", job.Name, err)
	}

	if existing == nil {
		if err := r.K8sClient.Create(ctx, job); err != nil {
			return nil, fmt.Errorf("creating RenovateJob %s: %w", job.Name, err)
		}
		log.FromContext(ctx).Info("created RenovateJob for GitHub App installation", "renovateJob", job.Name, "installationId", inst.ID, "account", inst.Account)
		return job, nil
	}

	if equality.Semantic.DeepEqual(original, job) {
		return job, nil
	}
	if err := r.K8sClient.Patch(ctx, job, client.MergeFrom(original)); err != nil {
		return nil, fmt.Errorf("patching RenovateJob %s: %w", job.Name, err)
	}
	return job, nil
}

// ensureCredentialsSecret writes the App credentials and the installation id
// the generated RenovateJob's githubAppReference points at. The source Secret
// already passed ValidateReferencedSecret, so the copy is opted in as well.
func (r *GitHubAppInstallationSetReconciler) ensureCredentialsSecret(ctx context.Context, set *api.GitHubAppInstallationSet, job *api.RenovateJob, installationID int64, creds *appCredentials) error {
	secret := &corev1.Secret{}
	secret.Name = github.GetNameForGithubAppCredentialsSecret(job.Name)
	secret.Namespace = set.Namespace
	_, err := controllerutil.CreateOrUpdate(ctx, r.K8sClient, secret, func() error {
		if secret.Labels == nil {
			secret.Labels = map[string]string{}
		}
		secret.Labels[api.LabelAllowRef] = "true"
		secret.Labels[api.LabelAppManagedBy] = api.LabelValueManagedBy
		secret.Labels[api.LabelAppComponent] = api.LabelValueComponentGitHubAppCredentials
		secret.Labels[api.LabelInstallationSet] = set.Name
		secret.Labels[api.LabelInstallationID] = strconv.FormatInt(installationID, 10)
		secret.Data = map[string][]byte{
			credentialsAppIDKey:          []byte(creds.appID),
			credentialsInstallationIDKey: []byte(strconv.FormatInt(installationID, 10)),
			credentialsPemKey:            []byte(creds.pem),
		}
		return controllerutil.SetControllerReference(job, secret, r.K8sClient.Scheme())
	})
	if err != nil {
		return fmt.Errorf("ensuring credentials secret for RenovateJob %s: %w", job.Name, err)
	}
	return nil
}

func (r *GitHubAppInstallationSetReconciler) markNotReady(ctx context.Context, set *api.GitHubAppInstallationSet, reason, message string) (ctrl.Result, error) {
	changed := setReadyCondition(set, metav1.ConditionFalse, reason, message)
	if changed {
		log.FromContext(ctx).Info("GitHubAppInstallationSet not ready", "reason", reason, "message", message)
		if err := r.K8sClient.Status().Update(ctx, set); err != nil {
			return ctrl.Result{}, fmt.Errorf("updating GitHubAppInstallationSet status: %w", err)
		}
	}
	return ctrl.Result{RequeueAfter: installationRetryInterval}, nil
}

func setReadyCondition(set *api.GitHubAppInstallationSet, status metav1.ConditionStatus, reason, message string) bool {
	return meta.SetStatusCondition(&set.Status.Conditions, metav1.Condition{
		Type:               api.ConditionReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: set.Generation,
	})
}

func reasonOr(err error, fallback string) string {
	if reason := policy.ReasonFor(err); reason != "" {
		return reason
	}
	return fallback
}

func resyncInterval(set *api.GitHubAppInstallationSet) time.Duration {
	if set.Spec.ResyncInterval == nil || set.Spec.ResyncInterval.Duration <= 0 {
		return defaultInstallationResyncInterval
	}
	return set.Spec.ResyncInterval.Duration
}

// needsInstallationSync reports whether the installations must be listed from
// the GitHub API again. Otherwise the last listing in the status is converged
// on, so events from owned RenovateJobs never cost an API call.
func needsInstallationSync(set *api.GitHubAppInstallationSet, interval time.Duration, now time.Time) bool {
	if set.Status.LastSyncTime == nil || set.Status.ObservedGeneration != set.Generation {
		return true
	}
	if set.Annotations[api.TriggerResyncAnnotationKey] == "true" {
		return true
	}
	return !now.Before(set.Status.LastSyncTime.Add(interval))
}

func filterInstallations(installations []github.AppInstallation, filter *api.GitHubAppInstallationFilter) []api.GitHubAppInstallationStatus {
	result := make([]api.GitHubAppInstallationStatus, 0, len(installations))
	for _, inst := range installations {
		if !installationMatches(inst, filter) {
			continue
		}
		result = append(result, api.GitHubAppInstallationStatus{
			ID:          inst.ID,
			Account:     inst.Account,
			AccountType: inst.AccountType,
			Suspended:   inst.Suspended,
		})
	}
	slices.SortFunc(result, func(a, b api.GitHubAppInstallationStatus) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return result
}

func installationMatches(inst github.AppInstallation, filter *api.GitHubAppInstallationFilter) bool {
	if filter == nil {
		return true
	}
	matchesLogin := func(login string) bool { return strings.EqualFold(login, inst.Account) }
	if slices.ContainsFunc(filter.Exclude, matchesLogin) {
		return false
	}
	if len(filter.Include) > 0 && !slices.ContainsFunc(filter.Include, matchesLogin) {
		return false
	}
	if len(filter.AccountTypes) > 0 && !slices.Contains(filter.AccountTypes, inst.AccountType) {
		return false
	}
	return true
}

func (r *GitHubAppInstallationSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.GitHubAppInstallationSet{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{}),
		)).
		// Generation only: status writes on every RenovateJob reconcile must not
		// re-enqueue the set. Deletes always pass, so a removed job is recreated.
		Owns(&api.RenovateJob{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}
