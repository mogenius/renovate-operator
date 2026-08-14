package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/go-logr/logr"
)

func TestNextPageURL(t *testing.T) {
	tests := []struct {
		name string
		link string
		want string
	}{
		{
			name: "no header",
			link: "",
			want: "",
		},
		{
			name: "next and last as GitHub sends them",
			link: `<https://api.github.com/user/teams?per_page=100&page=2>; rel="next", ` +
				`<https://api.github.com/user/teams?per_page=100&page=5>; rel="last"`,
			want: "https://api.github.com/user/teams?per_page=100&page=2",
		},
		{
			// The last page has prev/first but no next, which is what terminates
			// the loop in fetchAllPages.
			name: "last page has no next",
			link: `<https://api.github.com/user/teams?per_page=100&page=4>; rel="prev", ` +
				`<https://api.github.com/user/teams?per_page=100&page=1>; rel="first"`,
			want: "",
		},
		{
			name: "next is not the first entry",
			link: `<https://api.github.com/user/orgs?page=1>; rel="first", <https://api.github.com/user/orgs?page=3>; rel="next"`,
			want: "https://api.github.com/user/orgs?page=3",
		},
		{
			name: "unquoted rel is accepted",
			link: `<https://api.github.com/user/orgs?page=2>; rel=next`,
			want: "https://api.github.com/user/orgs?page=2",
		},
		{
			// "nextish" must not be mistaken for "next".
			name: "similar rel value is not matched",
			link: `<https://api.github.com/user/orgs?page=2>; rel="nextish"`,
			want: "",
		},
		{
			name: "malformed entry is ignored",
			link: `https://api.github.com/user/orgs?page=2; rel="next"`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextPageURL(tt.link); got != tt.want {
				t.Errorf("nextPageURL(%q) = %q, want %q", tt.link, got, tt.want)
			}
		})
	}
}

// newTestGitHubOAuth returns a provider whose requests go to the given handler.
func newTestGitHubOAuth(t *testing.T, handler http.Handler) (*GitHubOAuth, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &GitHubOAuth{
		baseAuth:   baseAuth{logger: logr.Discard()},
		httpClient: srv.Client(),
	}, srv
}

// TestFetchAllPagesFollowsLinkHeader covers the case the single-request version
// silently truncated: a user with more memberships than fit on one page.
func TestFetchAllPagesFollowsLinkHeader(t *testing.T) {
	type org struct {
		Login string `json:"login"`
	}

	var srv *httptest.Server
	requests := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/orgs?page=2>; rel="next"`, srv.URL))
			_, _ = w.Write([]byte(`[{"login":"org-a"},{"login":"org-b"}]`))
		case "2":
			// Final page: no Link header at all.
			_, _ = w.Write([]byte(`[{"login":"org-c"}]`))
		default:
			t.Errorf("unexpected page %q requested", page)
		}
	})

	provider, server := newTestGitHubOAuth(t, handler)
	srv = server

	orgs, err := fetchAllPages[org](provider, "token", srv.URL+"/orgs")
	if err != nil {
		t.Fatalf("fetchAllPages failed: %v", err)
	}

	got := make([]string, 0, len(orgs))
	for _, o := range orgs {
		got = append(got, o.Login)
	}
	want := []string{"org-a", "org-b", "org-c"}
	if !slices.Equal(got, want) {
		t.Errorf("orgs = %v, want %v", got, want)
	}
	if requests != 2 {
		t.Errorf("made %d requests, want 2", requests)
	}
}

// TestFetchAllPagesStopsAtPageLimit ensures a server that always advertises a
// next page cannot make one login loop forever, and that hitting the bound is an
// error rather than a truncated membership list.
func TestFetchAllPagesStopsAtPageLimit(t *testing.T) {
	type item struct {
		Login string `json:"login"`
	}

	var srv *httptest.Server
	requests := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Link", fmt.Sprintf(`<%s/orgs?page=next>; rel="next"`, srv.URL))
		_, _ = w.Write([]byte(`[{"login":"org"}]`))
	})

	provider, server := newTestGitHubOAuth(t, handler)
	srv = server

	if _, err := fetchAllPages[item](provider, "token", srv.URL+"/orgs"); err == nil {
		t.Fatal("expected an error when the page limit is exceeded, got nil")
	}
	if requests != maxGitHubPages {
		t.Errorf("made %d requests, want the %d page cap", requests, maxGitHubPages)
	}
}

// TestFetchGitHubGroupsFailsInsteadOfDowngrading pins the rule that a failed
// membership lookup must not produce a partial group list: the caller refuses to
// create a session, rather than minting one that quietly lacks access.
func TestFetchGitHubGroupsFailsInsteadOfDowngrading(t *testing.T) {
	// fetchGitHubGroups builds absolute api.github.com URLs, so the interception
	// has to happen at the transport rather than by pointing it at a test server.
	provider := &GitHubOAuth{
		baseAuth:   baseAuth{logger: logr.Discard()},
		httpClient: &http.Client{Transport: forbiddenTransport{}},
	}

	groups, err := provider.fetchGitHubGroups("token")
	if err == nil {
		t.Fatal("expected an error when membership cannot be determined, got nil")
	}
	if groups != nil {
		t.Errorf("groups = %v, want nil so no partial set can be used", groups)
	}
}

// forbiddenTransport answers every request with 403, standing in for GitHub
// refusing the read:org scope.
type forbiddenTransport struct{}

func (forbiddenTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusForbidden,
		Body:       http.NoBody,
		Header:     make(http.Header),
		Request:    r,
	}, nil
}
