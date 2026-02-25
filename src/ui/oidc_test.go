package ui

import (
	"testing"
)

func TestBuildOIDCScopes(t *testing.T) {
	tests := []struct {
		name       string
		additional []string
		want       []string
	}{
		{
			name:       "no additional scopes",
			additional: nil,
			want:       []string{"openid", "email", "profile"},
		},
		{
			name:       "empty slice",
			additional: []string{},
			want:       []string{"openid", "email", "profile"},
		},
		{
			name:       "single additional scope",
			additional: []string{"groups"},
			want:       []string{"openid", "email", "profile", "groups"},
		},
		{
			name:       "multiple additional scopes",
			additional: []string{"groups", "roles"},
			want:       []string{"openid", "email", "profile", "groups", "roles"},
		},
		{
			name:       "duplicate of base scope is deduplicated",
			additional: []string{"openid"},
			want:       []string{"openid", "email", "profile"},
		},
		{
			name:       "duplicate of email scope is deduplicated",
			additional: []string{"email"},
			want:       []string{"openid", "email", "profile"},
		},
		{
			name:       "duplicate within additional scopes",
			additional: []string{"groups", "groups"},
			want:       []string{"openid", "email", "profile", "groups"},
		},
		{
			name:       "mix of duplicates and new scopes",
			additional: []string{"openid", "groups", "email", "roles", "groups"},
			want:       []string{"openid", "email", "profile", "groups", "roles"},
		},
		{
			name:       "offline_access scope",
			additional: []string{"offline_access"},
			want:       []string{"openid", "email", "profile", "offline_access"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildOIDCScopes(tt.additional)
			if len(result) != len(tt.want) {
				t.Errorf("buildOIDCScopes() returned %d scopes, want %d\nGot:  %v\nWant: %v",
					len(result), len(tt.want), result, tt.want)
				return
			}
			for i := range result {
				if result[i] != tt.want[i] {
					t.Errorf("buildOIDCScopes()[%d] = %q, want %q", i, result[i], tt.want[i])
				}
			}
		})
	}
}
