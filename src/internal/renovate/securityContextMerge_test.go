package renovate

import (
	"testing"

	api "renovate-operator/api/v1alpha1"

	v1 "k8s.io/api/core/v1"
)

// A partial override used to replace the whole hardened context, silently dropping
// runAsNonRoot, the seccomp profile and the dropped capabilities along with it.
func TestPodSecurityContextPartialOverrideKeepsHardenedDefaults(t *testing.T) {
	spec := api.RenovateJobSpec{SecurityContext: &api.RenovateJobSecurityContext{
		Pod: &v1.PodSecurityContext{FSGroup: new(int64(2000))},
	}}

	got := getPodSecurityContext(spec)

	if got.FSGroup == nil || *got.FSGroup != 2000 {
		t.Errorf("expected the override to win, got %v", got.FSGroup)
	}
	if got.RunAsNonRoot == nil || !*got.RunAsNonRoot {
		t.Error("runAsNonRoot must survive a partial override")
	}
	if got.RunAsUser == nil || *got.RunAsUser != 12021 {
		t.Errorf("expected the default uid to survive, got %v", got.RunAsUser)
	}
	if got.RunAsGroup == nil || *got.RunAsGroup != 12021 {
		t.Errorf("expected the default gid to survive, got %v", got.RunAsGroup)
	}
	if got.SeccompProfile == nil || got.SeccompProfile.Type != v1.SeccompProfileTypeRuntimeDefault {
		t.Error("the seccomp profile must survive a partial override")
	}
}

func TestContainerSecurityContextPartialOverrideKeepsHardenedDefaults(t *testing.T) {
	spec := api.RenovateJobSpec{SecurityContext: &api.RenovateJobSecurityContext{
		Container: &v1.SecurityContext{ReadOnlyRootFilesystem: new(true)},
	}}

	got := getContainerSecurityContext(spec)

	if got.ReadOnlyRootFilesystem == nil || !*got.ReadOnlyRootFilesystem {
		t.Error("expected the override to win")
	}
	if got.Privileged == nil || *got.Privileged {
		t.Error("privileged=false must survive a partial override")
	}
	if got.AllowPrivilegeEscalation == nil || *got.AllowPrivilegeEscalation {
		t.Error("allowPrivilegeEscalation=false must survive a partial override")
	}
	if got.RunAsNonRoot == nil || !*got.RunAsNonRoot {
		t.Error("runAsNonRoot must survive a partial override")
	}
	if got.Capabilities == nil || len(got.Capabilities.Drop) != 1 || got.Capabilities.Drop[0] != "ALL" {
		t.Errorf("dropped capabilities must survive a partial override, got %v", got.Capabilities)
	}
	if got.SeccompProfile == nil || got.SeccompProfile.Type != v1.SeccompProfileTypeRuntimeDefault {
		t.Error("the seccomp profile must survive a partial override")
	}
}

func TestSecurityContextExplicitOverridesWin(t *testing.T) {
	spec := api.RenovateJobSpec{SecurityContext: &api.RenovateJobSecurityContext{
		Pod:       &v1.PodSecurityContext{RunAsUser: new(int64(1000)), RunAsGroup: new(int64(1001))},
		Container: &v1.SecurityContext{RunAsUser: new(int64(1000))},
	}}

	pod := getPodSecurityContext(spec)
	if *pod.RunAsUser != 1000 || *pod.RunAsGroup != 1001 {
		t.Errorf("expected the spec uid/gid to win, got %d/%d", *pod.RunAsUser, *pod.RunAsGroup)
	}
	if container := getContainerSecurityContext(spec); *container.RunAsUser != 1000 {
		t.Errorf("expected the spec uid to win on the container, got %d", *container.RunAsUser)
	}
}

func TestSecurityContextDefaultsWhenUnset(t *testing.T) {
	pod := getPodSecurityContext(api.RenovateJobSpec{})
	if pod.RunAsUser == nil || *pod.RunAsUser != 12021 || pod.RunAsNonRoot == nil || !*pod.RunAsNonRoot {
		t.Errorf("expected the hardened pod defaults, got %+v", pod)
	}

	container := getContainerSecurityContext(api.RenovateJobSpec{})
	if container.Privileged == nil || *container.Privileged {
		t.Errorf("expected the hardened container defaults, got %+v", container)
	}
}

// The spec comes from the informer cache, so building a Job must not alias, let
// alone mutate, the object every other reader shares.
func TestSecurityContextDoesNotAliasTheSpec(t *testing.T) {
	specPod := &v1.PodSecurityContext{FSGroup: new(int64(2000))}
	specContainer := &v1.SecurityContext{ReadOnlyRootFilesystem: new(true)}
	spec := api.RenovateJobSpec{SecurityContext: &api.RenovateJobSecurityContext{
		Pod:       specPod,
		Container: specContainer,
	}}

	pod := getPodSecurityContext(spec)
	if pod == specPod {
		t.Fatal("the returned pod security context aliases the spec")
	}
	container := getContainerSecurityContext(spec)
	if container == specContainer {
		t.Fatal("the returned container security context aliases the spec")
	}

	// Filling in the defaults must not have written them back into the spec.
	if specPod.RunAsNonRoot != nil {
		t.Error("the spec's pod security context was mutated")
	}
	if specContainer.Privileged != nil {
		t.Error("the spec's container security context was mutated")
	}
}

// mergedPodSecurityContext is the hardened pod default with only runAsUser overridden,
// which is what a spec that sets runAsUser alone should produce.
func mergedPodSecurityContext(uid int64) *v1.PodSecurityContext {
	merged := hardenedPodSecurityContext()
	merged.RunAsUser = new(int64(uid))
	return merged
}

// mergedContainerSecurityContext is the container equivalent of
// mergedPodSecurityContext.
func mergedContainerSecurityContext(uid int64) *v1.SecurityContext {
	merged := hardenedContainerSecurityContext()
	merged.RunAsUser = new(int64(uid))
	return merged
}
