package v1alpha1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func ptr[T any](v T) *T { return &v }

func TestMergeTemplateSpec_InheritsUnsetFields(t *testing.T) {
	base := RenovateJobSpec{
		Schedule:    "0 */6 * * *",
		Image:       "renovate/renovate:latest",
		Parallelism: 4,
		Provider:    &RenovateProvider{Name: "github"},
		SecretRef:   "shared-secret",
		ExtraEnv:    []corev1.EnvVar{{Name: "LOG_LEVEL", Value: "debug"}},
		SkipForks:   ptr(true),
	}
	overlay := RenovateJobSpec{
		// Only the per-job bits are set; everything else inherits.
		DiscoveryFilters: []string{"org/*"},
	}

	got := MergeTemplateSpec(base, overlay)

	if got.Schedule != "0 */6 * * *" {
		t.Errorf("schedule: expected inherited value, got %q", got.Schedule)
	}
	if got.Image != "renovate/renovate:latest" {
		t.Errorf("image: expected inherited value, got %q", got.Image)
	}
	if got.Parallelism != 4 {
		t.Errorf("parallelism: expected inherited 4, got %d", got.Parallelism)
	}
	if got.SecretRef != "shared-secret" {
		t.Errorf("secretRef: expected inherited value, got %q", got.SecretRef)
	}
	if got.SkipForks == nil || !*got.SkipForks {
		t.Errorf("skipForks: expected inherited true, got %v", got.SkipForks)
	}
	if len(got.DiscoveryFilters) != 1 || got.DiscoveryFilters[0] != "org/*" {
		t.Errorf("discoveryFilters: expected the job's own value, got %v", got.DiscoveryFilters)
	}
	if len(got.ExtraEnv) != 1 || got.ExtraEnv[0].Name != "LOG_LEVEL" {
		t.Errorf("extraEnv: expected inherited value, got %v", got.ExtraEnv)
	}
}

func TestMergeTemplateSpec_OverlayReplacesPerField(t *testing.T) {
	base := RenovateJobSpec{
		Schedule:    "0 */6 * * *",
		Image:       "renovate/renovate:37",
		Parallelism: 4,
		ExtraEnv:    []corev1.EnvVar{{Name: "FROM_TEMPLATE", Value: "1"}},
		SkipForks:   ptr(true),
	}
	overlay := RenovateJobSpec{
		Schedule:    "*/5 * * * *",
		Image:       "renovate/renovate:38",
		Parallelism: 1,
		ExtraEnv:    []corev1.EnvVar{{Name: "FROM_JOB", Value: "1"}},
		SkipForks:   ptr(false),
	}

	got := MergeTemplateSpec(base, overlay)

	if got.Schedule != "*/5 * * * *" {
		t.Errorf("schedule: expected override, got %q", got.Schedule)
	}
	if got.Image != "renovate/renovate:38" {
		t.Errorf("image: expected override, got %q", got.Image)
	}
	if got.Parallelism != 1 {
		t.Errorf("parallelism: expected override 1, got %d", got.Parallelism)
	}
	// A whole-list replace, not a merge: the template's entry must be gone.
	if len(got.ExtraEnv) != 1 || got.ExtraEnv[0].Name != "FROM_JOB" {
		t.Errorf("extraEnv: expected the job's list to replace the template's, got %v", got.ExtraEnv)
	}
	// The whole reason skipForks is a pointer: an explicit false must override an inherited true.
	if got.SkipForks == nil || *got.SkipForks {
		t.Errorf("skipForks: expected explicit false to override inherited true, got %v", got.SkipForks)
	}
}

func TestMergeTemplateSpec_DoesNotMutateInputs(t *testing.T) {
	base := RenovateJobSpec{ExtraEnv: []corev1.EnvVar{{Name: "A"}}}
	overlay := RenovateJobSpec{DiscoveryFilters: []string{"x"}}

	got := MergeTemplateSpec(base, overlay)
	got.ExtraEnv[0].Name = "MUTATED"
	got.DiscoveryFilters[0] = "mutated"

	if base.ExtraEnv[0].Name != "A" {
		t.Error("merge aliased the template's ExtraEnv slice")
	}
	if overlay.DiscoveryFilters[0] != "x" {
		t.Error("merge aliased the job's DiscoveryFilters slice")
	}
}

func TestMergeTemplateSpec_ClearsTemplateRef(t *testing.T) {
	base := RenovateJobSpec{Image: "renovate/renovate:37"}
	overlay := RenovateJobSpec{
		TemplateRef: &RenovateJobTemplateRef{Name: "shared"},
	}

	got := MergeTemplateSpec(base, overlay)
	if got.TemplateRef != nil {
		t.Errorf("TemplateRef must be nil after merge to prevent double-resolve, got %+v", got.TemplateRef)
	}
}

func TestMergeTemplateSpec_AccessControlIsMutuallyExclusive(t *testing.T) {
	// Template uses the deprecated allowedGroups, job migrates to access. The merged
	// spec must carry only the job's access, never both (the CRD forbids both on one
	// object and the effective spec is never validated by the API server).
	base := RenovateJobSpec{AllowedGroups: []string{"legacy-admins"}}
	overlay := RenovateJobSpec{Access: &RenovateJobAccess{AdminGroups: []string{"platform"}}}

	got := MergeTemplateSpec(base, overlay)

	if got.AllowedGroups != nil {
		t.Errorf("expected inherited allowedGroups to be dropped when the job sets access, got %v", got.AllowedGroups)
	}
	if got.Access == nil || len(got.Access.AdminGroups) != 1 || got.Access.AdminGroups[0] != "platform" {
		t.Errorf("expected the job's access to win, got %v", got.Access)
	}

	// The reverse: template uses access, job sets the deprecated allowedGroups.
	base = RenovateJobSpec{Access: &RenovateJobAccess{AdminGroups: []string{"platform"}}}
	overlay = RenovateJobSpec{AllowedGroups: []string{"team"}}

	got = MergeTemplateSpec(base, overlay)

	if got.Access != nil {
		t.Errorf("expected inherited access to be dropped when the job sets allowedGroups, got %v", got.Access)
	}
	if len(got.AllowedGroups) != 1 || got.AllowedGroups[0] != "team" {
		t.Errorf("expected the job's allowedGroups to win, got %v", got.AllowedGroups)
	}
}

func TestRenovateJobTemplateRef_IsCluster(t *testing.T) {
	if (&RenovateJobTemplateRef{Kind: KindClusterRenovateJobTemplate}).IsCluster() != true {
		t.Error("expected cluster ref to report IsCluster")
	}
	if (&RenovateJobTemplateRef{Kind: KindRenovateJobTemplate}).IsCluster() != false {
		t.Error("namespaced ref must not report IsCluster")
	}
	if (&RenovateJobTemplateRef{}).IsCluster() != false {
		t.Error("empty kind defaults to namespaced, must not report IsCluster")
	}
}
