package v1alpha1

// Well-known label keys the operator sets and selects on. They are part of this
// API group's contract - users apply LabelAllowRef by hand and third parties
// select Jobs by these keys - so they must never be spelled inline.

const (
	// LabelJobType records whether a Job is a discovery or an executor run.
	LabelJobType = GroupName + "/type"
	// LabelRenovateJob names the RenovateJob a Job belongs to.
	LabelRenovateJob = GroupName + "/renovatejob"
	// LabelProject holds the sanitized project name, since label values cannot
	// contain "/". Use ProjectAnnotationKey for the exact CRD status key.
	LabelProject = GroupName + "/project"
	// LabelGeneration stamps the Unix timestamp of the dispatch that created a Job,
	// so older generations can be collected.
	LabelGeneration = GroupName + "/generation"
)

// LabelAllowRef opts a Secret in to being dereferenced by a RenovateJob at a
// caller-chosen key. Enforced by internal/policy.
const LabelAllowRef = GroupName + "/allow-ref"

// Labels below are not owned by this group. They are the upstream recommended
// set, declared here so the operator spells them one way.
const (
	LabelAppManagedBy = "app.kubernetes.io/managed-by"
	LabelAppComponent = "app.kubernetes.io/component"

	// LabelValueManagedBy identifies this operator as the managing controller.
	LabelValueManagedBy = "renovate-operator"
	// LabelValueComponentValkeyCache marks the generated Secret holding the
	// Renovate cache URL.
	LabelValueComponentValkeyCache = "renovate-valkey-cache"
	// LabelValueComponentRenovateConfig marks the generated ConfigMap holding a
	// job's inline Renovate configuration.
	LabelValueComponentRenovateConfig = "renovate-config"
)

// FinalizerWebhookCleanup marks RenovateJobs whose synced webhooks must be
// removed from their repositories before the resource disappears.
const FinalizerWebhookCleanup = GroupName + "/webhook-cleanup"
