package renovate

import (
	context "context"
	"fmt"

	"renovate-operator/config"
	"renovate-operator/internal/kvstore"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const redisURLSecretName = "renovate-operator-job-redis-cache"

func getRenovateCacheURL() string {
	cfg := kvstore.ValkeyConfig{
		URL:      config.GetValue("VALKEY_URL"),
		Host:     config.GetValue("VALKEY_HOST"),
		Port:     config.GetValue("VALKEY_PORT"),
		Password: config.GetValue("VALKEY_PASSWORD"),
	}
	return cfg.URLForUsage(kvstore.UsageRenovateCache)
}

func ensureRedisURLSecret(ctx context.Context, c client.Client, namespace string) error {
	valkeyURL := getRenovateCacheURL()

	if valkeyURL == "" {
		return nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisURLSecretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "renovate-operator",
				"app.kubernetes.io/component":  "renovate-valkey-cache",
			},
		},
		Data: map[string][]byte{
			"redis-url": []byte(valkeyURL),
		},
	}

	existing := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKey{Name: redisURLSecretName, Namespace: namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := c.Create(ctx, secret); err != nil {
				return fmt.Errorf("creating redis url secret: %w", err)
			}
			return nil
		}
		return fmt.Errorf("reading redis url secret: %w", err)
	}

	secret.ResourceVersion = existing.ResourceVersion
	if err := c.Update(ctx, secret); err != nil {
		return fmt.Errorf("updating redis url secret: %w", err)
	}
	return nil
}
