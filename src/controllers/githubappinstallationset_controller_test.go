package controllers

import (
	"context"
	"errors"
	"testing"
	"time"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/github"
	"renovate-operator/internal/policy"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const testSetNamespace = "renovate"

type fakeInstallationLister struct {
	installations []github.AppInstallation
	err           error
	calls         int
}

func (f *fakeInstallationLister) ListInstallations(_ context.Context, appID, pem, githubApi string) ([]github.AppInstallation, error) {
	f.calls++
	return f.installations, f.err
}

func newInstallationSet(name string) *api.GitHubAppInstallationSet {
	set := &api.GitHubAppInstallationSet{
		Spec: api.GitHubAppInstallationSetSpec{
			App: api.GitHubAppCredentialsReference{
				SecretName:     "app-credentials",
				AppIdSecretKey: "APP_ID",
				PemSecretKey:   "PEM",
			},
			TemplateRef: api.RenovateJobTemplateRef{Kind: api.KindRenovateJobTemplate, Name: "defaults"},
		},
	}
	set.Name = name
	set.Namespace = testSetNamespace
	set.Generation = 1
	set.UID = k8stypes.UID("set-uid-" + name)
	return set
}

func appCredentialsSecret(labelled bool) *corev1.Secret {
	secret := &corev1.Secret{Data: map[string][]byte{"APP_ID": []byte("42"), "PEM": []byte("pem-data")}}
	secret.Name = "app-credentials"
	secret.Namespace = testSetNamespace
	if labelled {
		secret.Labels = map[string]string{api.LabelAllowRef: "true"}
	}
	return secret
}

func installationSetClient(t *testing.T, objs ...crclient.Object) crclient.Client {
	t.Helper()
	return installationSetClientWithInterceptor(t, interceptor.Funcs{}, objs...)
}

func installationSetClientWithInterceptor(t *testing.T, funcs interceptor.Funcs, objs ...crclient.Object) crclient.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&api.GitHubAppInstallationSet{}).
		WithInterceptorFuncs(funcs).
		Build()
}

func installationSetReconciler(c crclient.Client, lister github.InstallationLister) *GitHubAppInstallationSetReconciler {
	return &GitHubAppInstallationSetReconciler{
		K8sClient:               c,
		APIReader:               c,
		Installations:           lister,
		Policy:                  policy.Policy{Disabled: true},
		ClusterTemplatesEnabled: true,
	}
}

func reconcileSet(t *testing.T, r *GitHubAppInstallationSetReconciler, name string) ctrl.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: name, Namespace: testSetNamespace}})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	return res
}

func getSet(t *testing.T, c crclient.Client, name string) *api.GitHubAppInstallationSet {
	t.Helper()
	set := &api.GitHubAppInstallationSet{}
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: name, Namespace: testSetNamespace}, set); err != nil {
		t.Fatalf("failed to get set: %v", err)
	}
	return set
}

func getJob(t *testing.T, c crclient.Client, name string) *api.RenovateJob {
	t.Helper()
	job := &api.RenovateJob{}
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: name, Namespace: testSetNamespace}, job); err != nil {
		t.Fatalf("failed to get RenovateJob %s: %v", name, err)
	}
	return job
}

func TestInstallationSetCreatesRenovateJobAndCredentialsPerInstallation(t *testing.T) {
	set := newInstallationSet("club")
	set.Spec.Provider = &api.GitHubAppProvider{Endpoint: "https://ghe.example.com/api/v3"}
	c := installationSetClient(t, set, appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 7, Account: "Alice", AccountType: api.GitHubAccountTypeUser},
		{ID: 3, Account: "tech-club", AccountType: api.GitHubAccountTypeOrganization},
	}}

	res := reconcileSet(t, installationSetReconciler(c, lister), "club")

	job := getJob(t, c, "club-alice")
	if !metav1.IsControlledBy(job, set) {
		t.Error("generated RenovateJob must be controlled by the set")
	}
	if job.Labels[api.LabelInstallationID] != "7" || job.Labels[api.LabelInstallationSet] != "club" {
		t.Errorf("unexpected labels %v", job.Labels)
	}
	if job.Spec.TemplateRef == nil || job.Spec.TemplateRef.Name != "defaults" {
		t.Errorf("templateRef not propagated: %#v", job.Spec.TemplateRef)
	}
	if !job.Spec.Provider.Is("github") || job.Spec.Provider.Endpoint != "https://ghe.example.com/api/v3" {
		t.Errorf("provider not propagated: %#v", job.Spec.Provider)
	}
	ref := job.Spec.GithubAppReference
	if ref == nil || ref.SecretName != github.GetNameForGithubAppCredentialsSecret("club-alice") {
		t.Fatalf("githubAppReference not pointing at the credentials secret: %#v", ref)
	}

	secret := &corev1.Secret{}
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: ref.SecretName, Namespace: testSetNamespace}, secret); err != nil {
		t.Fatalf("credentials secret missing: %v", err)
	}
	if string(secret.Data[ref.AppIdSecretKey]) != "42" ||
		string(secret.Data[ref.InstallationIdSecretKey]) != "7" ||
		string(secret.Data[ref.PemSecretKey]) != "pem-data" {
		t.Errorf("unexpected credentials secret data %v", secret.Data)
	}
	if secret.Labels[api.LabelAllowRef] != "true" {
		t.Error("credentials secret must be opted in to being referenced")
	}
	if !metav1.IsControlledBy(secret, job) {
		t.Error("credentials secret must be controlled by its RenovateJob")
	}

	getJob(t, c, "club-tech-club")

	got := getSet(t, c, "club")
	if len(got.Status.Installations) != 2 || got.Status.Installations[0].ID != 3 || got.Status.Installations[1].RenovateJob != "club-alice" {
		t.Errorf("unexpected status installations %#v", got.Status.Installations)
	}
	if cond := meta.FindStatusCondition(got.Status.Conditions, api.ConditionReady); cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("expected Ready=True, got %#v", cond)
	}
	if got.Status.ObservedGeneration != 1 || got.Status.LastSyncTime == nil {
		t.Errorf("sync not recorded: %#v", got.Status)
	}
	if res.RequeueAfter <= 0 || res.RequeueAfter > defaultInstallationResyncInterval {
		t.Errorf("expected requeue within the resync interval, got %s", res.RequeueAfter)
	}
}

func TestInstallationSetDeletesRenovateJobOfRemovedInstallation(t *testing.T) {
	set := newInstallationSet("club")
	set.Annotations = map[string]string{api.TriggerResyncAnnotationKey: "true"}
	c := installationSetClient(t, set, appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "alice", AccountType: api.GitHubAccountTypeUser},
		{ID: 2, Account: "bob", AccountType: api.GitHubAccountTypeUser},
	}}
	r := installationSetReconciler(c, lister)
	reconcileSet(t, r, "club")

	lister.installations = lister.installations[:1]
	setAnnotationForTest(t, c, "club", api.TriggerResyncAnnotationKey, "true")
	reconcileSet(t, r, "club")

	getJob(t, c, "club-alice")
	err := c.Get(context.Background(), k8stypes.NamespacedName{Name: "club-bob", Namespace: testSetNamespace}, &api.RenovateJob{})
	if !kerrors.IsNotFound(err) {
		t.Errorf("RenovateJob of the removed installation must be deleted, got %v", err)
	}
}

func TestInstallationSetKeepsRenovateJobsWhenListingFails(t *testing.T) {
	set := newInstallationSet("club")
	c := installationSetClient(t, set, appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "alice", AccountType: api.GitHubAccountTypeUser},
	}}
	r := installationSetReconciler(c, lister)
	reconcileSet(t, r, "club")

	lister.err = errors.New("github is down")
	setAnnotationForTest(t, c, "club", api.TriggerResyncAnnotationKey, "true")
	res := reconcileSet(t, r, "club")

	getJob(t, c, "club-alice")
	got := getSet(t, c, "club")
	cond := meta.FindStatusCondition(got.Status.Conditions, api.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != ReasonListInstallationsError {
		t.Errorf("expected Ready=False/%s, got %#v", ReasonListInstallationsError, cond)
	}
	if res.RequeueAfter != installationRetryInterval {
		t.Errorf("expected retry after %s, got %s", installationRetryInterval, res.RequeueAfter)
	}
}

func TestInstallationSetListsOnlyWhenSyncIsDue(t *testing.T) {
	set := newInstallationSet("club")
	c := installationSetClient(t, set, appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "alice", AccountType: api.GitHubAccountTypeUser},
	}}
	r := installationSetReconciler(c, lister)

	reconcileSet(t, r, "club")
	reconcileSet(t, r, "club")
	if lister.calls != 1 {
		t.Fatalf("a fresh sync must not list again, got %d calls", lister.calls)
	}

	setAnnotationForTest(t, c, "club", api.TriggerResyncAnnotationKey, "true")
	reconcileSet(t, r, "club")
	if lister.calls != 2 {
		t.Fatalf("the resync annotation must force a listing, got %d calls", lister.calls)
	}
	if _, ok := getSet(t, c, "club").Annotations[api.TriggerResyncAnnotationKey]; ok {
		t.Error("the resync annotation must be removed after the sync")
	}
}

func TestInstallationSetRecreatesDeletedRenovateJobWithoutListing(t *testing.T) {
	set := newInstallationSet("club")
	c := installationSetClient(t, set, appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "alice", AccountType: api.GitHubAccountTypeUser},
	}}
	r := installationSetReconciler(c, lister)
	reconcileSet(t, r, "club")

	if err := c.Delete(context.Background(), getJob(t, c, "club-alice")); err != nil {
		t.Fatalf("failed to delete job: %v", err)
	}
	reconcileSet(t, r, "club")

	getJob(t, c, "club-alice")
	if lister.calls != 1 {
		t.Errorf("recreating from the last listing must not call the API, got %d calls", lister.calls)
	}
}

func TestInstallationSetFallsBackToInstallationIDOnNameConflict(t *testing.T) {
	set := newInstallationSet("club")
	foreign := &api.RenovateJob{}
	foreign.Name = "club-alice"
	foreign.Namespace = testSetNamespace
	c := installationSetClient(t, set, appCredentialsSecret(true), foreign)
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 99, Account: "alice", AccountType: api.GitHubAccountTypeUser},
	}}

	reconcileSet(t, installationSetReconciler(c, lister), "club")

	job := getJob(t, c, "club-99")
	if !metav1.IsControlledBy(job, set) {
		t.Error("fallback RenovateJob must be controlled by the set")
	}
	if untouched := getJob(t, c, "club-alice"); len(untouched.OwnerReferences) != 0 {
		t.Error("a RenovateJob the set does not own must not be adopted")
	}
}

func TestInstallationSetSuspendsRenovateJobOfSuspendedInstallation(t *testing.T) {
	set := newInstallationSet("club")
	c := installationSetClient(t, set, appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "alice", AccountType: api.GitHubAccountTypeUser, Suspended: true},
	}}
	r := installationSetReconciler(c, lister)
	reconcileSet(t, r, "club")

	if !getJob(t, c, "club-alice").Spec.GetSuspend() {
		t.Fatal("RenovateJob of a suspended installation must be suspended")
	}

	lister.installations[0].Suspended = false
	setAnnotationForTest(t, c, "club", api.TriggerResyncAnnotationKey, "true")
	reconcileSet(t, r, "club")

	if getJob(t, c, "club-alice").Spec.Suspend != nil {
		t.Error("suspend must be released once the installation is unsuspended")
	}
}

func TestInstallationSetKeepsUserSuspendAcrossInstallationSuspend(t *testing.T) {
	set := newInstallationSet("club")
	c := installationSetClient(t, set, appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "alice", AccountType: api.GitHubAccountTypeUser},
	}}
	r := installationSetReconciler(c, lister)
	reconcileSet(t, r, "club")

	job := getJob(t, c, "club-alice")
	suspend := true
	job.Spec.Suspend = &suspend
	if err := c.Update(context.Background(), job); err != nil {
		t.Fatalf("failed to suspend job: %v", err)
	}

	lister.installations[0].Suspended = true
	setAnnotationForTest(t, c, "club", api.TriggerResyncAnnotationKey, "1")
	reconcileSet(t, r, "club")
	lister.installations[0].Suspended = false
	setAnnotationForTest(t, c, "club", api.TriggerResyncAnnotationKey, "2")
	reconcileSet(t, r, "club")

	if !getJob(t, c, "club-alice").Spec.GetSuspend() {
		t.Error("a suspend set by a user must survive the installation being suspended and unsuspended")
	}
}

func TestInstallationSetRetriesReleasingSuspendAfterFailedPatch(t *testing.T) {
	set := newInstallationSet("club")
	failPatch := false
	c := installationSetClientWithInterceptor(t, interceptor.Funcs{
		Patch: func(ctx context.Context, c crclient.WithWatch, obj crclient.Object, patch crclient.Patch, opts ...crclient.PatchOption) error {
			if _, ok := obj.(*api.RenovateJob); ok && failPatch {
				failPatch = false
				return errors.New("conflict")
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	}, set, appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "alice", AccountType: api.GitHubAccountTypeUser, Suspended: true},
	}}
	r := installationSetReconciler(c, lister)
	reconcileSet(t, r, "club")

	lister.installations[0].Suspended = false
	setAnnotationForTest(t, c, "club", api.TriggerResyncAnnotationKey, "1")
	failPatch = true
	reconcileSet(t, r, "club")
	if !getJob(t, c, "club-alice").Spec.GetSuspend() {
		t.Fatal("test setup: the failed patch must leave the job suspended")
	}

	reconcileSet(t, r, "club")
	if getJob(t, c, "club-alice").Spec.GetSuspend() {
		t.Error("the suspend must be released on the retry after a failed patch")
	}
}

// A request arriving before the status write makes that write conflict; one
// arriving after it must make the annotation's removal conflict.
func TestInstallationSetKeepsResyncRequestedDuringSync(t *testing.T) {
	set := newInstallationSet("club")
	var c crclient.Client
	reannotate := false
	c = installationSetClientWithInterceptor(t, interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, sc crclient.Client, subResourceName string, obj crclient.Object, opts ...crclient.SubResourceUpdateOption) error {
			if err := sc.SubResource(subResourceName).Update(ctx, obj, opts...); err != nil {
				return err
			}
			if reannotate {
				reannotate = false
				setAnnotationForTest(t, c, "club", api.TriggerResyncAnnotationKey, "2")
			}
			return nil
		},
	}, set, appCredentialsSecret(true))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "alice", AccountType: api.GitHubAccountTypeUser},
	}}
	r := installationSetReconciler(c, lister)
	reconcileSet(t, r, "club")

	setAnnotationForTest(t, c, "club", api.TriggerResyncAnnotationKey, "1")
	reannotate = true
	reconcileSet(t, r, "club")

	if got := getSet(t, c, "club").Annotations[api.TriggerResyncAnnotationKey]; got != "2" {
		t.Errorf("a resync requested during the sync must be kept, got annotation %q", got)
	}
}

func TestInstallationSetRefusesUnlabelledCredentialsUnderPolicy(t *testing.T) {
	set := newInstallationSet("club")
	c := installationSetClient(t, set, appCredentialsSecret(false))
	lister := &fakeInstallationLister{}
	r := installationSetReconciler(c, lister)
	r.Policy = policy.Policy{AllowedHosts: []string{"api.github.com"}}

	reconcileSet(t, r, "club")

	if lister.calls != 0 {
		t.Error("credentials refused by policy must not be used")
	}
	cond := meta.FindStatusCondition(getSet(t, c, "club").Status.Conditions, api.ConditionReady)
	if cond == nil || cond.Reason != policy.ReasonSecretRefNotOptedIn {
		t.Errorf("expected Ready=False/%s, got %#v", policy.ReasonSecretRefNotOptedIn, cond)
	}
}

func TestInstallationSetMirrorsAllowRefOfSourceSecret(t *testing.T) {
	set := newInstallationSet("club")
	c := installationSetClient(t, set, appCredentialsSecret(false))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "alice", AccountType: api.GitHubAccountTypeUser},
	}}
	reconcileSet(t, installationSetReconciler(c, lister), "club")

	secret := &corev1.Secret{}
	name := github.GetNameForGithubAppCredentialsSecret("club-alice")
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: name, Namespace: testSetNamespace}, secret); err != nil {
		t.Fatalf("credentials secret missing: %v", err)
	}
	if _, ok := secret.Labels[api.LabelAllowRef]; ok {
		t.Error("the copy of an unlabelled source secret must not be opted in")
	}
}

func TestInstallationSetRevokesCredentialsWhenSourceIsRefused(t *testing.T) {
	set := newInstallationSet("club")
	source := appCredentialsSecret(true)
	c := installationSetClient(t, set, source)
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "alice", AccountType: api.GitHubAccountTypeUser},
	}}
	r := installationSetReconciler(c, lister)
	r.Policy = policy.Policy{AllowedHosts: []string{"api.github.com"}}
	reconcileSet(t, r, "club")

	job := getJob(t, c, "club-alice")
	token := &corev1.Secret{Data: map[string][]byte{"RENOVATE_TOKEN": []byte("ghs_token")}}
	token.Name = github.GetNameForGithubAppSecret(job)
	token.Namespace = testSetNamespace
	token.Annotations = map[string]string{api.TokenExpiresAtAnnotationKey: time.Now().Add(time.Hour).Format(time.RFC3339)}
	if err := controllerutil.SetControllerReference(job, token, c.Scheme()); err != nil {
		t.Fatalf("failed to own token secret: %v", err)
	}
	if err := c.Create(context.Background(), token); err != nil {
		t.Fatalf("failed to create token secret: %v", err)
	}

	unlabelSecret(t, c, source)
	reconcileSet(t, r, "club")

	getJob(t, c, "club-alice")
	secret := getSecret(t, c, github.GetNameForGithubAppCredentialsSecret("club-alice"))
	if len(secret.Data) != 0 {
		t.Errorf("credentials must be revoked once the source is refused, got keys %v", secret.Data)
	}
	if _, ok := secret.Labels[api.LabelAllowRef]; ok {
		t.Error("revoked credentials must no longer be opted in")
	}
	token = getSecret(t, c, token.Name)
	if len(token.Data) != 0 {
		t.Errorf("the minted token must be revoked once the source is refused, got keys %v", token.Data)
	}
	if _, ok := token.Annotations[api.TokenExpiresAtAnnotationKey]; ok {
		t.Error("a revoked token must lose its expiry, so it is minted anew once credentials are usable")
	}
}

func TestInstallationSetRestoresCredentialsWhenPolicyAllowsUnchangedSource(t *testing.T) {
	set := newInstallationSet("club")
	c := installationSetClient(t, set, appCredentialsSecret(false))
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "alice", AccountType: api.GitHubAccountTypeUser},
	}}
	r := installationSetReconciler(c, lister)
	reconcileSet(t, r, "club")

	r.Policy = policy.Policy{AllowedHosts: []string{"api.github.com"}}
	reconcileSet(t, r, "club")
	if len(getSecret(t, c, github.GetNameForGithubAppCredentialsSecret("club-alice")).Data) != 0 {
		t.Fatal("test setup: credentials must be revoked while the policy refuses the source")
	}

	r.Policy = policy.Policy{Disabled: true}
	reconcileSet(t, r, "club")

	if got := getSecret(t, c, github.GetNameForGithubAppCredentialsSecret("club-alice")); string(got.Data[credentialsPemKey]) != "pem-data" {
		t.Errorf("credentials must be restored once the policy allows the unchanged source, got %v", got.Data)
	}
	if lister.calls != 1 {
		t.Errorf("restoring credentials must not list installations, got %d calls", lister.calls)
	}
}

func TestInstallationSetPropagatesRotatedCredentialsWithoutListing(t *testing.T) {
	set := newInstallationSet("club")
	source := appCredentialsSecret(true)
	c := installationSetClient(t, set, source)
	lister := &fakeInstallationLister{installations: []github.AppInstallation{
		{ID: 1, Account: "alice", AccountType: api.GitHubAccountTypeUser},
	}}
	r := installationSetReconciler(c, lister)
	reconcileSet(t, r, "club")

	if err := c.Get(context.Background(), crclient.ObjectKeyFromObject(source), source); err != nil {
		t.Fatalf("failed to get source secret: %v", err)
	}
	source.Data["PEM"] = []byte("rotated-pem")
	if err := c.Update(context.Background(), source); err != nil {
		t.Fatalf("failed to rotate source secret: %v", err)
	}
	res := reconcileSet(t, r, "club")

	if got := getSecret(t, c, github.GetNameForGithubAppCredentialsSecret("club-alice")); string(got.Data[credentialsPemKey]) != "rotated-pem" {
		t.Errorf("rotated credentials must reach the generated secret, got %q", got.Data[credentialsPemKey])
	}
	if lister.calls != 1 {
		t.Errorf("propagating credentials must not list installations, got %d calls", lister.calls)
	}
	if res.RequeueAfter > credentialsCheckInterval {
		t.Errorf("expected the source to be checked again within %s, got %s", credentialsCheckInterval, res.RequeueAfter)
	}
}

func getSecret(t *testing.T, c crclient.Client, name string) *corev1.Secret {
	t.Helper()
	secret := &corev1.Secret{}
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: name, Namespace: testSetNamespace}, secret); err != nil {
		t.Fatalf("failed to get secret %s: %v", name, err)
	}
	return secret
}

func unlabelSecret(t *testing.T, c crclient.Client, secret *corev1.Secret) {
	t.Helper()
	if err := c.Get(context.Background(), crclient.ObjectKeyFromObject(secret), secret); err != nil {
		t.Fatalf("failed to get secret: %v", err)
	}
	delete(secret.Labels, api.LabelAllowRef)
	if err := c.Update(context.Background(), secret); err != nil {
		t.Fatalf("failed to unlabel secret: %v", err)
	}
}

func TestInstallationSetRefusesEndpointOutsidePolicy(t *testing.T) {
	set := newInstallationSet("club")
	set.Spec.Provider = &api.GitHubAppProvider{Endpoint: "https://evil.example.com"}
	c := installationSetClient(t, set, appCredentialsSecret(true))
	lister := &fakeInstallationLister{}
	r := installationSetReconciler(c, lister)
	r.Policy = policy.Policy{AllowedHosts: []string{"api.github.com"}}

	reconcileSet(t, r, "club")

	if lister.calls != 0 {
		t.Error("App credentials must not be sent to a host outside the policy")
	}
	cond := meta.FindStatusCondition(getSet(t, c, "club").Status.Conditions, api.ConditionReady)
	if cond == nil || cond.Reason != policy.ReasonDestinationNotAllowed {
		t.Errorf("expected Ready=False/%s, got %#v", policy.ReasonDestinationNotAllowed, cond)
	}
}

func TestInstallationMatches(t *testing.T) {
	alice := github.AppInstallation{ID: 1, Account: "Alice", AccountType: api.GitHubAccountTypeUser}
	club := github.AppInstallation{ID: 2, Account: "club", AccountType: api.GitHubAccountTypeOrganization}

	tests := []struct {
		name   string
		filter *api.GitHubAppInstallationFilter
		want   []bool
	}{
		{"no filter matches all", nil, []bool{true, true}},
		{"include is case-insensitive", &api.GitHubAppInstallationFilter{Include: []string{"alice"}}, []bool{true, false}},
		{"exclude wins over include", &api.GitHubAppInstallationFilter{Include: []string{"alice", "club"}, Exclude: []string{"ALICE"}}, []bool{false, true}},
		{"account types", &api.GitHubAppInstallationFilter{AccountTypes: []string{api.GitHubAccountTypeOrganization}}, []bool{false, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, inst := range []github.AppInstallation{alice, club} {
				if got := installationMatches(inst, tt.filter); got != tt.want[i] {
					t.Errorf("installationMatches(%s) = %v, want %v", inst.Account, got, tt.want[i])
				}
			}
		})
	}
}

func TestNeedsInstallationSync(t *testing.T) {
	now := time.Now()
	synced := newInstallationSet("club")
	synced.Status.ObservedGeneration = 1
	synced.Status.LastSyncTime = &metav1.Time{Time: now.Add(-time.Minute)}

	if needsInstallationSync(synced, 10*time.Minute, now) {
		t.Error("a fresh sync of the current generation must not resync")
	}
	if !needsInstallationSync(synced, 10*time.Minute, now.Add(10*time.Minute)) {
		t.Error("an expired sync must resync")
	}
	synced.Generation = 2
	if !needsInstallationSync(synced, 10*time.Minute, now) {
		t.Error("a spec change must resync")
	}
}

func setAnnotationForTest(t *testing.T, c crclient.Client, name, key, value string) {
	t.Helper()
	set := getSet(t, c, name)
	if set.Annotations == nil {
		set.Annotations = map[string]string{}
	}
	set.Annotations[key] = value
	if err := c.Update(context.Background(), set); err != nil {
		t.Fatalf("failed to annotate set: %v", err)
	}
}
