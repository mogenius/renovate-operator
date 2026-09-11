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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getRenovateCacheURL() string {
	cfg := kvstore.ConfigFromEnv(config.GetValue)
	return cfg.URLForUsage(kvstore.UsageRenovateCache)
}

func redisCacheForwardingEnabled() bool {
	return config.GetValue("VALKEY_FORWARD_CACHE_TO_JOBS") == "true" && getRenovateCacheURL() != ""
}

func newRedisURLSecret(namespace, renovateJobName, project, jobType, valkeyURL string) *corev1.Secret {
	labels := map[string]string{
		api.LabelAppManagedBy: api.LabelValueManagedBy,
		api.LabelAppComponent: api.LabelValueComponentValkeyCache,
		api.LabelRenovateJob:  renovateJobName,
		api.LabelJobType:      jobType,
	}
	if project != "" {
		labels[api.LabelProject] = utils.KubernetesCompatibleProjectName(project)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "redis-forward-secret-",
			Namespace:    namespace,
			Labels:       labels,
		},
		Data: map[string][]byte{
			"redis-url": []byte(valkeyURL),
		},
	}
}

// createRedisURLSecret creates a new per-dispatch secret holding the forwarded Valkey
// cache URL. Returns nil, nil when forwarding is disabled or no cache is configured.
// The caller passes the returned secret's Name to the Job spec (SecretKeyRef).
func createRedisURLSecret(ctx context.Context, c client.Client, namespace, renovateJobName, project, jobType string) (*corev1.Secret, error) {
	if !redisCacheForwardingEnabled() {
		return nil, nil
	}
	secret := newRedisURLSecret(namespace, renovateJobName, project, jobType, getRenovateCacheURL())
	if err := c.Create(ctx, secret); err != nil {
		return nil, fmt.Errorf("creating redis url secret: %w", err)
	}
	return secret, nil
}

// ownRedisURLSecretsByLabel finds all redis forwarding secrets for the given
// RenovateJob and project by label selector and sets the k8s Job as their owner,
// so GC deletes them together with the Job (TTL cleanup or explicit deletion).
func ownRedisURLSecretsByLabel(ctx context.Context, c client.Client, namespace, renovateJobName, project, jobType string, job *batchv1.Job) error {
	if !redisCacheForwardingEnabled() {
		return nil
	}

	matchLabels := client.MatchingLabels{
		api.LabelRenovateJob:  renovateJobName,
		api.LabelJobType:      jobType,
		api.LabelAppComponent: api.LabelValueComponentValkeyCache,
	}
	if project != "" {
		matchLabels[api.LabelProject] = utils.KubernetesCompatibleProjectName(project)
	}

	secretList := &corev1.SecretList{}
	if err := c.List(ctx, secretList, client.InNamespace(namespace), matchLabels); err != nil {
		return fmt.Errorf("listing redis url secrets: %w", err)
	}

	ownerRef := metav1.OwnerReference{
		APIVersion: batchv1.SchemeGroupVersion.String(),
		Kind:       "Job",
		Name:       job.Name,
		UID:        job.UID,
	}

	for i := range secretList.Items {
		secret := &secretList.Items[i]
		secret.OwnerReferences = []metav1.OwnerReference{ownerRef}
		if err := c.Update(ctx, secret); err != nil {
			return fmt.Errorf("owning redis url secret %s: %w", secret.Name, err)
		}
	}
	return nil
}
