// Package v1alpha1 contains API Schema definitions for the renovate v1alpha1 API group
// +kubebuilder:object:generate=true
// +groupName=renovate-operator.mogenius.com
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// RenovateJobSpec defines the desired state of RenovateJob
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type RenovateJobSpec struct {
	// Cron schedule in standard cron format
	Schedule string `json:"schedule"`
	// Renovate Docker image to use
	Image string `json:"image"`
	// Renovate Provider Information to fill "RENOVATE_ENDPOINT" and "RENOVATE_PLATFORM" environment variables in the renovate container
	Provider *RenovateProvider `json:"provider"`
	// Filter to select which projects to process, will be concatenated using , separator
	DiscoveryFilters []string `json:"discoveryFilters,omitempty"`
	// Topics to discover projects from, will be concatenated using , separator
	DiscoverTopics []string `json:"discoverTopics,omitempty"`
	// If true, forked repositories discovered during autodiscovery will be excluded by querying the platform API
	SkipForks bool `json:"skipForks,omitempty"`
	// Reference to the secret containing the renovate config
	SecretRef string `json:"secretRef,omitempty"`
	// Additional environment variables to set in the renovate container
	ExtraEnv []corev1.EnvVar `json:"extraEnv,omitempty"`
	// Additional environment variable sources to set in the renovate container
	ExtraEnvFrom []corev1.EnvFromSource `json:"extraEnvFrom,omitempty"`
	// Maximum number of projects to process in parallel
	Parallelism int32 `json:"parallelism"`
	// Resource requirements for the renovate container
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// Node selector for scheduling the resulting pod
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Affinity settings for scheduling the resulting pod
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// Tolerations for scheduling the resulting pod
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// Topology spread constraints for scheduling the resulting pod
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	// Settings for the serviceaccount the renovate pod should use
	ServiceAccount *RenovateJobServiceAccount `json:"serviceAccount,omitempty"`
	// Metadata that shall be applied to the resulting pod
	Metadata *RenovateJobMetadata `json:"metadata,omitempty"`
	// Security context for the resulting pod and container
	SecurityContext *RenovateJobSecurityContext `json:"securityContext,omitempty"`
	// Configuration for webhooks to trigger renovate runs
	Webhook *RenovateWebhook `json:"webhook,omitempty"`
	// Additional volumes to mount in the renovate pods
	ExtraVolumes []corev1.Volume `json:"extraVolumes,omitempty"`
	// Additional volume mounts for the renovate pods
	ExtraVolumeMounts []corev1.VolumeMount `json:"extraVolumeMounts,omitempty"`
	// Image pull secrets for the renovate pods
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// DNS Policy for the renovate pods
	DNSPolicy corev1.DNSPolicy `json:"dnsPolicy,omitempty"`
	// Groups allowed to view this RenovateJob when authentication is enabled.
	// If empty or not set, the job is hidden from all users.
	// +optional
	AllowedGroups []string `json:"allowedGroups,omitempty"`
	// Configuration for the scratch volume
	// +optional
	ScratchVolume *RenovateJobScratchVolume `json:"scratchVolume,omitempty"`
}

type RenovateJobScratchVolume struct {
	// If enabled a scratch volume will be created and RENOVATE_BASE_DIR will be set accordingly
	// +kubebuilder:default=true
	// +optional
	Enabled bool `json:"enabled"`
	// Path within the container where the scratch volume will be mounted, RENOVATE_BASE_DIR will be set to this path.
	// +kubebuilder:default="/tmp"
	// +optional
	Path string `json:"path"`
	// Ephemeral uses a Kubernetes generic ephemeral volume for scratch (volume.ephemeral).
	// When set, Medium and SizeLimit are ignored.
	Ephemeral *corev1.EphemeralVolumeSource `json:"ephemeral,omitempty"`
	// Medium for the emptyDir volume. Ignored when Ephemeral is set.
	// Empty uses the node's default medium; Memory uses a tmpfs (corev1.StorageMediumMemory).
	Medium corev1.StorageMedium `json:"medium,omitempty"`
	// SizeLimit caps how large the emptyDir may grow (Kubernetes emptyDir.sizeLimit). Ignored when Ephemeral is set.
	SizeLimit *resource.Quantity `json:"sizeLimit,omitempty"`
}

// configuration regarding serviceaccounts for the resulting pod
type RenovateJobServiceAccount struct {
	AutomountServiceAccountToken *bool  `json:"automountServiceAccountToken,omitempty"`
	Name                         string `json:"name,omitempty"`
}

// security context for either the pod or the container
type RenovateJobSecurityContext struct {
	Pod       *corev1.PodSecurityContext `json:"pod,omitempty"`
	Container *corev1.SecurityContext    `json:"container,omitempty"`
}

// configuration for webhooks that can be used to trigger renovate runs
type RenovateWebhook struct {
	Enabled        bool                    `json:"enabled"`
	Authentication *RenovateWebhookAuth    `json:"authentication,omitempty"`
	Forgejo        *RenovateWebhookForgejo `json:"forgejo,omitempty"`
}

// Forgejo-specific webhook configuration
type RenovateWebhookForgejo struct {
	Sync *RenovateWebhookForgejoSync `json:"sync,omitempty"`
}

// configuration for syncing webhooks to Forgejo repos by topic
type RenovateWebhookForgejoSync struct {
	Enabled            bool                        `json:"enabled"`
	WebhookURL         string                      `json:"webhookURL"`
	Topic              string                      `json:"topic,omitempty"`
	Events             []string                    `json:"events,omitempty"`
	TokenSecretRef     *RenovateSecretKeyReference `json:"tokenSecretRef,omitempty"`
	AuthTokenSecretRef *RenovateSecretKeyReference `json:"authTokenSecretRef,omitempty"`
}

// authentication configuration for webhooks
type RenovateWebhookAuth struct {
	Enabled   bool                        `json:"enabled"`
	SecretRef *RenovateSecretKeyReference `json:"secretRef,omitempty"`
}

// reference to a secret and key
type RenovateSecretKeyReference struct {
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
}

// metadata that shall be applied to the resulting pod
type RenovateJobMetadata struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

/*
Renovate Provider Information
This will be used to fill "RENOVATE_ENDPOINT" and "RENOVATE_PLATFORM" environment variables in the renovate container
*/
type RenovateProvider struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint,omitempty"`
}

// PRAction represents what happened to a PR in a Renovate run.
type PRAction string

const (
	PRActionAutomerged PRAction = "automerged"
	PRActionCreated    PRAction = "created"
	PRActionUpdated    PRAction = "updated"
	PRActionUnchanged  PRAction = "unchanged"
)

// PRDetail represents a single PR found in Renovate logs.
type PRDetail struct {
	Branch string   `json:"branch"`
	Number int      `json:"number,omitempty"`
	Title  string   `json:"title,omitempty"`
	Action PRAction `json:"action"`
}

// PRActivity contains aggregate counts and individual details of PR activity from a run.
type PRActivity struct {
	Automerged int        `json:"automerged"`
	Created    int        `json:"created"`
	Updated    int        `json:"updated"`
	Unchanged  int        `json:"unchanged"`
	PRs        []PRDetail `json:"prs,omitempty"`
	Truncated  bool       `json:"truncated,omitempty"`
}

// LogIssue represents a single warning or error from Renovate logs.
type LogIssue struct {
	Level   int    `json:"level"`
	Message string `json:"message"`
}

// LogIssues contains aggregate counts and individual issue messages from a Renovate run.
type LogIssues struct {
	WarnCount  int        `json:"warnCount"`
	ErrorCount int        `json:"errorCount"`
	Issues     []LogIssue `json:"issues,omitempty"`
	Truncated  bool       `json:"truncated,omitempty"`
}

/*
Status of a single project within a RenovateJob
*/
type ProjectStatus struct {
	Name                 string                `json:"name"`
	LastRun              metav1.Time           `json:"lastRun"`
	Duration             *string               `json:"duration,omitempty"`
	Status               RenovateProjectStatus `json:"status"`
	Priority             int32                 `json:"priority,omitempty"`
	RenovateResultStatus *string               `json:"renovateResultStatus,omitempty"`
	PRActivity           *PRActivity           `json:"prActivity,omitempty"`
	LogIssues            *LogIssues            `json:"logIssues,omitempty"`
}

type RenovateProjectStatus string

const (
	JobStatusScheduled RenovateProjectStatus = "scheduled"
	JobStatusRunning   RenovateProjectStatus = "running"
	JobStatusCompleted RenovateProjectStatus = "completed"
	JobStatusFailed    RenovateProjectStatus = "failed"
	JobStatusCancelled RenovateProjectStatus = "cancelled"
)

// RenovateJobStatus defines the observed state of RenovateJob
// +kubebuilder:object:root=true
type RenovateJobStatus struct {
	Projects         []ProjectStatus           `json:"projects,omitempty"`
	ExecutionOptions *RenovateExecutionOptions `json:"executionOptions,omitempty"`
}

type RenovateExecutionOptions struct {
	// If true, the renovate job will be executed with LOG_LEVEL=debug
	Debug bool `json:"debug,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type RenovateJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RenovateJobSpec   `json:"spec,omitempty"`
	Status RenovateJobStatus `json:"status,omitempty"`
}

// DeepCopyInto deep copies a RenovateJobScratchVolume into out.
func (in *RenovateJobScratchVolume) DeepCopyInto(out *RenovateJobScratchVolume) {
	*out = *in
	if in.SizeLimit != nil {
		sl := in.SizeLimit.DeepCopy()
		out.SizeLimit = &sl
	}
	if in.Ephemeral != nil {
		out.Ephemeral = new(corev1.EphemeralVolumeSource)
		in.Ephemeral.DeepCopyInto(out.Ephemeral)
	}
}

// DeepCopyInto deep copies a RenovateJob into out.
func (in *RenovateJob) DeepCopyInto(out *RenovateJob) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Spec.ScratchVolume != nil {
		out.Spec.ScratchVolume = new(RenovateJobScratchVolume)
		in.Spec.ScratchVolume.DeepCopyInto(out.Spec.ScratchVolume)
	}
	// Deep copy Status.Projects (contains pointer and slice fields)
	if in.Status.Projects != nil {
		out.Status.Projects = make([]ProjectStatus, len(in.Status.Projects))
		for i := range in.Status.Projects {
			in.Status.Projects[i].DeepCopyInto(&out.Status.Projects[i])
		}
	}
}

func (in *RenovateJob) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(RenovateJob)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto deep copies a ProjectStatus into out.
func (in *ProjectStatus) DeepCopyInto(out *ProjectStatus) {
	*out = *in
	if in.Duration != nil {
		out.Duration = new(string)
		*out.Duration = *in.Duration
	}
	if in.RenovateResultStatus != nil {
		out.RenovateResultStatus = new(string)
		*out.RenovateResultStatus = *in.RenovateResultStatus
	}
	if in.PRActivity != nil {
		out.PRActivity = new(PRActivity)
		*out.PRActivity = *in.PRActivity
		if in.PRActivity.PRs != nil {
			out.PRActivity.PRs = make([]PRDetail, len(in.PRActivity.PRs))
			copy(out.PRActivity.PRs, in.PRActivity.PRs)
		}
	}
	if in.LogIssues != nil {
		out.LogIssues = new(LogIssues)
		*out.LogIssues = *in.LogIssues
		if in.LogIssues.Issues != nil {
			out.LogIssues.Issues = make([]LogIssue, len(in.LogIssues.Issues))
			copy(out.LogIssues.Issues, in.LogIssues.Issues)
		}
	}
}

// unique name for a renovatejob ${name}-${namespace}
func (in *RenovateJob) Fullname() string {
	return in.Name + "-" + in.Namespace
}

type RenovateJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RenovateJob `json:"items"`
}

func (in *RenovateJobList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(RenovateJobList)
	*out = *in
	if in.Items != nil {
		out.Items = make([]RenovateJob, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}

func init() {
	SchemeBuilder.Register(&RenovateJob{}, &RenovateJobList{})
}
