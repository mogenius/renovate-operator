package renovate

import (
	"context"
	"fmt"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	"renovate-operator/internal/kvstore"
	"renovate-operator/internal/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Deterministic per-Job secret names: a re-dispatch upserts the same object
// instead of piling up orphans while Job creation keeps failing.
func executorRedisSecretName(job *api.RenovateJob, project string) string {
	return utils.ExecutorJobName(job, project) + "-redis"
}

func discoveryRedisSecretName(job *api.RenovateJob) string {
	return utils.DiscoveryJobName(job) + "-redis"
}

func getRenovateCacheURL() string {
	cfg := kvstore.ConfigFromEnv(config.GetValue)
	return cfg.URLForUsage(kvstore.UsageRenovateCache)
}

func redisCacheForwardingEnabled() bool {
	return config.GetValue("VALKEY_FORWARD_CACHE_TO_JOBS") == "true" && getRenovateCacheURL() != ""
}

func newRedisURLSecret(namespace, name, valkeyURL string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				api.LabelAppManagedBy: api.LabelValueManagedBy,
				api.LabelAppComponent: api.LabelValueComponentValkeyCache,
			},
		},
		Data: map[string][]byte{
			"redis-url": []byte(valkeyURL),
		},
	}
}

// ensureRedisURLSecret upserts the per-job secret holding the forwarded Valkey
// cache URL; a no-op when forwarding is disabled or no cache is configured.
// Upserting on every dispatch is what propagates credential rotation.
func ensureRedisURLSecret(ctx context.Context, c client.Client, namespace, name string) error {
	if !redisCacheForwardingEnabled() {
		return nil
	}
	valkeyURL := getRenovateCacheURL()

	existing := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := c.Create(ctx, newRedisURLSecret(namespace, name, valkeyURL)); err != nil {
				return fmt.Errorf("creating redis url secret: %w", err)
			}
			return nil
		}
		return fmt.Errorf("reading redis url secret: %w", err)
	}

	if string(existing.Data["redis-url"]) == valkeyURL {
		return nil
	}

	// Update in place: a fresh literal would drop the owner reference pointing
	// at the Job that currently owns the secret.
	existing.Data = map[string][]byte{"redis-url": []byte(valkeyURL)}
	if err := c.Update(ctx, existing); err != nil {
		return fmt.Errorf("updating redis url secret: %w", err)
	}
	return nil
}

// ownRedisURLSecret makes the created Job the secret's sole owner so garbage
// collection deletes the secret together with the Job (TTL cleanup or explicit
// deletion). A plain owner reference, not controllerutil.SetControllerReference:
// blockOwnerDeletion would require update rights on jobs/finalizers, which the
// chart deliberately does not grant.
func ownRedisURLSecret(ctx context.Context, c client.Client, namespace, name string, job *batchv1.Job) error {
	if !redisCacheForwardingEnabled() {
		return nil
	}

	ownerRef := metav1.OwnerReference{
		APIVersion: batchv1.SchemeGroupVersion.String(),
		Kind:       "Job",
		Name:       job.Name,
		UID:        job.UID,
	}

	secret := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, secret)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("reading redis url secret: %w", err)
		}
		// Garbage collection can win this race when a previous generation's Job
		// still owned the secret. Recreate it already owned, or the new Job's
		// pods cannot start.
		secret = newRedisURLSecret(namespace, name, getRenovateCacheURL())
		secret.OwnerReferences = []metav1.OwnerReference{ownerRef}
		if err := c.Create(ctx, secret); err != nil {
			return fmt.Errorf("recreating redis url secret: %w", err)
		}
		return nil
	}

	secret.OwnerReferences = []metav1.OwnerReference{ownerRef}
	if err := c.Update(ctx, secret); err != nil {
		return fmt.Errorf("owning redis url secret: %w", err)
	}
	return nil
}
