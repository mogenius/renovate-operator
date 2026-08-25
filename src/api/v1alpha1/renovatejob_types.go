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
// +kubebuilder:validation:XValidation:rule="!(has(self.allowedGroups) && has(self.access))",message="allowedGroups and access are mutually exclusive; migrate allowedGroups to access.adminGroups"
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
	// If true, repositories marked for delayed deletion (pending deletion) will be excluded by querying the platform API. Only GitLab exposes this state.
	SkipPendingDeletion bool `json:"skipPendingDeletion,omitempty"`
	// Reference to the secret containing the renovate config
	SecretRef string `json:"secretRef,omitempty"`
	// Renovate configuration file for the job pods
	// +optional
	RenovateConfig *RenovateJobConfig `json:"renovateConfig,omitempty"`
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
	// PriorityClassName for the resulting pod, used to set the pod's scheduling priority.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// Settings for the serviceaccount the renovate pod should use
	ServiceAccount *RenovateJobServiceAccount `json:"serviceAccount,omitempty"`
	// Metadata that shall be applied to the resulting pod
	Metadata *RenovateJobMetadata `json:"metadata,omitempty"`
	// Security context for the resulting pod and container
	SecurityContext *RenovateJobSecurityContext `json:"securityContext,omitempty"`
	// Configuration for webhooks to trigger renovate runs
	Webhook *RenovateWebhook `json:"webhook,omitempty"`
	// Additional volumes to mount in the renovate pods.
	// hostPath is rejected: it would give the pod the node's filesystem
	// +kubebuilder:validation:MaxItems=64
	// +kubebuilder:validation:XValidation:rule="self.all(v, !has(v.hostPath))",message="hostPath volumes are not allowed on RenovateJob pods"
	ExtraVolumes []corev1.Volume `json:"extraVolumes,omitempty"`
	// Additional volume mounts for the renovate pods
	// +kubebuilder:validation:MaxItems=64
	ExtraVolumeMounts []corev1.VolumeMount `json:"extraVolumeMounts,omitempty"`
	// Image pull secrets for the renovate pods
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// DNS Policy for the renovate pods
	DNSPolicy corev1.DNSPolicy `json:"dnsPolicy,omitempty"`
	// Deprecated: use Access.AdminGroups. Groups granted full access to this
	// RenovateJob when authentication is enabled. Mutually exclusive with Access.
	// +optional
	AllowedGroups []string `json:"allowedGroups,omitempty"`
	// Access control for this RenovateJob in the web UI when authentication is
	// enabled. If empty or not set, the job is hidden from all users.
	// +optional
	Access *RenovateJobAccess `json:"access,omitempty"`
	// Configuration for the scratch volume
	// +optional
	ScratchVolume *RenovateJobScratchVolume `json:"scratchVolume,omitempty"`
	// Reference to a Github App for authentication, this will automatically mount a secret with
	// RENOVATE_TOKEN
	GithubAppReference *GithubAppReference `json:"githubAppReference,omitempty"`
	// RuntimeClassName for the resulting pod, used to select a non-default container runtime
	// +optional
	RuntimeClassName *string `json:"runtimeClassName,omitempty"`
}

// Renovate configuration file source for the job pods
// +kubebuilder:validation:XValidation:rule="has(self.inline) != has(self.configMapRef)",message="exactly one of inline and configMapRef must be set"
type RenovateJobConfig struct {
	// Inline Renovate configuration, written to a ConfigMap owned by the RenovateJob
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=262144
	Inline string `json:"inline,omitempty"`
	// File name the inline configuration is mounted as; the extension tells Renovate the format. Defaults to "config.js". Ignored with configMapRef.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9._-]+\.(js|cjs|mjs|json|json5)$`
	FileName string `json:"fileName,omitempty"`
	// Reference to a key in an existing ConfigMap holding the configuration file
	// +optional
	ConfigMapRef *RenovateConfigMapKeyReference `json:"configMapRef,omitempty"`
}

// reference to a ConfigMap and key holding a Renovate configuration file
type RenovateConfigMapKeyReference struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name"`
	// Key holding the configuration file, also used as the mounted file name
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9._-]+\.(js|cjs|mjs|json|json5)$`
	Key string `json:"key"`
}

// access control for a RenovateJob in the web UI.
//
// Inheritance is per field: a field left unset takes the operator-wide default
// (DEFAULT_READER_GROUPS, DEFAULT_ADMIN_GROUPS, DEFAULT_READER_USERS,
// DEFAULT_ADMIN_USERS, DEFAULT_ANONYMOUS_READ, DEFAULT_ANONYMOUS_READ_LOGS). A
// field that IS set **replaces** the default for that field rather than adding
// to it, so listing one group here drops every default group, and setting
// anonymousRead: false revokes an enabled default. Fields left unset are
// unaffected by the ones that are set.
type RenovateJobAccess struct {
	// Groups allowed to view this RenovateJob without triggering, cancelling or
	// reconfiguring anything.
	// +optional
	ReaderGroups []string `json:"readerGroups,omitempty"`
	// Groups allowed to view this RenovateJob and to trigger, cancel and
	// reconfigure its runs.
	// +optional
	AdminGroups []string `json:"adminGroups,omitempty"`
	// Individual users allowed to view this RenovateJob, named by the email or
	// username their identity provider reports. Compared case-insensitively.
	// +optional
	ReaderUsers []string `json:"readerUsers,omitempty"`
	// Individual users allowed to view this RenovateJob and to trigger, cancel
	// and reconfigure its runs. See ReaderUsers for how a user is matched.
	// +optional
	AdminUsers []string `json:"adminUsers,omitempty"`
	// If true, this RenovateJob is readable without a session. Grants read access
	// to every visitor, which group matches can only extend.
	// +optional
	AnonymousRead *bool `json:"anonymousRead,omitempty"`
	// If true, visitors that only hold anonymous read access may also stream
	// Renovate logs. Has no effect unless AnonymousRead is in effect. Renovate
	// logs are unredacted, so this is opt-in separately from AnonymousRead.
	// +optional
	AnonymousReadLogs *bool `json:"anonymousReadLogs,omitempty"`
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

type GithubAppReference struct {
	SecretName              string `json:"secretName"`
	AppIdSecretKey          string `json:"appIdSecretKey"`
	InstallationIdSecretKey string `json:"installationIdSecretKey"`
	PemSecretKey            string `json:"pemSecretKey"`
}

// security context for either the pod or the container.
// Fields left unset keep the operator's hardened defaults; setting one field does not
// discard the others.
type RenovateJobSecurityContext struct {
	Pod *corev1.PodSecurityContext `json:"pod,omitempty"`
	// Container security context. privileged and allowPrivilegeEscalation are rejected
	// outright: both hand the pod a route off the node, which no Renovate use case
	// needs. Running as root is governed by the operator's policy.allowRootUser
	// instead, since a custom image may legitimately need it.
	// +kubebuilder:validation:XValidation:rule="!has(self.privileged) || !self.privileged",message="privileged containers are not allowed on RenovateJob pods"
	// +kubebuilder:validation:XValidation:rule="!has(self.allowPrivilegeEscalation) || !self.allowPrivilegeEscalation",message="allowPrivilegeEscalation is not allowed on RenovateJob pods"
	Container *corev1.SecurityContext `json:"container,omitempty"`
}

// configuration for webhooks that can be used to trigger renovate runs
type RenovateWebhook struct {
	Enabled bool `json:"enabled"`
	// Externally reachable base URL of the operator's webhook server for this
	// job, e.g. https://renovate.example.com. The platform-specific path is
	// appended to it. Takes precedence over the operator-wide
	// WEBHOOK_BASE_URL environment variable, which is used when this is empty.
	// Set it when a platform needs a different hostname to reach the operator
	// than the operator-wide default provides.
	// +optional
	// +kubebuilder:validation:Pattern=`^https?://[^?#]+$`
	BaseURL        string               `json:"baseUrl,omitempty"`
	Authentication *RenovateWebhookAuth `json:"authentication,omitempty"`
	Sync           *RenovateWebhookSync `json:"sync,omitempty"`
}

// configuration for syncing webhooks onto the repositories discovered for this
// job.
type RenovateWebhookSync struct {
	// Flag to enable the automatic repo webhook sync
	Enabled bool `json:"enabled"`
	// Optional reference to a secret key holding the platform token used for
	// webhook management. When not set, the job's platform token
	// (spec.secretRef or spec.githubAppReference) is used. If key is empty,
	// the common Renovate token key names are tried.
	// +optional
	SecretRef *RenovateSecretKeyReference `json:"secretRef,omitempty"`
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
	Name string `json:"name"`
	// Endpoint is the platform API base URL. Its host must be listed in the
	// operator's policy.allowedHosts, otherwise the job is refused.
	// +kubebuilder:validation:Pattern=`^https?://[^?#]+$`
	Endpoint string `json:"endpoint,omitempty"`
	// PublicEndpoint is the externally reachable URL for the provider, used only for UI links.
	// When set, this overrides Endpoint for dashboard links while Endpoint continues to be
	// used for Renovate API calls and cloning. Defaults to Endpoint when omitted.
	// Its host must be listed in the operator's policy.allowedHosts.
	// +kubebuilder:validation:Pattern=`^https?://[^?#]+$`
	PublicEndpoint string `json:"publicEndpoint,omitempty"`
}

// PRAction represents what happened to a PR in a Renovate run.
type PRAction string

const (
	PRActionAutomerged    PRAction = "automerged"
	PRActionCreated       PRAction = "created"
	PRActionUpdated       PRAction = "updated"
	PRActionNeedsApproval PRAction = "needs-approval"
	PRActionUnchanged     PRAction = "unchanged"
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
	Automerged    int        `json:"automerged"`
	Created       int        `json:"created"`
	Updated       int        `json:"updated"`
	NeedsApproval int        `json:"needsApproval"`
	Unchanged     int        `json:"unchanged"`
	PRs           []PRDetail `json:"prs,omitempty"`
	Truncated     bool       `json:"truncated,omitempty"`
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
	Name string `json:"name"`
	// LastTransition records when the project most recently changed state.
	LastTransition       metav1.Time               `json:"lastTransition,omitempty"`
	Duration             *string                   `json:"duration,omitempty"`
	Status               RenovateProjectStatus     `json:"status"`
	Priority             int32                     `json:"priority,omitempty"`
	RenovateResultStatus *string                   `json:"renovateResultStatus,omitempty"`
	PRActivity           *PRActivity               `json:"prActivity,omitempty"`
	LogIssues            *LogIssues                `json:"logIssues,omitempty"`
	ExecutionOptions     *RenovateExecutionOptions `json:"executionOptions,omitempty"`
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
	Projects []ProjectStatus `json:"projects,omitempty"`
	// Conditions holds the observed state of the RenovateJob. The operator sets the
	// "Accepted" condition to False when the job violates the operator's policy, with
	// a reason and a message naming the value to fix; nothing runs while it is False.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// ConditionAccepted reports whether the RenovateJob passes the operator's policy.
const ConditionAccepted = "Accepted"

type RenovateExecutionOptions struct {
	// If true, the renovate job will be executed with RENOVATE_LOG_LEVEL=debug
	Debug bool `json:"debug,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.provider.name`
// +kubebuilder:printcolumn:name="Accepted",type=string,JSONPath=`.status.conditions[?(@.type=="Accepted")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,priority=1,JSONPath=`.status.conditions[?(@.type=="Accepted")].reason`
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

// DeepCopyInto deep copies a RenovateJobAccess into out.
func (in *RenovateJobAccess) DeepCopyInto(out *RenovateJobAccess) {
	*out = *in
	if in.ReaderGroups != nil {
		out.ReaderGroups = make([]string, len(in.ReaderGroups))
		copy(out.ReaderGroups, in.ReaderGroups)
	}
	if in.AdminGroups != nil {
		out.AdminGroups = make([]string, len(in.AdminGroups))
		copy(out.AdminGroups, in.AdminGroups)
	}
	if in.ReaderUsers != nil {
		out.ReaderUsers = make([]string, len(in.ReaderUsers))
		copy(out.ReaderUsers, in.ReaderUsers)
	}
	if in.AdminUsers != nil {
		out.AdminUsers = make([]string, len(in.AdminUsers))
		copy(out.AdminUsers, in.AdminUsers)
	}
	if in.AnonymousRead != nil {
		out.AnonymousRead = new(bool)
		*out.AnonymousRead = *in.AnonymousRead
	}
	if in.AnonymousReadLogs != nil {
		out.AnonymousReadLogs = new(bool)
		*out.AnonymousReadLogs = *in.AnonymousReadLogs
	}
}

// DeepCopyInto deep copies a RenovateJob into out.
func (in *RenovateJob) DeepCopyInto(out *RenovateJob) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Spec.AllowedGroups != nil {
		out.Spec.AllowedGroups = make([]string, len(in.Spec.AllowedGroups))
		copy(out.Spec.AllowedGroups, in.Spec.AllowedGroups)
	}
	if in.Spec.Access != nil {
		out.Spec.Access = new(RenovateJobAccess)
		in.Spec.Access.DeepCopyInto(out.Spec.Access)
	}
	if in.Spec.ScratchVolume != nil {
		out.Spec.ScratchVolume = new(RenovateJobScratchVolume)
		in.Spec.ScratchVolume.DeepCopyInto(out.Spec.ScratchVolume)
	}
	if in.Spec.RuntimeClassName != nil {
		out.Spec.RuntimeClassName = new(string)
		*out.Spec.RuntimeClassName = *in.Spec.RuntimeClassName
	}
	if in.Spec.RenovateConfig != nil {
		out.Spec.RenovateConfig = new(RenovateJobConfig)
		*out.Spec.RenovateConfig = *in.Spec.RenovateConfig
		if in.Spec.RenovateConfig.ConfigMapRef != nil {
			out.Spec.RenovateConfig.ConfigMapRef = new(RenovateConfigMapKeyReference)
			*out.Spec.RenovateConfig.ConfigMapRef = *in.Spec.RenovateConfig.ConfigMapRef
		}
	}
	// Deep copy Status.Projects (contains pointer and slice fields)
	if in.Status.Projects != nil {
		out.Status.Projects = make([]ProjectStatus, len(in.Status.Projects))
		for i := range in.Status.Projects {
			in.Status.Projects[i].DeepCopyInto(&out.Status.Projects[i])
		}
	}
	if in.Status.Conditions != nil {
		out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
		copy(out.Status.Conditions, in.Status.Conditions)
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
	if in.ExecutionOptions != nil {
		out.ExecutionOptions = new(RenovateExecutionOptions)
		*out.ExecutionOptions = *in.ExecutionOptions
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
