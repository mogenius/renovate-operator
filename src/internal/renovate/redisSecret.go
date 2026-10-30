package renovate

import (
	"context"
	"errors"
	"fmt"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/kvstore"
	"renovate-operator/internal/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getRenovateCacheURL() string {
	cfg := kvstore.ConfigFromEnv(config.GetValue)
	return cfg.URLForUsage(kvstore.UsageRenovateCache)
}

func redisCacheForwardingEnabled() bool {
	return config.GetValue("VALKEY_FORWARD_CACHE_TO_JOBS") == "true" && getRenovateCacheURL() != ""
}

// newRedisURLSecretName returns the name the Job spec references for the forwarded
// Valkey cache URL, or "" when forwarding is disabled or no cache is configured.
// The name is chosen client-side because the Job must reference the secret before
// the secret can exist.
func newRedisURLSecretName() string {
	if !redisCacheForwardingEnabled() {
		return ""
	}
	return "redis-forward-secret-" + utilrand.String(10)
}

func newRedisURLSecret(name, renovateJobName, project, jobType, valkeyURL string, job *batchv1.Job) *corev1.Secret {
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
		Name:      name,
		Namespace: job.Namespace,
		Labels:    labels,
		// Plain owner reference: blockOwnerDeletion would require jobs/finalizers RBAC.
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: batchv1.SchemeGroupVersion.String(),
			Kind:       "Job",
			Name:       job.Name,
			UID:        job.UID,
		}},
		Data: map[string][]byte{
			"redis-url": []byte(valkeyURL),
		},
	}
}

// createRedisURLSecret creates the named secret holding the forwarded Valkey cache
// URL, owned by job so GC deletes it together with the Job. The owner reference is
// set at creation because OwnerReferencesPermissionEnforcement (OpenShift) demands
// delete permission on secrets for adding one later. On failure the Job is deleted,
// as its pod cannot start without the secret. No-op when name is "".
func createRedisURLSecret(ctx context.Context, c client.Client, name, renovateJobName, project, jobType string, job *batchv1.Job) error {
	if name == "" {
		return nil
	}
	secret := newRedisURLSecret(name, renovateJobName, project, jobType, getRenovateCacheURL(), job)
	if err := c.Create(ctx, secret); err != nil {
		err = fmt.Errorf("creating redis url secret: %w", err)
		// Detached from ctx: the Create may have failed because ctx was cancelled.
		if delErr := crdManager.DeleteJob(context.WithoutCancel(ctx), c, job); delErr != nil {
			return errors.Join(err, delErr)
		}
		return err
	}
	return nil
}
