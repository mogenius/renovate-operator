package renovate

import (
	"context"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/types"
	"renovate-operator/internal/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func valkeyForwardConfig(t *testing.T) {
	t.Helper()
	if err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "VALKEY_URL", Optional: true, Default: "redis://redis.svc.cluster.local:6379/0"},
		{Key: "VALKEY_HOST", Optional: true, Default: ""},
		{Key: "VALKEY_PORT", Optional: true, Default: "6379"},
		{Key: "VALKEY_PASSWORD", Optional: true, Default: ""},
		{Key: "VALKEY_FORWARD_CACHE_TO_JOBS", Optional: true, Default: "true"},
	}); err != nil {
		t.Fatalf("failed to init config: %v", err)
	}
}

func getSecret(t *testing.T, c client.Client, namespace, name string) *corev1.Secret {
	t.Helper()
	secret := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, secret); err != nil {
		t.Fatalf("expected secret %s/%s: %v", namespace, name, err)
	}
	return secret
}

func listSecrets(t *testing.T, c client.Client) []corev1.Secret {
	t.Helper()
	list := &corev1.SecretList{}
	if err := c.List(context.Background(), list); err != nil {
		t.Fatalf("failed to list secrets: %v", err)
	}
	return list.Items
}

func TestEnsureRedisURLSecretCreatesPerJobSecret(t *testing.T) {
	valkeyForwardConfig(t)
	scheme := policyScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	if err := ensureRedisURLSecret(context.Background(), c, "ns", "rj-proj-701b9b0a-redis"); err != nil {
		t.Fatalf("ensureRedisURLSecret returned error: %v", err)
	}

	stored := getSecret(t, c, "ns", "rj-proj-701b9b0a-redis")
	if got := string(stored.Data["redis-url"]); got != "redis://redis.svc.cluster.local:6379/1" {
		t.Errorf("expected redis-url with cache db offset, got %q", got)
	}
	if got := stored.Labels[api.LabelAppManagedBy]; got != api.LabelValueManagedBy {
		t.Errorf("expected managed-by label %q, got %q", api.LabelValueManagedBy, got)
	}
	if got := stored.Labels[api.LabelAppComponent]; got != api.LabelValueComponentValkeyCache {
		t.Errorf("expected component label %q, got %q", api.LabelValueComponentValkeyCache, got)
	}
}

func TestEnsureRedisURLSecretUpdatesRotatedURL(t *testing.T) {
	valkeyForwardConfig(t)
	scheme := policyScheme(t)
	stale := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "rj-proj-701b9b0a-redis", Namespace: "ns"},
		Data:       map[string][]byte{"redis-url": []byte("redis://:old-password@redis.svc.cluster.local:6379/1")},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stale).Build()

	if err := ensureRedisURLSecret(context.Background(), c, "ns", "rj-proj-701b9b0a-redis"); err != nil {
		t.Fatalf("ensureRedisURLSecret returned error: %v", err)
	}

	stored := getSecret(t, c, "ns", "rj-proj-701b9b0a-redis")
	if got := string(stored.Data["redis-url"]); got != "redis://redis.svc.cluster.local:6379/1" {
		t.Errorf("expected rotated redis-url, got %q", got)
	}
}

func TestEnsureRedisURLSecretSkippedWithoutForwardFlag(t *testing.T) {
	if err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "VALKEY_URL", Optional: true, Default: "redis://redis.svc.cluster.local:6379/0"},
		{Key: "VALKEY_FORWARD_CACHE_TO_JOBS", Optional: true, Default: "false"},
	}); err != nil {
		t.Fatalf("failed to init config: %v", err)
	}
	scheme := policyScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	if err := ensureRedisURLSecret(context.Background(), c, "ns", "rj-proj-701b9b0a-redis"); err != nil {
		t.Fatalf("ensureRedisURLSecret returned error: %v", err)
	}
	if secrets := listSecrets(t, c); len(secrets) != 0 {
		t.Errorf("expected no secrets created, got %d", len(secrets))
	}
}

func TestEnsureRedisURLSecretSkippedWithoutValkey(t *testing.T) {
	if err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "VALKEY_URL", Optional: true, Default: ""},
		{Key: "VALKEY_HOST", Optional: true, Default: ""},
		{Key: "VALKEY_FORWARD_CACHE_TO_JOBS", Optional: true, Default: "true"},
	}); err != nil {
		t.Fatalf("failed to init config: %v", err)
	}
	scheme := policyScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	if err := ensureRedisURLSecret(context.Background(), c, "ns", "rj-proj-701b9b0a-redis"); err != nil {
		t.Fatalf("ensureRedisURLSecret returned error: %v", err)
	}
	if secrets := listSecrets(t, c); len(secrets) != 0 {
		t.Errorf("expected no secrets created, got %d", len(secrets))
	}
}

func TestOwnRedisURLSecret(t *testing.T) {
	valkeyForwardConfig(t)
	scheme := policyScheme(t)
	stored := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "rj-proj-701b9b0a-redis", Namespace: "ns"},
		Data:       map[string][]byte{"redis-url": []byte("redis://redis.svc.cluster.local:6379/1")},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stored).Build()

	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "rj-proj-701b9b0a-abcde", Namespace: "ns", UID: "uid-123"}}
	if err := ownRedisURLSecret(context.Background(), c, "ns", "rj-proj-701b9b0a-redis", job); err != nil {
		t.Fatalf("ownRedisURLSecret returned error: %v", err)
	}

	secret := getSecret(t, c, "ns", "rj-proj-701b9b0a-redis")
	if len(secret.OwnerReferences) != 1 {
		t.Fatalf("expected exactly one owner reference, got %+v", secret.OwnerReferences)
	}
	owner := secret.OwnerReferences[0]
	if owner.Kind != "Job" || owner.Name != job.Name || owner.UID != job.UID {
		t.Errorf("expected owner reference to the job, got %+v", owner)
	}
	// A controller reference would set blockOwnerDeletion, which requires
	// jobs/finalizers RBAC the chart deliberately does not grant.
	if owner.Controller != nil && *owner.Controller {
		t.Errorf("expected a plain owner reference, got controller reference")
	}
	if owner.BlockOwnerDeletion != nil && *owner.BlockOwnerDeletion {
		t.Errorf("expected blockOwnerDeletion to be unset")
	}
}

func TestOwnRedisURLSecretRecreatesCollectedSecret(t *testing.T) {
	valkeyForwardConfig(t)
	scheme := policyScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "rj-proj-701b9b0a-abcde", Namespace: "ns", UID: "uid-123"}}
	if err := ownRedisURLSecret(context.Background(), c, "ns", "rj-proj-701b9b0a-redis", job); err != nil {
		t.Fatalf("ownRedisURLSecret returned error: %v", err)
	}

	secret := getSecret(t, c, "ns", "rj-proj-701b9b0a-redis")
	if got := string(secret.Data["redis-url"]); got != "redis://redis.svc.cluster.local:6379/1" {
		t.Errorf("expected redis-url data on recreated secret, got %q", got)
	}
	if len(secret.OwnerReferences) != 1 || secret.OwnerReferences[0].Name != job.Name {
		t.Errorf("expected recreated secret owned by the job, got %+v", secret.OwnerReferences)
	}
}

func TestDispatchScheduledCreatesOwnedRedisSecret(t *testing.T) {
	valkeyForwardConfig(t)
	scheme := policyScheme(t)

	renovateJob := policyJob("allowed", "")
	renovateJob.Spec.Parallelism = 1
	projectCRD := &api.RenovateProject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.RenovateProjectCRDName(renovateJob.Name, "org/b"),
			Namespace: renovateJob.Namespace,
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(projectCRD).Build()

	mgr := &fakeJobManager{
		getProjectsByStatusFn: func(_ context.Context, _ crdManager.RenovateJobIdentifier, status api.RenovateProjectStatus) ([]crdManager.RenovateProjectStatus, error) {
			if status == api.JobStatusScheduled {
				return []crdManager.RenovateProjectStatus{{Name: "org/b", Status: api.JobStatusScheduled}}, nil
			}
			return nil, nil
		},
		updateProjectStatusFn: func(_ context.Context, _ string, _ crdManager.RenovateJobIdentifier, _ *types.RenovateStatusUpdate) error {
			return nil
		},
	}

	e := &renovateExecutor{client: c, scheme: scheme, logger: testLogger, policy: gatePolicy(), manager: mgr}

	if err := e.dispatchScheduled(context.Background(), []api.RenovateJob{renovateJob}, 0, map[string]int{}, executionOptions{}); err != nil {
		t.Fatalf("dispatchScheduled returned error: %v", err)
	}

	jobs := createdJobs(t, c)
	if len(jobs) != 1 {
		t.Fatalf("expected one Kubernetes Job, got %d", len(jobs))
	}

	secretName := executorRedisSecretName(&renovateJob, "org/b")
	secret := getSecret(t, c, renovateJob.Namespace, secretName)
	if got := string(secret.Data["redis-url"]); got != "redis://redis.svc.cluster.local:6379/1" {
		t.Errorf("expected redis-url with cache db offset, got %q", got)
	}
	if len(secret.OwnerReferences) != 1 || secret.OwnerReferences[0].Name != jobs[0].Name {
		t.Errorf("expected secret owned by the created job %s, got %+v", jobs[0].Name, secret.OwnerReferences)
	}
	if secrets := listSecrets(t, c); len(secrets) != 1 {
		t.Errorf("expected exactly one secret (no shared namespace secret), got %d", len(secrets))
	}

	container := expectContainer(t, &jobs[0])
	expectEnvVarFromSecretKey(t, container, "RENOVATE_REDIS_URL", secretName, "redis-url")
}

func TestCreateDiscoveryJobCreatesOwnedRedisSecret(t *testing.T) {
	valkeyForwardConfig(t)
	scheme := policyScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	da := NewDiscoveryAgent(scheme, c, testLogger, nil, nil, gatePolicy())
	renovateJob := policyJob("job1", "")
	if _, err := da.CreateDiscoveryJob(context.Background(), renovateJob, DiscoveryJobOptions{}); err != nil {
		t.Fatalf("CreateDiscoveryJob returned error: %v", err)
	}

	jobs := createdJobs(t, c)
	if len(jobs) != 1 {
		t.Fatalf("expected one Kubernetes Job, got %d", len(jobs))
	}

	secretName := discoveryRedisSecretName(&renovateJob)
	secret := getSecret(t, c, renovateJob.Namespace, secretName)
	if got := string(secret.Data["redis-url"]); got != "redis://redis.svc.cluster.local:6379/1" {
		t.Errorf("expected redis-url with cache db offset, got %q", got)
	}
	if len(secret.OwnerReferences) != 1 || secret.OwnerReferences[0].Name != jobs[0].Name {
		t.Errorf("expected secret owned by the created job %s, got %+v", jobs[0].Name, secret.OwnerReferences)
	}

	container := expectContainer(t, &jobs[0])
	expectEnvVarFromSecretKey(t, container, "RENOVATE_REDIS_URL", secretName, "redis-url")
}
