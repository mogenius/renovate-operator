package ui

import (
	"strings"

	"renovate-operator/config"
)

// BasePath returns the normalized sub-path the UI is served under.
//
// The value comes from the BASE_PATH config item. It is normalized so that a
// non-empty base path always has a leading slash and never a trailing slash
// (e.g. "renovate/" and "/renovate/" both become "/renovate"). An empty base
// path means the UI is served from the domain root and BasePath returns "".
func BasePath() string {
	return normalizeBasePath(config.GetValue("BASE_PATH"))
}

// normalizeBasePath applies the leading-slash / no-trailing-slash rules.
func normalizeBasePath(raw string) string {
	p := strings.TrimSpace(raw)
	p = strings.Trim(p, "/")
	if p == "" {
		return ""
	}
	return "/" + p
}

// withBase prefixes an absolute application path with the base path. The input
// path must start with "/". When no base path is configured the path is
// returned unchanged.
func withBase(path string) string {
	base := BasePath()
	if base == "" {
		return path
	}
	if path == "/" {
		return base + "/"
	}
	return base + path
}

// cookiePath returns the Path attribute for cookies. It scopes cookies to the
// configured base path so they do not leak to other apps sharing the hostname.
// When no base path is set it returns "/".
func cookiePath() string {
	if base := BasePath(); base != "" {
		return base
	}
	return "/"
}
