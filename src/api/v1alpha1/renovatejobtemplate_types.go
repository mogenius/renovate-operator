package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Template kinds a RenovateJobTemplateRef may name.
const (
	KindRenovateJobTemplate        = "RenovateJobTemplate"
	KindClusterRenovateJobTemplate = "ClusterRenovateJobTemplate"
)

// RenovateJobTemplateRef points a RenovateJob at the shared configuration it
// inherits. A namespaced RenovateJobTemplate is resolved in the RenovateJob's own
// namespace; a ClusterRenovateJobTemplate is cluster-scoped and any namespace may
// reference it.
type RenovateJobTemplateRef struct {
	// Kind of the referenced template. Defaults to RenovateJobTemplate.
	// +kubebuilder:validation:Enum=RenovateJobTemplate;ClusterRenovateJobTemplate
	// +kubebuilder:default=RenovateJobTemplate
	// +optional
	Kind string `json:"kind,omitempty"`
	// Name of the referenced template.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

// IsCluster reports whether the ref names a cluster-scoped template.
func (r *RenovateJobTemplateRef) IsCluster() bool {
	return r != nil && r.Kind == KindClusterRenovateJobTemplate
}

// RenovateJobTemplate holds shared RenovateJob configuration that RenovateJobs in
// the same namespace inherit through spec.templateRef. Its spec has the same shape
// as a RenovateJob's; any subset of fields may be set.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=rjt
type RenovateJobTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RenovateJobSpec `json:"spec,omitempty"`
}

// ClusterRenovateJobTemplate holds shared RenovateJob configuration that
// RenovateJobs in any namespace inherit through spec.templateRef. Secret,
// ConfigMap and token references it carries resolve in the consuming RenovateJob's
// own namespace.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=crjt
type ClusterRenovateJobTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RenovateJobSpec `json:"spec,omitempty"`
}

type RenovateJobTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RenovateJobTemplate `json:"items"`
}

type ClusterRenovateJobTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterRenovateJobTemplate `json:"items"`
}

// MergeTemplateSpec overlays a RenovateJob's spec onto its template's, per
// top-level field: a field the overlay sets replaces base's wholesale, no deep
// merge. base is the template, overlay the job. The result aliases neither input.
func MergeTemplateSpec(base, overlay RenovateJobSpec) RenovateJobSpec {
	merged := *base.DeepCopy()
	ov := *overlay.DeepCopy()

	// Deliberately nil: the merged spec must not carry a TemplateRef, so a
	// second resolve of an already-resolved spec is a no-op.
	merged.TemplateRef = nil

	if ov.Schedule != "" {
		merged.Schedule = ov.Schedule
	}
	if ov.Image != "" {
		merged.Image = ov.Image
	}
	if ov.Provider != nil {
		merged.Provider = ov.Provider
	}
	if ov.DiscoveryFilters != nil {
		merged.DiscoveryFilters = ov.DiscoveryFilters
	}
	if ov.DiscoverTopics != nil {
		merged.DiscoverTopics = ov.DiscoverTopics
	}
	if ov.SkipForks != nil {
		merged.SkipForks = ov.SkipForks
	}
	if ov.SkipPendingDeletion != nil {
		merged.SkipPendingDeletion = ov.SkipPendingDeletion
	}
	if ov.Suspend != nil {
		merged.Suspend = ov.Suspend
	}
	if ov.SecretRef != "" {
		merged.SecretRef = ov.SecretRef
	}
	if ov.RenovateConfig != nil {
		merged.RenovateConfig = ov.RenovateConfig
	}
	if ov.ExtraEnv != nil {
		merged.ExtraEnv = ov.ExtraEnv
	}
	if ov.ExtraEnvFrom != nil {
		merged.ExtraEnvFrom = ov.ExtraEnvFrom
	}
	if ov.Parallelism != 0 {
		merged.Parallelism = ov.Parallelism
	}
	if len(ov.Resources.Limits) > 0 || len(ov.Resources.Requests) > 0 || len(ov.Resources.Claims) > 0 {
		merged.Resources = ov.Resources
	}
	if ov.NodeSelector != nil {
		merged.NodeSelector = ov.NodeSelector
	}
	if ov.Affinity != nil {
		merged.Affinity = ov.Affinity
	}
	if ov.Tolerations != nil {
		merged.Tolerations = ov.Tolerations
	}
	if ov.TopologySpreadConstraints != nil {
		merged.TopologySpreadConstraints = ov.TopologySpreadConstraints
	}
	if ov.PriorityClassName != "" {
		merged.PriorityClassName = ov.PriorityClassName
	}
	if ov.ServiceAccount != nil {
		merged.ServiceAccount = ov.ServiceAccount
	}
	if ov.Metadata != nil {
		merged.Metadata = ov.Metadata
	}
	if ov.SecurityContext != nil {
		merged.SecurityContext = ov.SecurityContext
	}
	if ov.Webhook != nil {
		merged.Webhook = ov.Webhook
	}
	if ov.ExtraVolumes != nil {
		merged.ExtraVolumes = ov.ExtraVolumes
	}
	if ov.ExtraVolumeMounts != nil {
		merged.ExtraVolumeMounts = ov.ExtraVolumeMounts
	}
	if ov.ImagePullSecrets != nil {
		merged.ImagePullSecrets = ov.ImagePullSecrets
	}
	if ov.DNSPolicy != "" {
		merged.DNSPolicy = ov.DNSPolicy
	}
	// allowedGroups (deprecated) and access are two spellings of the same access
	// control, mutually exclusive on any one object. A job that sets either replaces
	// the template's access control wholesale, dropping the inherited spelling too,
	// so the merged spec never carries both.
	if ov.AllowedGroups != nil || ov.Access != nil {
		merged.AllowedGroups = ov.AllowedGroups
		merged.Access = ov.Access
	}
	if ov.ScratchVolume != nil {
		merged.ScratchVolume = ov.ScratchVolume
	}
	if ov.GithubAppReference != nil {
		merged.GithubAppReference = ov.GithubAppReference
	}
	if ov.RuntimeClassName != nil {
		merged.RuntimeClassName = ov.RuntimeClassName
	}

	return merged
}

func (in *RenovateJobTemplate) DeepCopyInto(out *RenovateJobTemplate) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

func (in *RenovateJobTemplate) DeepCopy() *RenovateJobTemplate {
	if in == nil {
		return nil
	}
	out := new(RenovateJobTemplate)
	in.DeepCopyInto(out)
	return out
}

func (in *RenovateJobTemplate) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	return in.DeepCopy()
}

func (in *ClusterRenovateJobTemplate) DeepCopyInto(out *ClusterRenovateJobTemplate) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

func (in *ClusterRenovateJobTemplate) DeepCopy() *ClusterRenovateJobTemplate {
	if in == nil {
		return nil
	}
	out := new(ClusterRenovateJobTemplate)
	in.DeepCopyInto(out)
	return out
}

func (in *ClusterRenovateJobTemplate) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	return in.DeepCopy()
}

func (in *RenovateJobTemplateList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(RenovateJobTemplateList)
	*out = *in
	if in.Items != nil {
		out.Items = make([]RenovateJobTemplate, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}

func (in *ClusterRenovateJobTemplateList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(ClusterRenovateJobTemplateList)
	*out = *in
	if in.Items != nil {
		out.Items = make([]ClusterRenovateJobTemplate, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}
