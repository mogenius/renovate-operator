package renovate

import (
	context "context"
	"fmt"
	"net/url"

	"renovate-operator/config"
	"renovate-operator/internal/kvstore"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const redisURLSecretName = "renovate-redis-url"

func getValkeyURL() string {
	valkeyURL := config.GetValue("VALKEY_URL")
	if valkeyURL == "" {
		valkeyURL = kvstore.BuildValkeyURL(
			config.GetValue("VALKEY_HOST"),
			config.GetValue("VALKEY_PORT"),
			config.GetValue("VALKEY_PASSWORD"),
		)
	}
	return valkeyURL
}

func getRenovateCacheURL() (string, error) {
	baseURL := getValkeyURL()
	if baseURL == "" {
		return "", nil
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parsing valkey url: %w", err)
	}
	parsed.Path = "/1"
	return parsed.String(), nil
}

func ensureRedisURLSecret(ctx context.Context, c client.Client, namespace string) error {
	valkeyURL, err := getRenovateCacheURL()
	if err != nil {
		return err
	}
	if valkeyURL == "" {
		return nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisURLSecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"redis-url": []byte(valkeyURL),
		},
	}

	existing := &corev1.Secret{}
	err = c.Get(ctx, client.ObjectKey{Name: redisURLSecretName, Namespace: namespace}, existing)
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
