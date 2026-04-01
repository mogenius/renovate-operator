// Package v1alpha1 contains API Schema definitions for the renovate v1alpha1 API group
// +kubebuilder:object:generate=true
// +groupName=renovate-operator.mogenius.com
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
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
	Image string `json:"image,omitempty"`
	// Filter to select which projects to process
	DiscoveryFilter string `json:"discoveryFilter,omitempty"`
	// Topics to discover projects from
	DiscoverTopics string `json:"discoverTopics,omitempty"`
	// Reference to the secret containing the renovate config
	SecretRef string `json:"secretRef,omitempty"`
	// Additional environment variables to set in the renovate container
	ExtraEnv []corev1.EnvVar `json:"extraEnv,omitempty"`
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
	Enabled        bool                 `json:"enabled"`
	Authentication *RenovateWebhookAuth `json:"authentication,omitempty"`
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

// PRAction represents what happened to a PR in a Renovate run
type PRAction string

const (
	PRActionAutomerged PRAction = "automerged"
	PRActionCreated    PRAction = "created"
	PRActionUpdated    PRAction = "updated"
	PRActionUnchanged  PRAction = "unchanged"
)

// PRDetail represents a single PR found in Renovate logs
type PRDetail struct {
	Branch string   `json:"branch"`
	Number int      `json:"number,omitempty"`
	URL    string   `json:"url,omitempty"`
	Title  string   `json:"title,omitempty"`
	Action PRAction `json:"action"`
}

// PRActivity contains aggregate counts and individual details of PR activity from a run
type PRActivity struct {
	Automerged int        `json:"automerged"`
	Created    int        `json:"created"`
	Updated    int        `json:"updated"`
	Unchanged  int        `json:"unchanged"`
	PRs        []PRDetail `json:"prs,omitempty"`
	Truncated  bool       `json:"truncated,omitempty"`
}

/*
Status of a single project within a RenovateJob
*/
type ProjectStatus struct {
	Name              string                `json:"name"`
	LastRun           metav1.Time           `json:"lastRun"`
	Status            RenovateProjectStatus `json:"status"`
	HasRenovateConfig *bool                 `json:"hasRenovateConfig,omitempty"`
	PRActivity        *PRActivity           `json:"prActivity,omitempty"`
}

type RenovateProjectStatus string

const (
	JobStatusScheduled RenovateProjectStatus = "scheduled"
	JobStatusRunning   RenovateProjectStatus = "running"
	JobStatusCompleted RenovateProjectStatus = "completed"
	JobStatusFailed    RenovateProjectStatus = "failed"
)

// RenovateJobStatus defines the observed state of RenovateJob
// +kubebuilder:object:root=true
type RenovateJobStatus struct {
	Projects []ProjectStatus `json:"projects,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type RenovateJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RenovateJobSpec   `json:"spec,omitempty"`
	Status RenovateJobStatus `json:"status,omitempty"`
}

// DeepCopyInto deep copies a RenovateJob into out
func (in *RenovateJob) DeepCopyInto(out *RenovateJob) {
	*out = *in
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

// DeepCopyInto deep copies a ProjectStatus into out
func (in *ProjectStatus) DeepCopyInto(out *ProjectStatus) {
	*out = *in
	if in.HasRenovateConfig != nil {
		out.HasRenovateConfig = new(bool)
		*out.HasRenovateConfig = *in.HasRenovateConfig
	}
	if in.PRActivity != nil {
		out.PRActivity = new(PRActivity)
		*out.PRActivity = *in.PRActivity
		if in.PRActivity.PRs != nil {
			out.PRActivity.PRs = make([]PRDetail, len(in.PRActivity.PRs))
			copy(out.PRActivity.PRs, in.PRActivity.PRs)
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
