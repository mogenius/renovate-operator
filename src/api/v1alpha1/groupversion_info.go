package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupName is this API group, and the prefix of every label, annotation and
// finalizer the operator owns. It must match the +groupName marker in
// renovatejob_types.go, which controller-gen reads from the comment and cannot
// resolve from this constant.
const GroupName = "renovate-operator.mogenius.com"

var GroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

func AddToScheme(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion, &RenovateJob{}, &RenovateJobList{}, &RenovateProject{}, &RenovateProjectList{})
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}
