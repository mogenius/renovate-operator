// Package v1alpha1 contains API Schema definitions for the renovate v1alpha1 API group
// +kubebuilder:object:generate=true
// +groupName=renovate-operator.mogenius.com
package v1alpha1

import (
	"strings"

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

/*
Status of a single project within a RenovateJob
*/
type ProjectStatus struct {
	Name    string                `json:"name"`
	LastRun metav1.Time           `json:"lastRun"`
	Status  RenovateProjectStatus `json:"status"`
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

func (in *RenovateJob) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(RenovateJob)
	*out = *in
	return out
}

// unique name for a renovatejob ${name}-${namespace}
func (in *RenovateJob) Fullname() string {
	return in.Name + "-" + in.Namespace
}

// jobname for the executor job for a project. normalized for kubernetes resourcenames
func (in *RenovateJob) ExecutorJobName(project string) string {
	jobName := in.Name + "-" + project
	jobName = strings.ReplaceAll(jobName, "/", "-") // Replace slashes to avoid issues with Kubernetes naming
	jobName = strings.ReplaceAll(jobName, "_", "-")
	jobName = strings.ReplaceAll(jobName, ".", "-")
	jobName = strings.ToLower(jobName) // Ensure lowercase for consistency
	return jobName
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
	return out
}

func init() {
	SchemeBuilder.Register(&RenovateJob{}, &RenovateJobList{})
}
