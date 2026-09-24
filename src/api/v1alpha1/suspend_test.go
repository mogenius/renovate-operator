package v1alpha1

import "testing"

// A template can suspend every job that uses it, and a job's own value wins, in
// both directions, like skipForks.
func TestMergeTemplateSpec_Suspend(t *testing.T) {
	cases := []struct {
		name          string
		template, job *bool
		want          bool
	}{
		{name: "inherits the template's suspend", template: ptr(true), want: true},
		{name: "an explicit false on the job overrides it", template: ptr(true), job: ptr(false), want: false},
		{name: "the job suspends itself", job: ptr(true), want: true},
		{name: "neither sets it", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MergeTemplateSpec(RenovateJobSpec{Suspend: tc.template}, RenovateJobSpec{Suspend: tc.job})
			if got.GetSuspend() != tc.want {
				t.Errorf("expected suspend %v, got %v", tc.want, got.GetSuspend())
			}
		})
	}
}

// MergeTemplateSpec promises a result that aliases neither input, which rests on
// the deep copy not sharing the pointer.
func TestRenovateJobSpecDeepCopyDoesNotAliasSuspend(t *testing.T) {
	in := RenovateJobSpec{Suspend: ptr(true)}
	out := in.DeepCopy()

	*out.Suspend = false

	if !*in.Suspend {
		t.Error("the copy shares its Suspend pointer with the original")
	}
}
