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
	"strings"
	"time"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/policy"
	"renovate-operator/metricStore"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// githubAppProvider labels the metrics of installation events once they reach
// the GitHubAppInstallationSets. Events no set verifies keep the github label.
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
// installation may also widen the autodiscovery of other RenovateJobs, so an
// installation event goes to discovery whether or not a set verified it.
func (s *Server) githubInstallationWebhook(w http.ResponseWriter, r *http.Request, body []byte, event string) {
	ctx := r.Context()
	const provider = "github"

	var payload GitHubInstallationEvent
	if err := json.Unmarshal(body, &payload); err != nil {
		metricStore.IncWebhookPayloadDecodeFailure(ctx, provider)
		metricStore.IncWebhookRequest(ctx, provider, "rejected")
		s.logger.Error(err, "failed to decode github installation webhook payload. Not processing.")
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

	if matched == 0 {
		// The event may be meant for another App sharing this URL, so it is
		// handled as it would be without installation sets, not rejected.
		s.logger.V(1).Info("no GitHubAppInstallationSet verified the installation event", "event", event, "candidates", candidates)
		if event == githubEventInstallation && s.discovery != nil {
			s.githubInstallationEvent(ctx, w, r, body)
			return
		}
		metricStore.IncWebhookRequest(ctx, provider, "ignored")
		s.writeJSON(w, http.StatusOK, map[string]string{"message": "event ignored", "reason": "no GitHubAppInstallationSet verified the event"})
		return
	}

	s.logger.Info("received github app installation event", "event", event, "action", payload.Action, "installationId", payload.Installation.ID, "sets", matched)
	response := map[string]any{"message": "resync triggered", "sets": matched}
	if event == githubEventInstallation {
		if started := s.discoverInstallation(ctx, r, body, payload); len(started) > 0 {
			response["renovateJobs"] = started
		}
	}
	metricStore.IncWebhookRequest(ctx, githubAppProvider, "accepted")
	s.writeJSON(w, http.StatusAccepted, response)
}

// discoverInstallation starts discovery for the RenovateJobs whose autodiscovery
// a created installation widens, as githubInstallationEvent does. The request
// is answered for the sets that verified it, so discovery's outcome is only
// returned as the jobs it started.
func (s *Server) discoverInstallation(ctx context.Context, r *http.Request, body []byte, payload GitHubInstallationEvent) []string {
	if s.discovery == nil || payload.Action != "created" {
		return nil
	}
	repositories := installationRepositories(payload)
	if len(repositories) == 0 {
		return nil
	}
	outcome, err := s.startDiscoveries(ctx, r, body, repositories)
	if err != nil {
		s.logger.Error(err, "failed to list RenovateJobs for installation event")
		return nil
	}
	return outcome.startedJobs
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

	// A malformed signature verifies against no set, so it costs no Secret
	// read. Sets sharing a webhook Secret read it once per request.
	signature, signatureOK := decodeHubSignature(r.Header.Get("X-Hub-Signature-256"))
	secrets := map[client.ObjectKey]*corev1.Secret{}
	for i := range sets.Items {
		set := &sets.Items[i]
		if set.Spec.Webhook == nil || !set.Spec.Webhook.Enabled {
			continue
		}
		candidates++
		if !signatureOK || !s.installationSetSignatureValid(ctx, set, signature, body, secrets) {
			continue
		}
		matched++
		if err := s.triggerInstallationSetResync(ctx, set, installationID, event); err != nil {
			s.logger.Error(err, "failed to trigger GitHubAppInstallationSet resync", "set", set.Name, "namespace", set.Namespace)
		}
	}
	return candidates, matched, nil
}

// decodeHubSignature returns the HMAC of an X-Hub-Signature-256 header value,
// or false when the value is not a sha256 signature.
func decodeHubSignature(header string) ([]byte, bool) {
	hexMAC, ok := strings.CutPrefix(header, "sha256=")
	if !ok || len(hexMAC) != hex.EncodedLen(sha256.Size) {
		return nil, false
	}
	mac, err := hex.DecodeString(hexMAC)
	if err != nil {
		return nil, false
	}
	return mac, true
}

// installationSetSignatureValid verifies signature against the set's webhook
// secret. secrets caches the Secrets read for this request; nil marks one that
// could not be read or is refused by policy.
func (s *Server) installationSetSignatureValid(ctx context.Context, set *api.GitHubAppInstallationSet, signature, body []byte, secrets map[client.ObjectKey]*corev1.Secret) bool {
	webhook := set.Spec.Webhook
	if webhook.SecretRef == nil {
		return false
	}

	ref := client.ObjectKey{Name: webhook.SecretRef.Name, Namespace: set.Namespace}
	secret, cached := secrets[ref]
	if !cached {
		secret = s.installationSetWebhookSecret(ctx, set, ref)
		secrets[ref] = secret
	}
	if secret == nil {
		return false
	}
	key, ok := secret.Data[webhook.SecretRef.Key]
	if !ok {
		metricStore.IncSecretResolutionError(ctx, "key_missing")
		return false
	}

	mac := hmac.New(sha256.New, key)
	mac.Write(body)
	return hmac.Equal(signature, mac.Sum(nil))
}

func (s *Server) installationSetWebhookSecret(ctx context.Context, set *api.GitHubAppInstallationSet, ref client.ObjectKey) *corev1.Secret {
	secret := &corev1.Secret{}
	if err := s.installationSets.Get(ctx, ref, secret); err != nil {
		metricStore.IncSecretResolutionError(ctx, "api_error")
		s.logger.Error(err, "failed to get webhook secret of GitHubAppInstallationSet", "set", set.Name, "namespace", set.Namespace)
		return nil
	}
	if err := s.policy.ValidateReferencedSecret(secret); err != nil {
		metricStore.IncPolicyDenial(ctx, "secret_ref")
		s.logger.Info("webhook secret of GitHubAppInstallationSet refused by policy", "set", set.Name, "namespace", set.Namespace, "error", err.Error())
		return nil
	}
	return secret
}

// triggerInstallationSetResync marks the set for an immediate resync. The
// annotation gets a fresh value on every event, so the reconciler can tell a
// request that arrived during its sync from the one it served. For
// repositories added to or removed from an installation, the installation's
// RenovateJob is also marked for discovery, since its installation is
// unchanged and a resync alone would not refresh its project list.
func (s *Server) triggerInstallationSetResync(ctx context.Context, set *api.GitHubAppInstallationSet, installationID int64, event string) error {
	if err := setAnnotation(ctx, s.installationSets, set, api.TriggerResyncAnnotationKey, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
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
