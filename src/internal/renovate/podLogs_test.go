package renovate

import (
	"testing"
)

func TestParseDiscoveredProjects(t *testing.T) {
	tests := []struct {
		name      string
		logs      string
		want      []string
		wantErr   bool
	}{
		{
			name: "clean JSON array",
			logs: `["org/repo1","org/repo2"]`,
			want: []string{"org/repo1", "org/repo2"},
		},
		{
			name: "empty JSON array",
			logs: `[]`,
			want: []string{},
		},
		{
			name: "JSON array with stderr prefix",
			logs: "some stderr warning\nanother warning line\n" + `["org/repo1","org/repo2"]`,
			want: []string{"org/repo1", "org/repo2"},
		},
		{
			name: "JSON array with stderr before and after",
			logs: "WARNING: something\n" + `["org/repo1"]` + "\nsome trailing output",
			want: []string{"org/repo1"},
		},
		{
			name: "JSON array with node deprecation warnings",
			logs: "(node:1) [DEP0040] DeprecationWarning: some warning\n(Use `node --trace-deprecation ...` to show where the warning was created)\n" + `["org/repo1","org/repo2","org/repo3"]`,
			want: []string{"org/repo1", "org/repo2", "org/repo3"},
		},
		{
			name:    "no JSON array at all",
			logs:    "just some error output\nno json here",
			wantErr: true,
		},
		{
			name:    "empty logs",
			logs:    "",
			wantErr: true,
		},
		{
			name:    "invalid JSON array",
			logs:    `["incomplete`,
			wantErr: true,
		},
		{
			name: "JSON object line before array",
			logs: `{"level":30,"msg":"some log"}` + "\n" + `["org/repo1"]`,
			want: []string{"org/repo1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDiscoveredProjects(tt.logs)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseDiscoveredProjects() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("parseDiscoveredProjects() unexpected error: %v", err)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("parseDiscoveredProjects() got %d projects, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseDiscoveredProjects() got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
