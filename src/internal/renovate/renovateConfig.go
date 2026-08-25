package renovate

import (
	context "context"
	"crypto/sha256"
	"fmt"

	api "renovate-operator/api/v1alpha1"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/utils"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	renovateConfigVolumeName = "renovate-config-file"
	renovateConfigMountPath  = "/etc/renovate"
	defaultConfigFileName    = "config.js"
)

// name of the operator-owned ConfigMap holding a job's inline renovate config
func renovateConfigMapName(job *api.RenovateJob) string {
	name := utils.KubernetesCompatibleName(job.Name)
	hash := sha256.Sum256([]byte(name))
	hashStr := fmt.Sprintf("%x", hash[:4])

	if len(name) > 38 {
		name = name[:38]
	}
	return fmt.Sprintf("%s-renovate-config-%s", name, hashStr)
}

// file name the config is mounted as; for configMapRef the key is the file name
func renovateConfigFileName(cfg *api.RenovateJobConfig) string {
	if cfg.ConfigMapRef != nil {
		return cfg.ConfigMapRef.Key
	}
	if cfg.FileName != "" {
		return cfg.FileName
	}
	return defaultConfigFileName
}

// inlineConfigHash returns a short hash covering the filename and content of an
// inline renovate config, used to detect when the ConfigMap needs to be updated.
func inlineConfigHash(cfg *api.RenovateJobConfig) string {
	h := sha256.Sum256([]byte(renovateConfigFileName(cfg) + "\x00" + cfg.Inline))
	return fmt.Sprintf("%x", h[:8])
}

// EnsureRenovateConfigMap syncs spec.renovateConfig.inline into a ConfigMap owned
// by the RenovateJob, and deletes it when inline config is no longer used.
// The annotation value is the hash of the inline config; a matching hash with an
// existing ConfigMap short-circuits the reconcile without any API writes.
func EnsureRenovateConfigMap(ctx context.Context, c client.Client, job *api.RenovateJob) error {
	name := renovateConfigMapName(job)

	if job.Spec.RenovateConfig == nil || job.Spec.RenovateConfig.Inline == "" {
		if job.Annotations[api.RenovateConfigMapAnnotationKey] == "" {
			return nil
		}
		existing := new(corev1.ConfigMap)
		err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: job.Namespace}, existing)
		switch {
		case apierrors.IsNotFound(err):
		case err != nil:
			return fmt.Errorf("reading renovate config configmap: %w", err)
		// only clean up ConfigMaps this job controls; leave foreign objects alone
		case metav1.IsControlledBy(existing, job):
			if err := c.Delete(ctx, existing, client.Preconditions{UID: &existing.UID}); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("deleting stale renovate config configmap: %w", err)
			}
		}
		return crdManager.RemoveAnnotation(ctx, c, job, api.RenovateConfigMapAnnotationKey)
	}

	// Fast path: hash matches → check ConfigMap exists via cache (no API write).
	// If missing (external delete), fall through to recreate.
	hash := inlineConfigHash(job.Spec.RenovateConfig)
	if job.Annotations[api.RenovateConfigMapAnnotationKey] == hash {
		existing := new(corev1.ConfigMap)
		switch err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: job.Namespace}, existing); {
		case err == nil:
			return nil
		case !apierrors.IsNotFound(err):
			return fmt.Errorf("checking renovate config configmap: %w", err)
		}
	}

	configMap := new(corev1.ConfigMap)
	configMap.Name = name
	configMap.Namespace = job.Namespace
	_, err := controllerutil.CreateOrUpdate(ctx, c, configMap, func() error {
		// never adopt a pre-existing ConfigMap this job does not control
		if configMap.ResourceVersion != "" && !metav1.IsControlledBy(configMap, job) {
			return fmt.Errorf("configmap %s/%s already exists and is not owned by this RenovateJob", job.Namespace, name)
		}
		if configMap.Labels == nil {
			configMap.Labels = make(map[string]string)
		}
		configMap.Labels[api.LabelAppManagedBy] = api.LabelValueManagedBy
		configMap.Labels[api.LabelAppComponent] = api.LabelValueComponentRenovateConfig
		configMap.Data = map[string]string{
			renovateConfigFileName(job.Spec.RenovateConfig): job.Spec.RenovateConfig.Inline,
		}
		return controllerutil.SetControllerReference(job, configMap, c.Scheme())
	})
	if err != nil {
		return fmt.Errorf("ensuring renovate config configmap: %w", err)
	}
	return crdManager.AddAnnotation(ctx, c, job, api.RenovateConfigMapAnnotationKey, hash)
}
