package v1alpha1

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"
)

// ownedKeys is every label, annotation and finalizer key this group owns. Add new
// keys here so they are covered by the checks below.
var ownedKeys = map[string]string{
	"LabelJobType":                        LabelJobType,
	"LabelRenovateJob":                    LabelRenovateJob,
	"LabelProject":                        LabelProject,
	"LabelGeneration":                     LabelGeneration,
	"LabelAllowRef":                       LabelAllowRef,
	"ProjectAnnotationKey":                ProjectAnnotationKey,
	"ScheduleAfterDiscoveryAnnotationKey": ScheduleAfterDiscoveryAnnotationKey,
	"ProcessedAnnotationKey":              ProcessedAnnotationKey,
	"TriggerDiscoveryAnnotationKey":       TriggerDiscoveryAnnotationKey,
	"TriggerScheduleAllAnnotationKey":     TriggerScheduleAllAnnotationKey,
	"TriggerScheduleAnnotationKey":        TriggerScheduleAnnotationKey,
	"TokenExpiresAtAnnotationKey":         TokenExpiresAtAnnotationKey,
	"RenovateConfigMapAnnotationKey":      RenovateConfigMapAnnotationKey,
	"FinalizerWebhookCleanup":             FinalizerWebhookCleanup,
}

func TestOwnedKeysAreGroupQualifiedAndValid(t *testing.T) {
	for name, key := range ownedKeys {
		if !strings.HasPrefix(key, GroupName+"/") {
			t.Errorf("%s = %q, want the %q prefix", name, key, GroupName+"/")
		}
		if errs := validation.IsQualifiedName(key); len(errs) > 0 {
			t.Errorf("%s = %q is not a valid qualified name: %v", name, key, errs)
		}
	}
}

// Labels and annotations are separate maps on an object, so LabelProject and
// ProjectAnnotationKey may share a key - but two keys of the same kind must not.
func TestOwnedKeysAreUniquePerKind(t *testing.T) {
	seen := map[string]map[string]string{
		"label":      {},
		"annotation": {},
	}
	for name, key := range ownedKeys {
		var kind string
		switch {
		case strings.HasPrefix(name, "Label"):
			kind = "label"
		case strings.HasSuffix(name, "AnnotationKey"):
			kind = "annotation"
		default:
			continue
		}
		if other, taken := seen[kind][key]; taken {
			t.Errorf("%s and %s both use the %s key %q", other, name, kind, key)
		}
		seen[kind][key] = name
	}
}

func TestGroupNameMatchesGroupVersion(t *testing.T) {
	if GroupVersion.Group != GroupName {
		t.Errorf("GroupVersion.Group = %q, want %q", GroupVersion.Group, GroupName)
	}
}
