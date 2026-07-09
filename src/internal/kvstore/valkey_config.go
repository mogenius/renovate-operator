package kvstore

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ValkeyConfig holds the configuration for connecting to Valkey.
// Either URL or Host must be set; URL takes precedence over Host/Port/Password.
type ValkeyConfig struct {
	URL      string
	Host     string
	Port     string
	Password string
}

func (cfg *ValkeyConfig) IsConfigured() bool {
	return cfg.URL != "" || cfg.Host != ""
}

// Usage identifies a logical Valkey database role.
// The value doubles as an offset from the base database when a predefined URL is used,
// or as an absolute database index when connecting via host/port.
type Usage int

const (
	UsageSessionStore  Usage = 0 // Session encryption store
	UsageRenovateCache Usage = 1 // Renovate job cache forwarded to executor jobs
	UsageRenovateLogs  Usage = 2 // Log storage for completed Renovate runs
)

// URLForUsage returns the Valkey connection URL for the given usage.
//
// Database selection depends on how the config is provided:
//
//   - URL-based (ValkeyConfig.URL set): the URL's database index is the base, and the
//     usage value is added as an offset. A predefined URL of redis://host/5 yields:
//     UsageSessionStore→5, UsageRenovateCache→6, UsageRenovateLogs→7.
//     If the URL carries no explicit database (e.g. redis://host), base is 0.
//
//   - Host-based (ValkeyConfig.Host set): usage value is the absolute database index.
//     UsageSessionStore→0, UsageRenovateCache→1, UsageRenovateLogs→2.
//
// Returns "" if neither URL nor Host is configured.
func (cfg ValkeyConfig) URLForUsage(usage Usage) string {
	if cfg.URL != "" {
		return offsetURLDB(cfg.URL, int(usage))
	}
	return BuildValkeyURL(cfg.Host, cfg.Port, cfg.Password, int(usage))
}

// BuildValkeyURL constructs a Valkey URL from host, port, password, and database index.
// Returns "" if host is empty. Uses the redis:// scheme (wire-compatible with Valkey).
func BuildValkeyURL(host, port, password string, db int) string {
	if host == "" {
		return ""
	}
	if port == "" {
		port = "6379"
	}
	u := url.URL{
		Scheme: "redis",
		Host:   host + ":" + port,
		Path:   fmt.Sprintf("/%d", db),
	}
	if password != "" {
		u.User = url.UserPassword("", password)
	}
	return u.String()
}

// offsetURLDB parses rawURL, reads its database index from the path as a base,
// adds offset, and returns the modified URL.
func offsetURLDB(rawURL string, offset int) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	base := 0
	if p := strings.TrimPrefix(u.Path, "/"); p != "" {
		if n, parseErr := strconv.Atoi(p); parseErr == nil {
			base = n
		}
	}
	u.Path = fmt.Sprintf("/%d", base+offset)
	return u.String()
}
