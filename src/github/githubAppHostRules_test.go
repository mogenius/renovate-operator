package github

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBuildHostRulesConfigScopesEachTokenToItsAccount(t *testing.T) {
	data, err := BuildHostRulesConfig("https://ghe.example.com/api/v3/", []InstallationToken{
		{Account: "org-a", Token: "ghs_a"},
		{Account: "presets", Token: "ghs_p"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var cfg struct {
		HostRules []map[string]string `json:"hostRules"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("config is not valid JSON: %v", err)
	}
	want := []map[string]string{
		{"matchHost": "https://ghe.example.com/api/v3/repos/org-a/", "hostType": "github", "token": "ghs_a"},
		{"matchHost": "https://ghe.example.com/api/v3/repos/presets/", "hostType": "github", "token": "ghs_p"},
	}
	if len(cfg.HostRules) != len(want) {
		t.Fatalf("expected %d rules, got %v", len(want), cfg.HostRules)
	}
	for i := range want {
		for k, v := range want[i] {
			if cfg.HostRules[i][k] != v {
				t.Errorf("rule %d: expected %s=%q, got %q", i, k, v, cfg.HostRules[i][k])
			}
		}
	}
}

func TestBuildHostRulesConfigWithoutTokensRendersEmptyList(t *testing.T) {
	data, err := BuildHostRulesConfig("https://api.github.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `{"hostRules":[]}` {
		t.Errorf("expected an empty hostRules list, got %s", data)
	}
}

func TestMintInstallationTokenCallsTheInstallationEndpoint(t *testing.T) {
	_, pemString, err := generateTestRSAKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	expires := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	var gotURL, gotAuth string
	httpClient := &http.Client{Transport: &mockRoundTripper{responseFunc: func(req *http.Request) (*http.Response, error) {
		gotURL = req.Method + " " + req.URL.String()
		gotAuth = req.Header.Get("Authorization")
		body := `{"token":"ghs_minted","expires_at":"` + expires.Format(time.RFC3339) + `"}`
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	}}}

	token, exp, err := NewInstallationTokenMinter(httpClient).MintInstallationToken("42", "7", pemString, "https://api.github.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "ghs_minted" || !exp.Equal(expires) {
		t.Errorf("expected ghs_minted expiring %s, got %q expiring %s", expires, token, exp)
	}
	if gotURL != "POST https://api.github.com/app/installations/7/access_tokens" {
		t.Errorf("unexpected request %q", gotURL)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("expected an App JWT, got %q", gotAuth)
	}
}
