package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/policy"
	"renovate-operator/metricStore"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const githubAppProvider = "github_app"

// GitHub App events that change which installations or repositories exist.
const (
	githubEventInstallation             = "installation"
	githubEventInstallationRepositories = "installation_repositories"
)

// WithInstallationSets enables GitHub App installation events on the github
// endpoint. Without it those events are ignored.
func (s *Server) WithInstallationSets(c client.Client, p policy.Policy) *Server {
	s.installationSets = c
	s.policy = p
	return s
}

// githubInstallationWebhook resyncs every GitHubAppInstallationSet whose webhook
// secret verifies the request. Several sets may share one App, each with its own
// filter, so every set that verifies is resynced, not just the first. An
// installation no set verifies may still widen the autodiscovery of other
// RenovateJobs, so it falls through to discovery.
func (s *Server) githubInstallationWebhook(w http.ResponseWriter, r *http.Request, body []byte, event string) {
	ctx := r.Context()

	var payload GitHubInstallationEvent
	if err := json.Unmarshal(body, &payload); err != nil {
		metricStore.IncWebhookPayloadDecodeFailure(ctx, githubAppProvider)
		metricStore.IncWebhookRequest(ctx, githubAppProvider, "rejected")
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to decode payload"})
		return
	}

	candidates, matched, err := s.resyncInstallationSets(ctx, r, body, int64(payload.Installation.ID), event)
	if err != nil {
		s.logger.Error(err, "failed to list GitHubAppInstallationSets")
		metricStore.IncWebhookRequest(ctx, githubAppProvider, "rejected")
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if matched > 0 {
		s.logger.Info("received github app installation event", "event", event, "action", payload.Action, "installationId", payload.Installation.ID, "sets", matched)
		metricStore.IncWebhookRequest(ctx, githubAppProvider, "accepted")
		s.writeJSON(w, http.StatusAccepted, map[string]string{"message": "resync triggered"})
		return
	}

	if event == githubEventInstallation && s.discovery != nil {
		s.githubInstallationEvent(ctx, w, r, body)
		return
	}

	if s.installationSets == nil {
		metricStore.IncWebhookRequest(ctx, githubAppProvider, "ignored")
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored", "reason": "installation events are not enabled"})
		return
	}
	if candidates == 0 {
		metricStore.IncWebhookAuthFailure(ctx, githubAppProvider, "no_matching_job")
	} else {
		metricStore.IncWebhookAuthFailure(ctx, githubAppProvider, "auth_failed")
		metricStore.IncWebhookSignatureFailure(ctx, githubAppProvider)
	}
	metricStore.IncWebhookRequest(ctx, githubAppProvider, "rejected")
	s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
}

// resyncInstallationSets returns how many sets accept installation events and
// how many of them verified the request and were resynced.
func (s *Server) resyncInstallationSets(ctx context.Context, r *http.Request, body []byte, installationID int64, event string) (candidates, matched int, err error) {
	if s.installationSets == nil {
		return 0, 0, nil
	}

	var sets api.GitHubAppInstallationSetList
	var listOpts []client.ListOption
	if namespace := r.URL.Query().Get("namespace"); namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}
	if err := s.installationSets.List(ctx, &sets, listOpts...); err != nil {
		return 0, 0, err
	}

	signature := r.Header.Get("X-Hub-Signature-256")
	for i := range sets.Items {
		set := &sets.Items[i]
		if set.Spec.Webhook == nil || !set.Spec.Webhook.Enabled {
			continue
		}
		candidates++
		if signature == "" || !s.installationSetSignatureValid(ctx, set, signature, body) {
			continue
		}
		matched++
		if err := s.triggerInstallationSetResync(ctx, set, installationID, event); err != nil {
			s.logger.Error(err, "failed to trigger GitHubAppInstallationSet resync", "set", set.Name, "namespace", set.Namespace)
		}
	}
	return candidates, matched, nil
}

func (s *Server) installationSetSignatureValid(ctx context.Context, set *api.GitHubAppInstallationSet, signature string, body []byte) bool {
	webhook := set.Spec.Webhook
	if webhook == nil || !webhook.Enabled || webhook.SecretRef == nil {
		return false
	}

	secret := &corev1.Secret{}
	if err := s.installationSets.Get(ctx, client.ObjectKey{Name: webhook.SecretRef.Name, Namespace: set.Namespace}, secret); err != nil {
		metricStore.IncSecretResolutionError(ctx, "api_error")
		s.logger.Error(err, "failed to get webhook secret of GitHubAppInstallationSet", "set", set.Name, "namespace", set.Namespace)
		return false
	}
	if err := s.policy.ValidateReferencedSecret(secret); err != nil {
		metricStore.IncPolicyDenial(ctx, "secret_ref")
		s.logger.Info("webhook secret of GitHubAppInstallationSet refused by policy", "set", set.Name, "namespace", set.Namespace, "error", err.Error())
		return false
	}
	key, ok := secret.Data[webhook.SecretRef.Key]
	if !ok {
		metricStore.IncSecretResolutionError(ctx, "key_missing")
		return false
	}

	mac := hmac.New(sha256.New, key)
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

// triggerInstallationSetResync marks the set for an immediate resync. For
// repositories added to or removed from an installation, the installation's
// RenovateJob is also marked for discovery, since its installation is
// unchanged and a resync alone would not refresh its project list.
func (s *Server) triggerInstallationSetResync(ctx context.Context, set *api.GitHubAppInstallationSet, installationID int64, event string) error {
	if err := setAnnotation(ctx, s.installationSets, set, api.TriggerResyncAnnotationKey, "true"); err != nil {
		return err
	}
	if event != githubEventInstallationRepositories || installationID == 0 {
		return nil
	}

	var jobs api.RenovateJobList
	if err := s.installationSets.List(ctx, &jobs,
		client.InNamespace(set.Namespace),
		client.MatchingLabels{
			api.LabelInstallationSet: set.Name,
			api.LabelInstallationID:  strconv.FormatInt(installationID, 10),
		},
	); err != nil {
		return fmt.Errorf("listing RenovateJobs of installation %d: %w", installationID, err)
	}
	for i := range jobs.Items {
		if err := setAnnotation(ctx, s.installationSets, &jobs.Items[i], api.TriggerDiscoveryAnnotationKey, "true"); err != nil {
			return err
		}
	}
	return nil
}

func setAnnotation(ctx context.Context, c client.Client, obj client.Object, key, value string) error {
	patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
	if err := c.Patch(ctx, obj, patch); err != nil {
		return fmt.Errorf("setting annotation %s on %s: %w", key, obj.GetName(), err)
	}
	return nil
}
