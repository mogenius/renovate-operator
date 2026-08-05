package ui

import (
	"slices"

	api "renovate-operator/api/v1alpha1"

	"github.com/go-logr/logr"
)

// accessRole is the level of access a request holds on a single RenovateJob.
type accessRole int

const (
	// roleNone means the job must behave as if it does not exist.
	roleNone accessRole = iota
	// roleReader may read the job but not act on it.
	roleReader
	// roleAdmin may read the job and trigger, cancel and reconfigure its runs.
	roleAdmin
)

func (r accessRole) String() string {
	switch r {
	case roleReader:
		return "reader"
	case roleAdmin:
		return "admin"
	default:
		return ""
	}
}

// Permissions are the action identifiers the UI gates its controls on. The
// frontend never derives permissions from the role name, because two requests
// can hold roleReader and still differ on logs (see accessDecision.CanViewLogs).
const (
	permLogs       = "logs"
	permTrigger    = "trigger"
	permTriggerAll = "triggerAll"
	permCancel     = "cancel"
	permDiscovery  = "discovery"
)

// AccessDefaults are the operator-wide fallbacks for jobs that leave parts of
// their access configuration unset.
type AccessDefaults struct {
	ReaderGroups      []string
	AdminGroups       []string
	AnonymousRead     bool
	AnonymousReadLogs bool
}

// accessDecision is the outcome of evaluating one request against one job.
type accessDecision struct {
	Role accessRole
	// GroupMatched records that the role came from a group match rather than
	// from anonymous read, which is what separates a reader who may stream logs
	// from one who may not.
	GroupMatched bool
	CanViewLogs  bool
}

func (d accessDecision) canRead() bool  { return d.Role != roleNone }
func (d accessDecision) canWrite() bool { return d.Role == roleAdmin }

// permissions lists the actions this decision allows, for the UI to gate on.
func (d accessDecision) permissions() []string {
	perms := make([]string, 0, 5)
	if d.CanViewLogs {
		perms = append(perms, permLogs)
	}
	if d.canWrite() {
		perms = append(perms, permTrigger, permTriggerAll, permCancel, permDiscovery)
	}
	return perms
}

// has reports whether the decision allows the given permission.
func (d accessDecision) has(permission string) bool {
	return slices.Contains(d.permissions(), permission)
}

// adminDecision is the decision handed out when there is no identity to
// evaluate, i.e. no authentication provider is configured at all.
func adminDecision() accessDecision {
	return accessDecision{Role: roleAdmin, GroupMatched: true, CanViewLogs: true}
}

// effectiveAccess is a job's access configuration after operator-wide defaults
// have been applied.
type effectiveAccess struct {
	readerGroups      []string
	adminGroups       []string
	anonymousRead     bool
	anonymousReadLogs bool
}

// resolveEffectiveAccess merges a job's access configuration with the
// operator-wide defaults. Inheritance is per field: an unset field takes the
// default, so a job can add to the defaults but never remove them.
func resolveEffectiveAccess(job *api.RenovateJob, defaults AccessDefaults) effectiveAccess {
	eff := effectiveAccess{
		readerGroups:      defaults.ReaderGroups,
		adminGroups:       defaults.AdminGroups,
		anonymousRead:     defaults.AnonymousRead,
		anonymousReadLogs: defaults.AnonymousReadLogs,
	}

	// The deprecated allowedGroups is an alias for access.adminGroups. The CRD
	// rejects setting both, so reaching here with allowedGroups means access is
	// absent.
	if len(job.Spec.AllowedGroups) > 0 { //nolint:staticcheck // deprecated field is intentionally still honoured
		eff.adminGroups = normalizeGroups(job.Spec.AllowedGroups) //nolint:staticcheck // deprecated field is intentionally still honoured
		return eff
	}

	access := job.Spec.Access
	if access == nil {
		return eff
	}

	if len(access.ReaderGroups) > 0 {
		eff.readerGroups = normalizeGroups(access.ReaderGroups)
	}
	if len(access.AdminGroups) > 0 {
		eff.adminGroups = normalizeGroups(access.AdminGroups)
	}
	if access.AnonymousRead != nil {
		eff.anonymousRead = *access.AnonymousRead
	}
	if access.AnonymousReadLogs != nil {
		eff.anonymousReadLogs = *access.AnonymousReadLogs
	}

	return eff
}

// resolveAccess evaluates a session against a job's effective access
// configuration. A nil session represents a request without authentication,
// which only anonymous read can satisfy.
//
// Anonymous read acts as a floor rather than a sessionless special case: it
// grants read access to everyone, and group matches only ever add to it, so
// logging in can never take access away.
func resolveAccess(job *api.RenovateJob, session *sessionData, defaults AccessDefaults, logger logr.Logger) accessDecision {
	if job == nil {
		return accessDecision{}
	}

	// Normally unreachable: the CRD's CEL rule rejects this combination. It is
	// still possible when the CRD is managed outside the chart and has not been
	// upgraded, in which case there is no safe way to guess which surface wins.
	if len(job.Spec.AllowedGroups) > 0 && job.Spec.Access != nil { //nolint:staticcheck // deprecated field is intentionally still honoured
		logger.Error(nil, "access rules ignored, job treated as inaccessible: allowedGroups and access are mutually exclusive",
			"resource", job.Name,
			"namespace", job.Namespace)
		return accessDecision{}
	}

	eff := resolveEffectiveAccess(job, defaults)

	var userGroups []string
	if session != nil {
		userGroups = normalizeGroups(session.Groups)
	}

	switch {
	case hasIntersection(userGroups, eff.adminGroups):
		return accessDecision{Role: roleAdmin, GroupMatched: true, CanViewLogs: true}
	case hasIntersection(userGroups, eff.readerGroups):
		return accessDecision{Role: roleReader, GroupMatched: true, CanViewLogs: true}
	case eff.anonymousRead:
		// Renovate logs are unredacted, so anonymous readers need a second opt-in.
		return accessDecision{Role: roleReader, CanViewLogs: eff.anonymousReadLogs}
	}

	return accessDecision{}
}
