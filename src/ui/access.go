package ui

import (
	"slices"
	"sync"
	"time"

	api "renovate-operator/api/v1alpha1"

	"github.com/go-logr/logr"
)

// accessCheckTTL is how long a misconfiguration verdict is reused. Short enough
// that fixing the configuration still clears the banner without a restart, long
// enough that a polling dashboard does not list every RenovateJob per request.
const accessCheckTTL = 10 * time.Second

// accessCheckCache holds the most recent misconfiguration verdict. Requests are
// served concurrently, so every field is behind the mutex.
type accessCheckCache struct {
	mu       sync.RWMutex
	verdict  *AccessMisconfiguration
	computed time.Time
}

// load returns the cached verdict, and whether it is still fresh enough to use.
func (c *accessCheckCache) load() (*AccessMisconfiguration, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.computed.IsZero() || time.Since(c.computed) > accessCheckTTL {
		return nil, false
	}
	return c.verdict, true
}

// store records a freshly computed verdict and reports whether it differs from
// the previous one, so the caller can log transitions rather than every check.
func (c *accessCheckCache) store(verdict *AccessMisconfiguration) (changed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	previous := ""
	if c.verdict != nil {
		previous = c.verdict.Reason
	}
	current := ""
	if verdict != nil {
		current = verdict.Reason
	}

	// A first "enforceable" verdict is the normal state, not a transition worth
	// announcing.
	firstAndClean := c.computed.IsZero() && verdict == nil
	changed = previous != current && !firstAndClean

	c.verdict = verdict
	c.computed = time.Now()
	return changed
}

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
	ReaderUsers       []string
	AdminUsers        []string
	AnonymousRead     bool
	AnonymousReadLogs bool
	// AuthorizationDisabled turns every authenticated request into an admin and
	// stops group and user rules from being evaluated at all. It is spelled
	// negatively, like policy.Disabled, so the zero value enforces and a test
	// constructing AccessDefaults{} cannot silently void authorization.
	AuthorizationDisabled bool
}

// hasGroups reports whether any group-based rule is configured operator-wide.
func (d AccessDefaults) hasGroups() bool {
	return len(d.ReaderGroups) > 0 || len(d.AdminGroups) > 0
}

// ReasonGroupsUnsupported marks access rules written against groups while the
// configured identity provider supplies none.
const ReasonGroupsUnsupported = "GroupsUnsupported"

// AccessMisconfiguration is an access configuration the operator cannot
// enforce. It is served to the UI so a deployment in this state says why
// instead of showing an empty dashboard.
type AccessMisconfiguration struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

const groupsUnsupportedMessage = "Access rules are configured against groups, but the identity provider supplies none, " +
	"so no group rule can ever match. Every RenovateJob stays hidden until this is fixed. " +
	"Either set auth.github.orgGroups=true (GITHUB_ORG_GROUPS) to map GitHub org and team membership to groups, " +
	"or replace the group lists with adminUsers/readerUsers, which name individual accounts and need no groups at all. " +
	"See the operator logs for the affected RenovateJobs."

func detectAccessMisconfiguration(provider AuthProvider, defaults AccessDefaults, jobs []api.RenovateJob) (m *AccessMisconfiguration, jobsWithGroups []string) {
	if provider == nil || provider.SupportsGroups() {
		return nil, nil
	}

	// Group rules that are never evaluated cannot hide anything, so a provider
	// without groups is not a misconfiguration here.
	if defaults.AuthorizationDisabled {
		return nil, nil
	}

	for i := range jobs {
		if jobConfiguresGroups(&jobs[i]) {
			jobsWithGroups = append(jobsWithGroups, jobs[i].Namespace+"/"+jobs[i].Name)
		}
	}

	if !defaults.hasGroups() && len(jobsWithGroups) == 0 {
		return nil, nil
	}

	return &AccessMisconfiguration{
		Reason:  ReasonGroupsUnsupported,
		Message: groupsUnsupportedMessage,
	}, jobsWithGroups
}

func jobConfiguresGroups(job *api.RenovateJob) bool {
	if len(job.Spec.AllowedGroups) > 0 { //nolint:staticcheck // deprecated field is intentionally still honoured
		return true
	}
	if job.Spec.Access == nil {
		return false
	}
	return len(job.Spec.Access.ReaderGroups) > 0 || len(job.Spec.Access.AdminGroups) > 0
}

type accessDecision struct {
	Role        accessRole
	CanViewLogs bool
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

func (d accessDecision) has(permission string) bool {
	return slices.Contains(d.permissions(), permission)
}

func adminDecision() accessDecision {
	return accessDecision{Role: roleAdmin, CanViewLogs: true}
}

type effectiveAccess struct {
	readerGroups      []string
	adminGroups       []string
	readerUsers       []string
	adminUsers        []string
	anonymousRead     bool
	anonymousReadLogs bool
}

func resolveEffectiveAccess(job *api.RenovateJob, defaults AccessDefaults) effectiveAccess {
	eff := effectiveAccess{
		readerGroups:      defaults.ReaderGroups,
		adminGroups:       defaults.AdminGroups,
		readerUsers:       defaults.ReaderUsers,
		adminUsers:        defaults.AdminUsers,
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
	if len(access.ReaderUsers) > 0 {
		eff.readerUsers = normalizeGroups(access.ReaderUsers)
	}
	if len(access.AdminUsers) > 0 {
		eff.adminUsers = normalizeGroups(access.AdminUsers)
	}
	if access.AnonymousRead != nil {
		eff.anonymousRead = *access.AnonymousRead
	}
	if access.AnonymousReadLogs != nil {
		eff.anonymousReadLogs = *access.AnonymousReadLogs
	}

	return eff
}

// conflictingAccessLogged remembers the jobs whose conflicting access
// configuration has already been reported, keyed by namespace/name. Entries are
// never removed: the set is bounded by the number of RenovateJobs, and a job that
// stops conflicting stops reaching the log anyway.
var conflictingAccessLogged sync.Map

// resolveAccess evaluates a session against a job's effective access
// configuration. A nil session represents a request without authentication,
// which only anonymous read can satisfy.
func resolveAccess(job *api.RenovateJob, session *sessionData, defaults AccessDefaults, logger logr.Logger) accessDecision {
	if job == nil {
		return accessDecision{}
	}

	// Authorization disabled: authentication alone decides, so anyone who got
	// past the login is an admin and no group or user rule is evaluated.
	// Requests without a session fall through, because anonymous read is the one
	// rule that answers a question authentication cannot: what someone who never
	// logs in may see.
	if defaults.AuthorizationDisabled && session != nil {
		return adminDecision()
	}

	// Normally unreachable: the CRD's CEL rule rejects this combination. It is
	// still possible when the CRD is managed outside the chart and has not been
	// upgraded, in which case there is no safe way to guess which surface wins.
	if len(job.Spec.AllowedGroups) > 0 && job.Spec.Access != nil { //nolint:staticcheck // deprecated field is intentionally still honoured
		// Once per job, not once per request: this runs on the endpoints the
		// dashboard polls, so logging every time would drown the log in repeats
		// of a message that only needs saying once.
		if _, seen := conflictingAccessLogged.LoadOrStore(job.Namespace+"/"+job.Name, struct{}{}); !seen {
			logger.Error(nil, "access rules ignored, job treated as inaccessible: allowedGroups and access are mutually exclusive",
				"resource", job.Name,
				"namespace", job.Namespace)
		}
		return accessDecision{}
	}

	eff := resolveEffectiveAccess(job, defaults)

	var userGroups []string
	if session != nil {
		userGroups = normalizeGroups(session.Groups)
	}
	identities := session.identities()

	switch {
	case hasIntersection(identities, eff.adminUsers),
		hasIntersection(userGroups, eff.adminGroups):
		return accessDecision{Role: roleAdmin, CanViewLogs: true}
	case hasIntersection(identities, eff.readerUsers),
		hasIntersection(userGroups, eff.readerGroups):
		return accessDecision{Role: roleReader, CanViewLogs: true}
	case eff.anonymousRead:
		// Renovate logs are unredacted, so anonymous readers need a second opt-in.
		return accessDecision{Role: roleReader, CanViewLogs: eff.anonymousReadLogs}
	}

	return accessDecision{}
}
