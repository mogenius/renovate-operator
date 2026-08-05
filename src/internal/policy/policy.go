/*
Package policy holds the operator's install-wide guard rails for RenovateJob specs.

Rules are resolved once at startup and injected, never read from the config
singleton inside a check, so a call site cannot silently observe different rules
than its neighbour and so the checks stay testable without global state.
*/
package policy

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	"renovate-operator/internal/utils"

	corev1 "k8s.io/api/core/v1"
)

// AllowRefLabel opts a Secret in to being dereferenced by a RenovateJob.
const AllowRefLabel = "renovate-operator.mogenius.com/allow-ref"

// Refusal reasons. These are surfaced verbatim as the reason of the RenovateJob's
// Accepted status condition, so they follow Kubernetes' CamelCase convention and
// are part of the resource's observable API.
const (
	ReasonDestinationNotAllowed    = "DestinationNotAllowed"
	ReasonInvalidDestinationURL    = "InvalidDestinationURL"
	ReasonSecretRefNotOptedIn      = "SecretRefNotOptedIn"
	ReasonServiceAccountNotAllowed = "ServiceAccountNotAllowed"
	ReasonRootUserNotAllowed       = "RootUserNotAllowed"
	ReasonImageNotAllowed          = "ImageNotAllowed"
	ReasonPolicySatisfied          = "PolicySatisfied"
	// ReasonPolicyDisabled marks a job accepted only because enforcement is off, so
	// `kubectl get renovatejobs` shows that the guard rails are not in play.
	ReasonPolicyDisabled = "PolicyDisabled"
)

// Violation is a policy refusal. It carries the reason as a code so callers can
// report it without parsing the message.
type Violation struct {
	Reason  string
	Message string
}

func (v *Violation) Error() string { return v.Message }

// ReasonFor returns the refusal reason carried by err, or "" if err is not a
// policy violation.
func ReasonFor(err error) string {
	var v *Violation
	if errors.As(err, &v) {
		return v.Reason
	}
	return ""
}

func violationf(reason string, format string, args ...any) *Violation {
	return &Violation{Reason: reason, Message: fmt.Sprintf(format, args...)}
}

// Policy is a resolved set of guard rails.
type Policy struct {
	// Disabled turns every check below into a no-op. Phrased as a negative so the zero
	// value enforces: a Policy nobody configured must not be a Policy nobody applies.
	// The chart exposes it as the positive policy.enabled.
	//
	// It does not (cannot) relax the invariants the CRD enforces (hostPath,
	// privileged, allowPrivilegeEscalation): those are rejected by the API server, not
	// by this package.
	Disabled bool
	// AllowedHosts bounds every destination the operator may use.
	AllowedHosts []string
	// AllowUnlabeledSecretRefs drops the AllowRefLabel requirement.
	AllowUnlabeledSecretRefs bool
	// AllowedServiceAccounts are the ServiceAccount names a RenovateJob may run its
	// pods as. Empty allows only the namespace default
	AllowedServiceAccounts []string
	// AllowRootUser permits a spec securityContext that runs as uid 0.
	AllowRootUser bool
	// AllowedImages are repository prefixes spec.image may resolve to.
	// Empty denies every image.
	AllowedImages []string
}

func FromConfig() Policy {
	return Policy{
		Disabled:                 config.GetValue("POLICY_ENABLED") == "false",
		AllowedHosts:             parseList(config.GetValue("POLICY_ALLOWED_HOSTS")),
		AllowUnlabeledSecretRefs: config.GetValue("POLICY_REQUIRE_SECRET_REF_OPT_IN") == "false",
		AllowedServiceAccounts:   parseList(config.GetValue("POLICY_ALLOWED_SERVICE_ACCOUNTS")),
		AllowRootUser:            config.GetValue("POLICY_ALLOW_ROOT_USER") == "true",
		AllowedImages:            parseList(config.GetValue("POLICY_ALLOWED_IMAGES")),
	}
}

// AcceptedReason is the condition reason to record for a job that passed, so a
// cluster running without enforcement says so on every RenovateJob.
func (p Policy) AcceptedReason() string {
	if p.Disabled {
		return ReasonPolicyDisabled
	}
	return ReasonPolicySatisfied
}

// ValidateJob checks if a RenovateJob passes configured Policy
func (p Policy) ValidateJob(job *api.RenovateJob) error {
	if job == nil {
		return nil
	}
	if err := p.ValidateJobDestinations(job); err != nil {
		return err
	}
	return p.ValidateJobSpec(job.Spec)
}

// ValidateJobSpec checks what the resulting pod is allowed to be. The absolute
// invariants (no hostPath, no privileged, no privilege escalation) are enforced by
// the CRD schema instead, so they are rejected at apply time and cannot be switched
// off per install.
func (p Policy) ValidateJobSpec(spec api.RenovateJobSpec) error {
	if p.Disabled {
		return nil
	}

	if name := serviceAccountName(spec); name != "" && !slices.Contains(p.AllowedServiceAccounts, name) {
		if len(p.AllowedServiceAccounts) == 0 {
			return violationf(ReasonServiceAccountNotAllowed,
				"spec.serviceAccount.name: %q is not allowed because no service accounts are configured; add it to policy.allowedServiceAccounts, or leave the field unset to use the namespace default", name)
		}
		return violationf(ReasonServiceAccountNotAllowed,
			"spec.serviceAccount.name: %q is not allowed; add it to policy.allowedServiceAccounts (allowed: %s)", name, strings.Join(p.AllowedServiceAccounts, ", "))
	}

	if err := p.validateImage(spec.Image); err != nil {
		return err
	}

	if p.AllowRootUser || spec.SecurityContext == nil {
		return nil
	}

	if pod := spec.SecurityContext.Pod; pod != nil {
		if err := checkNonRoot("spec.securityContext.pod", pod.RunAsUser, pod.RunAsNonRoot); err != nil {
			return err
		}
	}
	if container := spec.SecurityContext.Container; container != nil {
		if err := checkNonRoot("spec.securityContext.container", container.RunAsUser, container.RunAsNonRoot); err != nil {
			return err
		}
	}
	return nil
}

func (p Policy) validateImage(image string) error {
	if image == "" {
		return nil
	}

	ref, err := parseImageRef(image)
	if err != nil {
		return violationf(ReasonImageNotAllowed, "spec.image: %q is not a valid image reference: %s", image, err)
	}

	if slices.Contains(p.AllowedImages, ref.Repository) {
		return nil
	}

	if len(p.AllowedImages) == 0 {
		return violationf(ReasonImageNotAllowed,
			"spec.image: %q is not allowed because no images are configured; set policy.allowedImages", image)
	}
	return violationf(ReasonImageNotAllowed,
		"spec.image: repository %q is not allowed; add it verbatim to policy.allowedImages (allowed: %s)", ref.Repository, strings.Join(p.AllowedImages, ", "))
}

// imageRef is an image reference split into the parts this package compares on.
type imageRef struct {
	// Repository is the reference with any tag and digest removed, otherwise exactly
	// as written. It is deliberately not normalized: see imageRepository.
	Repository string
	HasTag     bool
	HasDigest  bool
}

// imageRepositoryPattern is what a repository may contain. Deliberately strict:
// anything it rejects is refused rather than guessed at.
var imageRepositoryPattern = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*(:[0-9]+)?(/[a-z0-9]+([._-][a-z0-9]+)*)*$`)

/*
parseImageRef strips any tag and digest, leaving the repository exactly as written.

It deliberately does not normalize. Resolving implicit registries would mean deciding
that "renovate/renovate" and "docker.io/renovate/renovate" are the same string.
*/
func parseImageRef(image string) (imageRef, error) {
	ref := imageRef{}
	remainder := image

	if at := strings.Index(remainder, "@"); at >= 0 {
		if remainder[at+1:] == "" {
			return imageRef{}, fmt.Errorf("empty digest")
		}
		ref.HasDigest = true
		remainder = remainder[:at]
	}

	// A tag's colon is the one after the last "/". Any earlier colon is a registry
	// port, as in "registry.internal:5000/mirror/renovate".
	if colon := strings.LastIndex(remainder, ":"); colon > strings.LastIndex(remainder, "/") {
		if remainder[colon+1:] == "" {
			return imageRef{}, fmt.Errorf("empty tag")
		}
		ref.HasTag = true
		remainder = remainder[:colon]
	}

	if !imageRepositoryPattern.MatchString(remainder) {
		return imageRef{}, fmt.Errorf("%q is not a valid image repository", remainder)
	}

	ref.Repository = remainder
	return ref, nil
}

// ValidateAllowedImages reports whether a raw POLICY_ALLOWED_IMAGES value is
// well-formed, so a mistyped entry fails at startup rather than silently matching
// nothing.
func ValidateAllowedImages(raw string) error {
	for entry := range strings.SplitSeq(raw, ",") {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		// FromConfig lower-cases entries, so validate what will actually be stored.
		ref, err := parseImageRef(strings.ToLower(trimmed))
		if err != nil {
			return fmt.Errorf("%q is not a valid image repository: %w", trimmed, err)
		}
		// Entries are prefixes; a tag or digest on one would never match anything.
		if ref.HasTag {
			return fmt.Errorf("%q must not carry a tag: entries are repository prefixes", trimmed)
		}
		if ref.HasDigest {
			return fmt.Errorf("%q must not carry a digest: entries are repository prefixes", trimmed)
		}
	}
	return nil
}

func serviceAccountName(spec api.RenovateJobSpec) string {
	if spec.ServiceAccount == nil {
		return ""
	}
	return spec.ServiceAccount.Name
}

func checkNonRoot(field string, runAsUser *int64, runAsNonRoot *bool) error {
	if runAsUser != nil && *runAsUser == 0 {
		return violationf(ReasonRootUserNotAllowed,
			"%s.runAsUser: running as uid 0 is not allowed; set policy.allowRootUser=true to permit it", field)
	}
	if runAsNonRoot != nil && !*runAsNonRoot {
		return violationf(ReasonRootUserNotAllowed,
			"%s.runAsNonRoot: disabling it is not allowed; set policy.allowRootUser=true to permit it", field)
	}
	return nil
}

// ValidateReferencedSecret reports whether a RenovateJob may have the operator
// read the given secret.
func (p Policy) ValidateReferencedSecret(secret *corev1.Secret) error {
	if p.Disabled || p.AllowUnlabeledSecretRefs {
		return nil
	}
	if secret == nil {
		return violationf(ReasonSecretRefNotOptedIn, "no secret to check")
	}
	if secret.Labels[AllowRefLabel] == "true" {
		return nil
	}
	return violationf(ReasonSecretRefNotOptedIn, "secret %q is not opted in to being referenced by a RenovateJob; label it %s=%q, or set policy.requireSecretRefOptIn=false to disable this check", secret.Name, AllowRefLabel, "true")
}

// ValidateDestination reports whether the operator may direct traffic at rawURL.
// purpose names the spec field or setting under test and appears in the error, so
// the message can be handed straight to a user without further context.
func (p Policy) ValidateDestination(rawURL string, purpose string) error {
	if p.Disabled {
		return nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return violationf(ReasonInvalidDestinationURL, "%s: %q is not a valid URL: %s", purpose, rawURL, err)
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return violationf(ReasonInvalidDestinationURL, "%s: %q has no host", purpose, rawURL)
	}

	if p.hostAllowed(host) {
		return nil
	}

	if len(p.AllowedHosts) == 0 {
		return violationf(ReasonDestinationNotAllowed, "%s: host %q is not allowed because no destinations are configured; set policy.allowedHosts", purpose, host)
	}
	return violationf(ReasonDestinationNotAllowed, "%s: host %q is not allowed; add it to policy.allowedHosts (allowed: %s)", purpose, host, strings.Join(p.AllowedHosts, ", "))
}

// ValidateJobDestinations checks every externally reachable URL a RenovateJob can
// point the operator, its Jobs, or the Git platform at. New URL-valued spec fields
// belong here.
func (p Policy) ValidateJobDestinations(job *api.RenovateJob) error {
	if job == nil {
		return nil
	}

	// The effective endpoint, not the raw field: an empty spec.provider.endpoint
	// resolves to the platform's public API for github and gitlab, and that
	// resolved host is what the operator actually talks to.
	if _, endpoint := utils.GetPlatformAndEndpoint(job.Spec.Provider); endpoint != "" {
		if err := p.ValidateDestination(endpoint, "spec.provider.endpoint"); err != nil {
			return err
		}
	}

	if job.Spec.Provider != nil && job.Spec.Provider.PublicEndpoint != "" {
		if err := p.ValidateDestination(job.Spec.Provider.PublicEndpoint, "spec.provider.publicEndpoint"); err != nil {
			return err
		}
	}

	if job.Spec.Webhook != nil && job.Spec.Webhook.BaseURL != "" {
		if err := p.ValidateDestination(job.Spec.Webhook.BaseURL, "spec.webhook.baseUrl"); err != nil {
			return err
		}
	}

	return nil
}

func (p Policy) hostAllowed(host string) bool {
	return slices.Contains(p.AllowedHosts, host)
}

// hostEntryPattern accepts a bare hostname and nothing else: no scheme, port,
// path, wildcard, or leading dot.
var hostEntryPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)

// ValidateAllowedHosts reports whether a raw POLICY_ALLOWED_HOSTS value is
// well-formed. It exists so a mistyped entry fails at startup rather than
// silently matching nothing, which would refuse every job with a confusing
// reason.
func ValidateAllowedHosts(raw string) error {
	for entry := range strings.SplitSeq(raw, ",") {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, ".") || strings.Contains(trimmed, "*") {
			return fmt.Errorf("%q: subtree and wildcard entries are not supported, list each hostname explicitly", trimmed)
		}
		if !hostEntryPattern.MatchString(trimmed) {
			return fmt.Errorf("%q must be a bare hostname, without scheme, port or path", trimmed)
		}
	}
	return nil
}

func parseList(raw string) []string {
	parts := strings.Split(raw, ",")
	list := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.ToLower(strings.TrimSpace(part))
		if trimmed != "" {
			list = append(list, trimmed)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return list
}
