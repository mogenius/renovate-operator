package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/oauth2"
)

func TestIsGroupFilterDenied(t *testing.T) {
	pattern := regexp.MustCompile(`^renovate-`)

	tests := []struct {
		name            string
		cfg             GroupFilterConfig
		validatedGroups []string
		want            bool
	}{
		{
			name:            "no filter, no groups — allowed",
			cfg:             GroupFilterConfig{},
			validatedGroups: nil,
			want:            false,
		},
		{
			name:            "no filter, some groups — allowed",
			cfg:             GroupFilterConfig{},
			validatedGroups: []string{"team-a"},
			want:            false,
		},
		{
			name:            "prefix filter, empty validated groups — denied",
			cfg:             GroupFilterConfig{AllowedPrefix: "renovate-"},
			validatedGroups: nil,
			want:            true,
		},
		{
			name:            "prefix filter, empty slice — denied",
			cfg:             GroupFilterConfig{AllowedPrefix: "renovate-"},
			validatedGroups: []string{},
			want:            true,
		},
		{
			name:            "prefix filter, matching groups survive — allowed",
			cfg:             GroupFilterConfig{AllowedPrefix: "renovate-"},
			validatedGroups: []string{"renovate-admin"},
			want:            false,
		},
		{
			name:            "pattern filter, empty validated groups — denied",
			cfg:             GroupFilterConfig{AllowedPattern: pattern},
			validatedGroups: []string{},
			want:            true,
		},
		{
			name:            "pattern filter, matching groups survive — allowed",
			cfg:             GroupFilterConfig{AllowedPattern: pattern},
			validatedGroups: []string{"renovate-team"},
			want:            false,
		},
		{
			name:            "both prefix and pattern, no survivors — denied",
			cfg:             GroupFilterConfig{AllowedPrefix: "renovate-", AllowedPattern: pattern},
			validatedGroups: []string{},
			want:            true,
		},
		{
			name:            "both prefix and pattern, survivors — allowed",
			cfg:             GroupFilterConfig{AllowedPrefix: "renovate-", AllowedPattern: pattern},
			validatedGroups: []string{"renovate-admin"},
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isGroupFilterDenied(tt.cfg, tt.validatedGroups)
			if got != tt.want {
				t.Errorf("isGroupFilterDenied() = %v, want %v", got, tt.want)
			}
		})
	}
}

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

func TestMergeGroups(t *testing.T) {
	tests := []struct {
		name           string
		idTokenGroups  []string
		userInfoGroups []string
		want           []string
	}{
		{
			name:           "two non-empty slices",
			idTokenGroups:  []string{"team-a", "team-b"},
			userInfoGroups: []string{"team-c", "team-d"},
			want:           []string{"team-a", "team-b", "team-c", "team-d"},
		},
		{
			name:           "overlapping entries are deduplicated",
			idTokenGroups:  []string{"team-a", "team-b"},
			userInfoGroups: []string{"team-b", "team-c"},
			want:           []string{"team-a", "team-b", "team-c"},
		},
		{
			name:           "first empty",
			idTokenGroups:  []string{},
			userInfoGroups: []string{"team-a"},
			want:           []string{"team-a"},
		},
		{
			name:           "second empty",
			idTokenGroups:  []string{"team-a"},
			userInfoGroups: []string{},
			want:           []string{"team-a"},
		},
		{
			name:           "both empty",
			idTokenGroups:  []string{},
			userInfoGroups: []string{},
			want:           []string{},
		},
		{
			name:           "nil and non-empty",
			idTokenGroups:  nil,
			userInfoGroups: []string{"team-a"},
			want:           []string{"team-a"},
		},
		{
			name:           "both nil",
			idTokenGroups:  nil,
			userInfoGroups: nil,
			want:           []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeGroups(tt.idTokenGroups, tt.userInfoGroups)
			if !slices.Equal(got, tt.want) {
				t.Errorf("mergeGroups() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseGroupsClaim(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"array of strings", `["team-a","team-b"]`, []string{"team-a", "team-b"}},
		{"single string coerced to slice", `"team-a"`, []string{"team-a"}},
		{"empty array", `[]`, []string{}},
		{"absent claim", ``, nil},
		{"null", `null`, nil},
		{"empty string", `""`, nil},
		{"non-string type ignored", `42`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGroupsClaim([]byte(tt.raw))
			if !slices.Equal(got, tt.want) {
				t.Errorf("parseGroupsClaim(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func newTestOIDCAuth(t *testing.T, endSessionURL, postLogoutRedirect, clientID string) *OIDCAuth {
	t.Helper()
	base, err := newBaseAuth(testEncryptionKey(t), logr.Discard(), NewMemorySessionStore())
	if err != nil {
		t.Fatalf("newBaseAuth: %v", err)
	}
	return &OIDCAuth{
		baseAuth:           base,
		oauth2Config:       oauth2.Config{ClientID: clientID},
		endSessionURL:      endSessionURL,
		postLogoutRedirect: postLogoutRedirect,
	}
}

func sessionCookieFor(t *testing.T, auth *OIDCAuth, sd sessionData) *http.Cookie {
	t.Helper()
	enc, err := auth.encryptSession(context.Background(), sd)
	if err != nil {
		t.Fatalf("encryptSession: %v", err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: enc}
}

func TestHandleLogout_IDTokenHint(t *testing.T) {
	const rawToken = "header.payload.sig"

	tests := []struct {
		name               string
		endSessionURL      string
		postLogoutRedirect string
		clientID           string
		idToken            string
		wantLocation       string
		wantContains       []string
		wantNotContains    []string
	}{
		{
			name:          "no end_session_endpoint — redirects locally, no hint",
			endSessionURL: "",
			clientID:      "my-client",
			idToken:       rawToken,
			wantLocation:  "/auth/logged-out",
		},
		{
			name:          "end_session_endpoint set, id_token present — appends hint",
			endSessionURL: "https://idp.example.com/logout",
			clientID:      "my-client",
			idToken:       rawToken,
			wantContains:  []string{"client_id=my-client", "id_token_hint=header.payload.sig"},
		},
		{
			name:               "end_session_endpoint and post_logout_redirect — all params present",
			endSessionURL:      "https://idp.example.com/logout",
			postLogoutRedirect: "https://app.example.com",
			clientID:           "my-client",
			idToken:            rawToken,
			wantContains:       []string{"client_id=my-client", "post_logout_redirect_uri=https://app.example.com", "id_token_hint=header.payload.sig"},
		},
		{
			name:            "end_session_endpoint set but no id_token — no hint param",
			endSessionURL:   "https://idp.example.com/logout",
			clientID:        "my-client",
			idToken:         "",
			wantContains:    []string{"client_id=my-client"},
			wantNotContains: []string{"id_token_hint"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := newTestOIDCAuth(t, tt.endSessionURL, tt.postLogoutRedirect, tt.clientID)

			sd := sessionData{
				Version: currentSessionVersion,
				Email:   "user@example.com",
				Groups:  []string{},
				Expiry:  time.Now().Add(time.Hour).Unix(),
				IDToken: tt.idToken,
			}

			req := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
			req.AddCookie(sessionCookieFor(t, auth, sd))
			w := httptest.NewRecorder()

			auth.HandleLogout(w, req)

			loc := w.Result().Header.Get("Location")
			if tt.wantLocation != "" && loc != tt.wantLocation {
				t.Errorf("Location = %q, want %q", loc, tt.wantLocation)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(loc, want) {
					t.Errorf("Location %q does not contain %q", loc, want)
				}
			}
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(loc, notWant) {
					t.Errorf("Location %q should not contain %q", loc, notWant)
				}
			}
		})
	}
}
