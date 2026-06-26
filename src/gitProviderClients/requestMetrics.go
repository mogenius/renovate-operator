package gitProviderClients

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"strings"
	"time"

	"renovate-operator/metricStore"
)

// Operation label values for Git-provider API metrics.
const (
	OperationGetRepositoryInfo = "get_repository_info"
	OperationSearch            = "search"
)

// RecordProviderRequest emits the Group K reliability metrics for a single
// outbound Git-provider API call. It is additive observability only and never
// alters control flow: callers still inspect resp/err exactly as before.
//
// Pass the time captured immediately before http.Client.Do, plus the resp and
// err it returned (resp may be nil when err is non-nil). The latency histogram
// is always observed. A transport error is recorded as a failed request and, if
// it stems from TLS/x509 verification, additionally increments the TLS-error
// counter. A response is classified into a 2xx/4xx/5xx status class, and a
// rate-limited response (HTTP 429 or rate-limit headers) increments the
// rate-limit counter.
func RecordProviderRequest(ctx context.Context, provider, operation string, start time.Time, resp *http.Response, err error) {
	metricStore.ObserveGitProviderRequestDuration(ctx, provider, operation, time.Since(start).Seconds())

	if err != nil {
		if isTLSError(err) {
			metricStore.IncGitProviderTLSError(ctx, provider)
		}
		// No HTTP response was produced; classify transport failures as 5xx so
		// the status class stays within the documented 2xx/4xx/5xx set.
		metricStore.IncGitProviderRequest(ctx, provider, operation, "5xx")
		return
	}
	if resp == nil {
		return
	}

	metricStore.IncGitProviderRequest(ctx, provider, operation, statusClass(resp.StatusCode))
	if isRateLimited(resp) {
		metricStore.IncGitProviderRateLimited(ctx, provider)
	}
}

// statusClass maps an HTTP status code to a low-cardinality 2xx/4xx/5xx label.
// Codes outside those ranges fall back to "5xx" so the label set stays bounded.
func statusClass(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 400 && code < 500:
		return "4xx"
	default:
		return "5xx"
	}
}

// isTLSError reports whether err originates from TLS or x509 certificate
// verification, using typed checks first and a string fallback for wrapped or
// platform-specific errors that do not expose the concrete types.
func isTLSError(err error) bool {
	if err == nil {
		return false
	}

	var certVerifyErr *tls.CertificateVerificationError
	var x509UnknownAuthority x509.UnknownAuthorityError
	var x509Hostname x509.HostnameError
	var x509CertInvalid x509.CertificateInvalidError
	var x509ConstraintViolation x509.ConstraintViolationError
	var x509SystemRoots x509.SystemRootsError
	if errors.As(err, &certVerifyErr) ||
		errors.As(err, &x509UnknownAuthority) ||
		errors.As(err, &x509Hostname) ||
		errors.As(err, &x509CertInvalid) ||
		errors.As(err, &x509ConstraintViolation) ||
		errors.As(err, &x509SystemRoots) {
		return true
	}

	msg := err.Error()
	return strings.Contains(msg, "tls:") || strings.Contains(msg, "x509:")
}

// isRateLimited reports whether resp indicates the provider throttled the
// request: an HTTP 429, an exhausted rate-limit budget, or a Retry-After hint.
func isRateLimited(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.Header.Get("Retry-After") != "" {
		return true
	}
	if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining == "0" {
		return true
	}
	return false
}

// providerNamer is an optional interface a GitProviderClient may implement to
// expose its metric provider label (e.g. "github"). It is intentionally kept
// separate from GitProviderClient so the core interface signature is unchanged.
type providerNamer interface {
	ProviderName() string
}

// providerLabel resolves the metric provider label for a client, falling back
// to "unknown" when the client does not advertise one.
func providerLabel(client GitProviderClient) string {
	if n, ok := client.(providerNamer); ok {
		return n.ProviderName()
	}
	return "unknown"
}
