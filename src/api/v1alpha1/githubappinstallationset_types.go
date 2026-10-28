package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Account types a GitHub App installation can belong to.
const (
	GitHubAccountTypeUser         = "User"
	GitHubAccountTypeOrganization = "Organization"
)

// ConditionReady reports whether a GitHubAppInstallationSet's last sync
// succeeded.
const ConditionReady = "Ready"

// GitHubAppInstallationSetSpec defines the desired state of a GitHubAppInstallationSet.
type GitHubAppInstallationSetSpec struct {
	// Secret holding the GitHub App's id and private key. The Secret must carry
	// the renovate-operator.mogenius.com/allow-ref label once the policy engine
	// is enabled.
	App GitHubAppCredentialsReference `json:"app"`
	// GitHub API endpoints. Defaults to https://api.github.com; set it for
	// GitHub Enterprise Server. Copied to the provider of every generated
	// RenovateJob.
	// +optional
	Provider *GitHubAppProvider `json:"provider,omitempty"`
	// Template every generated RenovateJob inherits its configuration from.
	// The operator sets provider and githubAppReference on each generated
	// RenovateJob, so the template's values for those are overridden.
	TemplateRef RenovateJobTemplateRef `json:"templateRef"`
	// Narrows which installations get a RenovateJob. Every installation of
	// the App matches when unset.
	// +optional
	Installations *GitHubAppInstallationFilter `json:"installations,omitempty"`
	// How often the installations are listed from the GitHub API.
	// +kubebuilder:default="10m"
	// +kubebuilder:validation:XValidation:rule="duration(self) >= duration('1m')",message="resyncInterval must be at least 1m"
	// +optional
	ResyncInterval *metav1.Duration `json:"resyncInterval,omitempty"`
	// Resyncs immediately when GitHub delivers an installation or
	// installation_repositories event to the operator's /webhook/v1/github
	// endpoint.
	// +optional
	Webhook *GitHubAppInstallationSetWebhook `json:"webhook,omitempty"`
}

// GitHubAppCredentialsReference points at the keys of a Secret holding a
// GitHub App's id and private key.
type GitHubAppCredentialsReference struct {
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName"`
	// +kubebuilder:validation:MinLength=1
	AppIdSecretKey string `json:"appIdSecretKey"`
	// +kubebuilder:validation:MinLength=1
	PemSecretKey string `json:"pemSecretKey"`
}

// GitHubAppProvider holds the GitHub endpoints of a GitHubAppInstallationSet.
type GitHubAppProvider struct {
	// GitHub API base URL. Its host must be listed in the operator's
	// policy.allowedHosts.
	// +kubebuilder:validation:Pattern=`^https?://[^?#]+$`
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// Externally reachable URL used for UI links. Its host must be listed in
	// the operator's policy.allowedHosts.
	// +kubebuilder:validation:Pattern=`^https?://[^?#]+$`
	// +optional
	PublicEndpoint string `json:"publicEndpoint,omitempty"`
}

// GitHubAppInstallationFilter selects installations by the account they are
// installed on. Account logins match case-insensitively.
type GitHubAppInstallationFilter struct {
	// Account logins to include. Every account matches when empty.
	// +optional
	Include []string `json:"include,omitempty"`
	// Account logins to exclude. Takes precedence over include.
	// +optional
	Exclude []string `json:"exclude,omitempty"`
	// Account types to include. Every type matches when empty.
	// +optional
	// +kubebuilder:validation:items:Enum=User;Organization
	AccountTypes []string `json:"accountTypes,omitempty"`
}

// GitHubAppInstallationSetWebhook configures event-driven resyncs.
// +kubebuilder:validation:XValidation:rule="!self.enabled || has(self.secretRef)",message="secretRef is required when the webhook is enabled"
type GitHubAppInstallationSetWebhook struct {
	Enabled bool `json:"enabled"`
	// Secret key holding the GitHub App's webhook secret, used to verify the
	// X-Hub-Signature-256 header. The Secret must carry the
	// renovate-operator.mogenius.com/allow-ref label once the policy engine is
	// enabled.
	// +optional
	SecretRef *RenovateSecretKeyReference `json:"secretRef,omitempty"`
}

// GitHubAppInstallationSetStatus defines the observed state of a GitHubAppInstallationSet.
type GitHubAppInstallationSetStatus struct {
	// Generation of the spec the last successful sync used.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Time of the last successful listing of installations.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
	// resourceVersion of the source credentials Secret last written to every
	// generated credentials Secret.
	// +optional
	CredentialsResourceVersion string `json:"credentialsResourceVersion,omitempty"`
	// Installations matching the filter as of the last successful sync.
	// +optional
	// +listType=map
	// +listMapKey=id
	Installations []GitHubAppInstallationStatus `json:"installations,omitempty"`
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// GitHubAppInstallationStatus records one installation and the RenovateJob
// generated for it.
type GitHubAppInstallationStatus struct {
	ID          int64  `json:"id"`
	Account     string `json:"account"`
	AccountType string `json:"accountType"`
	// True when the installation is suspended on GitHub. Its RenovateJob is
	// suspended too, since no token can be minted for it.
	// +optional
	Suspended bool `json:"suspended,omitempty"`
	// Name of the generated RenovateJob. Empty until it has been created.
	// +optional
	RenovateJob string `json:"renovateJob,omitempty"`
}

// GitHubAppInstallationSet generates one RenovateJob per installation of a
// GitHub App, each inheriting its configuration from spec.templateRef.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=gais
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,priority=1,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Last Sync",type=date,JSONPath=`.status.lastSyncTime`
type GitHubAppInstallationSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitHubAppInstallationSetSpec   `json:"spec,omitempty"`
	Status GitHubAppInstallationSetStatus `json:"status,omitempty"`
}

type GitHubAppInstallationSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitHubAppInstallationSet `json:"items"`
}

// RenovateProvider is the provider every generated RenovateJob carries.
func (in *GitHubAppInstallationSetSpec) RenovateProvider() *RenovateProvider {
	p := &RenovateProvider{Name: "github"}
	if in.Provider != nil {
		p.Endpoint = in.Provider.Endpoint
		p.PublicEndpoint = in.Provider.PublicEndpoint
	}
	return p
}

func (in *GitHubAppInstallationFilter) DeepCopyInto(out *GitHubAppInstallationFilter) {
	*out = *in
	if in.Include != nil {
		out.Include = make([]string, len(in.Include))
		copy(out.Include, in.Include)
	}
	if in.Exclude != nil {
		out.Exclude = make([]string, len(in.Exclude))
		copy(out.Exclude, in.Exclude)
	}
	if in.AccountTypes != nil {
		out.AccountTypes = make([]string, len(in.AccountTypes))
		copy(out.AccountTypes, in.AccountTypes)
	}
}

func (in *GitHubAppInstallationSetSpec) DeepCopyInto(out *GitHubAppInstallationSetSpec) {
	*out = *in
	if in.Provider != nil {
		out.Provider = new(GitHubAppProvider)
		*out.Provider = *in.Provider
	}
	if in.Installations != nil {
		out.Installations = new(GitHubAppInstallationFilter)
		in.Installations.DeepCopyInto(out.Installations)
	}
	if in.ResyncInterval != nil {
		out.ResyncInterval = new(metav1.Duration)
		*out.ResyncInterval = *in.ResyncInterval
	}
	if in.Webhook != nil {
		out.Webhook = new(GitHubAppInstallationSetWebhook)
		*out.Webhook = *in.Webhook
		if in.Webhook.SecretRef != nil {
			out.Webhook.SecretRef = new(RenovateSecretKeyReference)
			*out.Webhook.SecretRef = *in.Webhook.SecretRef
		}
	}
}

func (in *GitHubAppInstallationSetStatus) DeepCopyInto(out *GitHubAppInstallationSetStatus) {
	*out = *in
	if in.LastSyncTime != nil {
		out.LastSyncTime = in.LastSyncTime.DeepCopy()
	}
	if in.Installations != nil {
		out.Installations = make([]GitHubAppInstallationStatus, len(in.Installations))
		copy(out.Installations, in.Installations)
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

func (in *GitHubAppInstallationSet) DeepCopyInto(out *GitHubAppInstallationSet) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *GitHubAppInstallationSet) DeepCopy() *GitHubAppInstallationSet {
	if in == nil {
		return nil
	}
	out := new(GitHubAppInstallationSet)
	in.DeepCopyInto(out)
	return out
}

func (in *GitHubAppInstallationSet) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	return in.DeepCopy()
}

func (in *GitHubAppInstallationSetList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(GitHubAppInstallationSetList)
	*out = *in
	if in.Items != nil {
		out.Items = make([]GitHubAppInstallationSet, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}
