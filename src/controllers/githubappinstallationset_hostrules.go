package controllers

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/github"
	"renovate-operator/internal/utils"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// hostRulesRenewBefore is how long before the earliest token expiry the shared
// hostRules are minted anew, the same margin EnsureToken applies to a
// RenovateJob's own token.
const hostRulesRenewBefore = 30 * time.Minute

// hostRulesLifetimeWithoutTokens stands in for the expiry when no installation
// is shared, so an empty selection is not listed again on every reconcile.
const hostRulesLifetimeWithoutTokens = time.Hour

// ensureHostRules converges the set's hostRules Secret. Disabled hostRules
// empty it. Otherwise the tokens of the selected installations are minted
// when the Secret is missing, its earliest token expires within
// hostRulesRenewBefore, or listed selects other installations than the Secret
// holds. listed is the installation listing of this reconcile, nil without a
// sync; the installations are then listed only when the tokens need renewing.
func (r *GitHubAppInstallationSetReconciler) ensureHostRules(ctx context.Context, set *api.GitHubAppInstallationSet, creds *appCredentials, listed []github.AppInstallation) error {
	if !set.Spec.HostRules.IsEnabled() {
		return r.emptyHostRulesSecret(ctx, set)
	}

	name := github.GetNameForGithubAppHostRulesSecret(set.Name)
	existing := &corev1.Secret{}
	err := r.K8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: set.Namespace}, existing)
	switch {
	case k8serrors.IsNotFound(err):
		existing = nil
	case err != nil:
		return fmt.Errorf("getting hostRules secret %s: %w", name, err)
	case !metav1.IsControlledBy(existing, set):
		return fmt.Errorf("hostRules secret %s exists and is not owned by this GitHubAppInstallationSet", name)
	}

	stale := existing == nil || !hostRulesFresh(existing, time.Now())
	_, endpoint := utils.GetPlatformAndEndpoint(set.Spec.RenovateProvider())
	if listed == nil {
		if !stale {
			return nil
		}
		listed, err = r.Installations.ListInstallations(ctx, creds.appID, creds.pem, endpoint)
		if err != nil {
			return fmt.Errorf("listing installations for hostRules: %w", err)
		}
	}

	shared := sharedInstallations(listed, set.Spec.HostRules.Installations)
	selection := sharedInstallationsValue(shared)
	if !stale && existing.Annotations[api.SharedInstallationsAnnotationKey] == selection {
		return nil
	}

	tokens := make([]github.InstallationToken, 0, len(shared))
	expiresAt := time.Now().Add(hostRulesLifetimeWithoutTokens)
	for i, inst := range shared {
		token, exp, err := r.Tokens.MintInstallationToken(creds.appID, strconv.FormatInt(inst.ID, 10), creds.pem, endpoint)
		if err != nil {
			// The previous Secret stays: its tokens may still be valid.
			return fmt.Errorf("minting hostRules token for installation %d (%s): %w", inst.ID, inst.Account, err)
		}
		tokens = append(tokens, github.InstallationToken{Account: inst.Account, Token: token})
		if i == 0 || exp.Before(expiresAt) {
			expiresAt = exp
		}
	}
	config, err := github.BuildHostRulesConfig(endpoint, tokens)
	if err != nil {
		return err
	}

	secret := &corev1.Secret{}
	secret.Name = name
	secret.Namespace = set.Namespace
	_, err = controllerutil.CreateOrUpdate(ctx, r.K8sClient, secret, func() error {
		if secret.Labels == nil {
			secret.Labels = map[string]string{}
		}
		secret.Labels[api.LabelAppManagedBy] = api.LabelValueManagedBy
		secret.Labels[api.LabelAppComponent] = api.LabelValueComponentGitHubAppHostRules
		secret.Labels[api.LabelInstallationSet] = set.Name
		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}
		secret.Annotations[api.TokenExpiresAtAnnotationKey] = expiresAt.UTC().Format(time.RFC3339)
		secret.Annotations[api.SharedInstallationsAnnotationKey] = selection
		secret.Data = map[string][]byte{github.HostRulesConfigFileKey: config}
		return controllerutil.SetControllerReference(set, secret, r.K8sClient.Scheme())
	})
	if err != nil {
		return fmt.Errorf("ensuring hostRules secret %s: %w", name, err)
	}
	log.FromContext(ctx).Info("minted shared hostRules tokens", "secret", name, "installations", len(tokens), "expiresAt", expiresAt)
	return nil
}

// emptyHostRulesSecret drops the shared tokens and the bookkeeping that would
// keep them from being minted anew. The Secret is emptied rather than deleted,
// since the operator may not delete Secrets; it goes with the set through its
// owner reference.
func (r *GitHubAppInstallationSetReconciler) emptyHostRulesSecret(ctx context.Context, set *api.GitHubAppInstallationSet) error {
	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: github.GetNameForGithubAppHostRulesSecret(set.Name), Namespace: set.Namespace}
	if err := r.K8sClient.Get(ctx, key, secret); err != nil {
		return client.IgnoreNotFound(err)
	}
	if !metav1.IsControlledBy(secret, set) {
		return nil
	}
	_, expiring := secret.Annotations[api.TokenExpiresAtAnnotationKey]
	_, selected := secret.Annotations[api.SharedInstallationsAnnotationKey]
	if !expiring && !selected && len(secret.Data) == 0 {
		return nil
	}
	delete(secret.Annotations, api.TokenExpiresAtAnnotationKey)
	delete(secret.Annotations, api.SharedInstallationsAnnotationKey)
	secret.Data = nil
	if err := r.K8sClient.Update(ctx, secret); err != nil {
		return fmt.Errorf("revoking hostRules secret %s: %w", key.Name, err)
	}
	log.FromContext(ctx).Info("revoked shared hostRules", "secret", key.Name)
	return nil
}

// hostRulesFresh reports whether the Secret holds a config whose earliest
// token is valid for longer than hostRulesRenewBefore.
func hostRulesFresh(secret *corev1.Secret, now time.Time) bool {
	if len(secret.Data[github.HostRulesConfigFileKey]) == 0 {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, secret.Annotations[api.TokenExpiresAtAnnotationKey])
	if err != nil {
		return false
	}
	return expiresAt.Sub(now) > hostRulesRenewBefore
}

// sharedInstallations selects the installations whose tokens are shared, from
// every installation of the App. Suspended installations cannot mint tokens.
func sharedInstallations(installations []github.AppInstallation, filter *api.GitHubAppInstallationFilter) []github.AppInstallation {
	var result []github.AppInstallation
	for _, inst := range installations {
		if inst.Suspended || !installationMatches(inst, filter) {
			continue
		}
		result = append(result, inst)
	}
	slices.SortFunc(result, func(a, b github.AppInstallation) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return result
}

// sharedInstallationsValue renders the selection as recorded in
// api.SharedInstallationsAnnotationKey. The login is part of it, since a
// renamed account needs a new matchHost.
func sharedInstallationsValue(installations []github.AppInstallation) string {
	parts := make([]string, 0, len(installations))
	for _, inst := range installations {
		parts = append(parts, strconv.FormatInt(inst.ID, 10)+"="+inst.Account)
	}
	return strings.Join(parts, ",")
}
