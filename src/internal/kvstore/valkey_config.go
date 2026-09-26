package kvstore

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ValkeyConfig holds the configuration for connecting to Valkey.
// Either URL or Host must be set; URL takes precedence over
// Host/Port/Username/Password/TLS.
type ValkeyConfig struct {
	URL      string
	Host     string
	Port     string
	Username string
	Password string
	TLS      bool
	Cluster  bool
}

func (cfg *ValkeyConfig) IsConfigured() bool {
	return cfg.URL != "" || cfg.Host != ""
}

// ConfigFromEnv builds a ValkeyConfig by looking up the VALKEY_* configuration
// values through the provided getter (typically config.GetValue). Keeping the
// lookup injected avoids coupling this package to the config singleton.
func ConfigFromEnv(get func(key string) string) ValkeyConfig {
	return ValkeyConfig{
		URL:      get("VALKEY_URL"),
		Host:     get("VALKEY_HOST"),
		Port:     get("VALKEY_PORT"),
		Username: get("VALKEY_USERNAME"),
		Password: get("VALKEY_PASSWORD"),
		TLS:      get("VALKEY_TLS") == "true",
		Cluster:  get("VALKEY_CLUSTER") == "true",
	}
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
//   - Cluster (URL with redis+cluster:// / rediss+cluster://, or Host with Cluster set):
//     cluster mode only has database 0, so all usages share it; their key prefixes don't
//     collide. A cluster URL is returned unchanged.
//
// Returns "" if neither URL nor Host is configured.
func (cfg ValkeyConfig) URLForUsage(usage Usage) string {
	if isClusterURL(cfg.URL) {
		return cfg.URL
	}
	if cfg.URL != "" {
		return offsetURLDB(cfg.URL, int(usage))
	}
	if cfg.Cluster {
		return strings.Replace(BuildValkeyURL(cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.TLS, 0), "://", "+cluster://", 1)
	}
	return BuildValkeyURL(cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.TLS, int(usage))
}

// isClusterURL reports whether rawURL uses a cluster scheme (redis+cluster://,
// rediss+cluster://), the convention Renovate uses for cluster connections.
func isClusterURL(rawURL string) bool {
	scheme, _, ok := strings.Cut(rawURL, "://")
	return ok && strings.HasSuffix(scheme, "+cluster")
}

// BuildValkeyURL constructs a Valkey URL from host, port, credentials, and
// database index. Returns "" if host is empty. Uses the redis:// scheme
// (wire-compatible with Valkey), or rediss:// when useTLS is set.
// A username without a password is emitted as user@host (valid for ACL users
// created with nopass).
func BuildValkeyURL(host, port, username, password string, useTLS bool, db int) string {
	if host == "" {
		return ""
	}
	if port == "" {
		port = "6379"
	}
	scheme := "redis"
	if useTLS {
		scheme = "rediss"
	}
	u := url.URL{
		Scheme: scheme,
		Host:   host + ":" + port,
		Path:   fmt.Sprintf("/%d", db),
	}
	switch {
	case password != "":
		u.User = url.UserPassword(username, password)
	case username != "":
		u.User = url.User(username)
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
