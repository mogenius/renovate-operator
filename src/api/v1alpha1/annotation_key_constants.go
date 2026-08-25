// This file should be kept consistent with the annotations documented in
// docs/annotation-triggers.md, which is the user-facing contract for the trigger
// keys below.

package v1alpha1

// Annotations the operator stamps on the Kubernetes Jobs it creates.
const (
	// ProjectAnnotationKey stores the original project name, which may contain
	// characters a label value cannot carry.
	ProjectAnnotationKey = GroupName + "/project"
	// ScheduleAfterDiscoveryAnnotationKey marks a discovery Job whose result should
	// schedule all non-running projects. Set for cron-triggered discovery; omitted
	// for UI-triggered discovery, which only refreshes the project list.
	ScheduleAfterDiscoveryAnnotationKey = GroupName + "/schedule-after-discovery"
	// ProcessedAnnotationKey is stamped on a Job once its result has been fully
	// processed, so informer resyncs skip it instead of re-processing.
	ProcessedAnnotationKey = GroupName + "/processed"
)

// RenovateConfigMapAnnotationKey marks a RenovateJob whose inline renovate config
// the operator has synced into a ConfigMap.
const RenovateConfigMapAnnotationKey = GroupName + "/renovate-config-configmap"

// Annotations users apply to a RenovateJob to trigger a run. The operator removes
// each one once it has acted on it.
const (
	// TriggerDiscoveryAnnotationKey starts a discovery run when set to "true".
	TriggerDiscoveryAnnotationKey = GroupName + "/discovery"
	// TriggerScheduleAllAnnotationKey sets all non-running projects to Scheduled
	// when set to "true".
	TriggerScheduleAllAnnotationKey = GroupName + "/schedule-all"
	// TriggerScheduleAnnotationKey sets the listed non-running projects to
	// Scheduled. Its value is a comma-separated list of project names.
	TriggerScheduleAnnotationKey = GroupName + "/schedule"
)

// TokenExpiresAtAnnotationKey records an RFC3339 expiry on a Secret holding a
// generated GitHub App token, so it can be renewed before it lapses.
const TokenExpiresAtAnnotationKey = GroupName + "/token-expires-at"
