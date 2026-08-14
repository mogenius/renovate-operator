package policy

import (
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
)

// chartDefaultAllowedImages mirrors policy.allowedImages in
// charts/renovate-operator/values.yaml.
var chartDefaultAllowedImages = []string{
	"renovate/renovate",
	"docker.io/renovate/renovate",
	"ghcr.io/renovatebot/renovate",
}

func TestValidateImage(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		image   string
		ok      bool
	}{
		// Both Docker Hub spellings ship in the defaults, because the match is literal.
		{name: "docker hub short form", allowed: chartDefaultAllowedImages, image: "renovate/renovate:43.104.1", ok: true},
		{name: "fully qualified", allowed: chartDefaultAllowedImages, image: "docker.io/renovate/renovate:43.104.1", ok: true},
		{name: "no tag", allowed: chartDefaultAllowedImages, image: "renovate/renovate", ok: true},
		// A spelling nobody listed is refused rather than resolved onto one that is.
		{name: "index.docker.io form is not implied", allowed: chartDefaultAllowedImages, image: "index.docker.io/renovate/renovate:1", ok: false},
		{name: "digest reference", allowed: chartDefaultAllowedImages, image: "renovate/renovate@sha256:" + strings.Repeat("a", 64), ok: true},
		{name: "tag and digest", allowed: chartDefaultAllowedImages, image: "renovate/renovate:43.104.1@sha256:" + strings.Repeat("a", 64), ok: true},
		{name: "ghcr official", allowed: chartDefaultAllowedImages, image: "ghcr.io/renovatebot/renovate:43.104.1", ok: true},

		{name: "foreign registry", allowed: chartDefaultAllowedImages, image: "ghcr.io/attacker/renovate:latest", ok: false},
		{name: "foreign org on docker hub", allowed: chartDefaultAllowedImages, image: "attacker/renovate:latest", ok: false},
		{name: "sibling repo with a shared prefix", allowed: chartDefaultAllowedImages, image: "renovate/renovate-evil:latest", ok: false},
		{name: "sibling org with a shared prefix", allowed: []string{"renovate"}, image: "renovateevil/tool:latest", ok: false},
		// Matching is exact: an entry never authorises anything below it. Otherwise
		// listing an org would silently permit every repository someone adds to it.
		{name: "subpath below an entry is not implied", allowed: []string{"ghcr.io/renovatebot"}, image: "ghcr.io/renovatebot/renovate:1", ok: false},
		{name: "deeper subpath below an entry is not implied", allowed: []string{"renovate/renovate"}, image: "renovate/renovate/extra:1", ok: false},

		{name: "empty allowlist denies everything", allowed: nil, image: "renovate/renovate:1", ok: false},
		{name: "empty image is left to Kubernetes", allowed: nil, image: "", ok: true},
		{name: "malformed reference", allowed: chartDefaultAllowedImages, image: "NOT AN IMAGE", ok: false},

		{name: "entry on another registry does not match", allowed: []string{"ghcr.io/renovate/renovate"}, image: "docker.io/renovate/renovate:1", ok: false},
		// A registry mirror is just another literal entry.
		{name: "internal mirror with a port", allowed: []string{"registry.internal:5000/mirror/renovate"}, image: "registry.internal:5000/mirror/renovate:1", ok: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := Policy{AllowedImages: tc.allowed}

			err := p.validateImage(tc.image)
			if tc.ok && err != nil {
				t.Fatalf("expected %q to be allowed, got %v", tc.image, err)
			}
			if !tc.ok {
				if err == nil {
					t.Fatalf("expected %q to be denied", tc.image)
				}
				if got := ReasonFor(err); got != ReasonImageNotAllowed {
					t.Errorf("expected reason %q, got %q", ReasonImageNotAllowed, got)
				}
			}
		})
	}
}

// parseImageRef only strips the tag and digest; it deliberately does not resolve
// implicit registries, so every rule it does not have is a rule that cannot map an
// unexpected input onto an allowlist entry. This table is the contract.
func TestParseImageRef(t *testing.T) {
	tests := []struct {
		image      string
		repository string
		hasTag     bool
		hasDigest  bool
		wantErr    bool
	}{
		// The repository comes back exactly as written.
		{image: "renovate/renovate", repository: "renovate/renovate"},
		{image: "renovate/renovate:1.2.3", repository: "renovate/renovate", hasTag: true},
		{image: "docker.io/renovate/renovate", repository: "docker.io/renovate/renovate"},
		{image: "docker.io/renovate/renovate:1", repository: "docker.io/renovate/renovate", hasTag: true},
		{image: "index.docker.io/renovate/renovate", repository: "index.docker.io/renovate/renovate"},
		{image: "alpine", repository: "alpine"},
		{image: "alpine:3.20", repository: "alpine", hasTag: true},
		{image: "ghcr.io/renovatebot/renovate:1", repository: "ghcr.io/renovatebot/renovate", hasTag: true},
		{image: "deep/path/with/many/parts", repository: "deep/path/with/many/parts"},
		{image: "my_org/repo.name-v2", repository: "my_org/repo.name-v2"},
		// A registry port must not be mistaken for a tag: the tag colon is the one
		// after the last "/".
		{image: "registry.internal:5000/mirror/renovate", repository: "registry.internal:5000/mirror/renovate"},
		{image: "registry.internal:5000/mirror/renovate:1", repository: "registry.internal:5000/mirror/renovate", hasTag: true},
		{image: "localhost:5000/renovate:1", repository: "localhost:5000/renovate", hasTag: true},
		{image: "renovate/renovate@sha256:" + strings.Repeat("a", 64), repository: "renovate/renovate", hasDigest: true},
		{image: "renovate/renovate:1@sha256:" + strings.Repeat("a", 64), repository: "renovate/renovate", hasTag: true, hasDigest: true},

		// Everything below is refused rather than guessed at, so it can never be
		// normalized into something that matches an entry.
		{image: "NOT AN IMAGE", wantErr: true},
		{image: "docker.io/RENOVATE/renovate", wantErr: true},
		{image: "GHCR.IO/renovatebot/renovate", wantErr: true},
		{image: "renovate/renovate:", wantErr: true},
		{image: "renovate/renovate@", wantErr: true},
		{image: "renovate//renovate", wantErr: true},
		{image: "/renovate", wantErr: true},
		{image: "renovate/", wantErr: true},
		{image: "", wantErr: true},
		{image: "../../etc/passwd", wantErr: true},
		{image: "renovate/renovate/../evil", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.image, func(t *testing.T) {
			got, err := parseImageRef(tc.image)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected %q to be rejected, got %q", tc.image, got.Repository)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.image, err)
			}
			if got.Repository != tc.repository {
				t.Errorf("repository: got %q, want %q", got.Repository, tc.repository)
			}
			if got.HasTag != tc.hasTag {
				t.Errorf("hasTag: got %v, want %v", got.HasTag, tc.hasTag)
			}
			if got.HasDigest != tc.hasDigest {
				t.Errorf("hasDigest: got %v, want %v", got.HasDigest, tc.hasDigest)
			}
		})
	}
}

// A reference that cannot be parsed must never be treated as allowed.
func TestUnparseableImageIsDenied(t *testing.T) {
	p := Policy{AllowedImages: []string{"docker.io/renovate/renovate"}}

	for _, image := range []string{"NOT AN IMAGE", "renovate//renovate", "docker.io/RENOVATE/renovate", "renovate/renovate:"} {
		if err := p.validateImage(image); err == nil {
			t.Errorf("expected %q to be denied", image)
		}
	}
}

func TestValidateImageErrorNamesTheRepositoryToAdd(t *testing.T) {
	p := Policy{AllowedImages: chartDefaultAllowedImages}

	err := p.validateImage("attacker/renovate:latest")
	if err == nil {
		t.Fatal("expected denial")
	}
	// The message has to carry the repository without its tag, because that is exactly
	// the string a user would paste into policy.allowedImages.
	for _, want := range []string{`"attacker/renovate"`, "policy.allowedImages", "verbatim"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected the error to mention %q, got: %v", want, err)
		}
	}
}

func TestValidateAllowedImages(t *testing.T) {
	tests := []struct {
		raw   string
		valid bool
	}{
		{raw: "", valid: true},
		{raw: "docker.io/renovate/renovate", valid: true},
		{raw: "docker.io/renovate/renovate,ghcr.io/renovatebot/renovate", valid: true},
		{raw: " renovate/renovate , ghcr.io/renovatebot/renovate ", valid: true},
		{raw: "renovate/renovate", valid: true},
		{raw: "myregistry.internal:5000/mirror/renovate", valid: true},
		// Entries are prefixes, so a tag or digest could never match.
		{raw: "docker.io/renovate/renovate:43.104.1", valid: false},
		{raw: "docker.io/renovate/renovate@sha256:" + strings.Repeat("a", 64), valid: false},
		{raw: "NOT AN IMAGE", valid: false},
		{raw: "docker.io/renovate/renovate,NOT AN IMAGE", valid: false},
	}

	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			err := ValidateAllowedImages(tc.raw)
			if tc.valid && err != nil {
				t.Fatalf("expected %q to validate, got %v", tc.raw, err)
			}
			if !tc.valid && err == nil {
				t.Fatalf("expected %q to be rejected", tc.raw)
			}
		})
	}
}

func TestChartDefaultImagesAreAcceptedAndValid(t *testing.T) {
	if err := ValidateAllowedImages(strings.Join(chartDefaultAllowedImages, ",")); err != nil {
		t.Fatalf("the images shipped in values.yaml must pass validation: %v", err)
	}

	p := Policy{AllowedImages: chartDefaultAllowedImages}
	// The tag used throughout the docs has to work under the shipped defaults.
	for _, image := range []string{"renovate/renovate:43.104.1", "ghcr.io/renovatebot/renovate:43.104.1"} {
		if err := p.validateImage(image); err != nil {
			t.Errorf("documented image %q is rejected under the chart defaults: %v", image, err)
		}
	}
}

// The image check has to run through the same gate as everything else.
func TestValidateJobSpecCoversImage(t *testing.T) {
	p := Policy{AllowedImages: chartDefaultAllowedImages}

	if err := p.ValidateJobSpec(api.RenovateJobSpec{Image: "renovate/renovate:1"}); err != nil {
		t.Fatalf("expected an allowed image to pass: %v", err)
	}

	err := p.ValidateJobSpec(api.RenovateJobSpec{Image: "ghcr.io/attacker/renovate:1"})
	if got := ReasonFor(err); got != ReasonImageNotAllowed {
		t.Errorf("expected reason %q, got %q", ReasonImageNotAllowed, got)
	}
}
