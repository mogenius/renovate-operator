package webhook

import "testing"

func TestRenovateCheckbox(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		expected bool
	}{
		{
			name:     "valid renovate description with checked checkbox",
			current:  "Some update\n- [x] <!-- rebase-check -->If you want to rebase/retry this MR",
			expected: true,
		},
		{
			name:     "valid renovate description with checked checkbox uppercase X",
			current:  "Some update\n- [X] <!-- rebase-check -->If you want to rebase/retry this MR",
			expected: true,
		},
		{
			name:     "renovate description but no checkbox checked",
			current:  "Some update\n- [ ] <!-- rebase-check -->If you want to rebase/retry this MR",
			expected: false,
		},
		{
			name:     "non-renovate description with checked checkbox",
			current:  "Some update\n- [x] This is not renovate",
			expected: false,
		},
		{
			name:     "empty description",
			current:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := verifyRenovateDescriptionChange(tt.current)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
