package v1alpha1

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// PullRequestTriggerEvent is a pull/merge request event that can trigger a
// Renovate run. Platforms that lack an event never send it.
// +kubebuilder:validation:Enum=edited;merged;closed;reopened
type PullRequestTriggerEvent string

const (
	// a Renovate checkbox was ticked in the description
	PullRequestEdited PullRequestTriggerEvent = "edited"
	// any pull/merge request was merged
	PullRequestMerged PullRequestTriggerEvent = "merged"
	// a Renovate pull/merge request was closed without merging
	PullRequestClosed PullRequestTriggerEvent = "closed"
	// a Renovate pull request was reopened (Gitea, Forgejo)
	PullRequestReopened PullRequestTriggerEvent = "reopened"
)

// IssueTriggerEvent is an issue (Dependency Dashboard) event that can trigger
// a Renovate run.
// +kubebuilder:validation:Enum=edited;closed
type IssueTriggerEvent string

const (
	// a checkbox was ticked in the Dependency Dashboard
	IssueEdited IssueTriggerEvent = "edited"
	// the Dependency Dashboard was closed with a checkbox ticked (GitHub)
	IssueClosed IssueTriggerEvent = "closed"
)

// WebhookTriggerEvents selects the platform events that trigger a Renovate
// run. An omitted list triggers on none of its events.
type WebhookTriggerEvents struct {
	// +optional
	PullRequest []PullRequestTriggerEvent `json:"pullRequest,omitempty"`
	// +optional
	Issues []IssueTriggerEvent `json:"issues,omitempty"`
}

var (
	allPullRequestTriggerEvents = []PullRequestTriggerEvent{PullRequestEdited, PullRequestMerged, PullRequestClosed, PullRequestReopened}
	allIssueTriggerEvents       = []IssueTriggerEvent{IssueEdited, IssueClosed}
)

// DefaultWebhookTriggerEvents returns every event, the behavior before
// triggers became configurable.
func DefaultWebhookTriggerEvents() WebhookTriggerEvents {
	return WebhookTriggerEvents{
		PullRequest: slices.Clone(allPullRequestTriggerEvents),
		Issues:      slices.Clone(allIssueTriggerEvents),
	}
}

// ParseWebhookTriggerEvents reads the WEBHOOK_TRIGGER_ON_EVENT value, a JSON
// object such as {"pullRequest":["edited"],"issues":["edited"]}. Empty means
// every event.
func ParseWebhookTriggerEvents(value string) (WebhookTriggerEvents, error) {
	if value == "" {
		return DefaultWebhookTriggerEvents(), nil
	}
	var events WebhookTriggerEvents
	dec := json.NewDecoder(strings.NewReader(value))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&events); err != nil {
		return events, fmt.Errorf(`expected a JSON object like {"pullRequest":["edited"],"issues":["edited"]}: %w`, err)
	}
	return events, events.Validate()
}

// Validate rejects unknown event names. The CRD enforces the same through its
// enums; this covers the WEBHOOK_TRIGGER_ON_EVENT environment variable.
func (e WebhookTriggerEvents) Validate() error {
	for _, ev := range e.PullRequest {
		if !slices.Contains(allPullRequestTriggerEvents, ev) {
			return fmt.Errorf("unknown pullRequest event %q, expected one of %v", ev, allPullRequestTriggerEvents)
		}
	}
	for _, ev := range e.Issues {
		if !slices.Contains(allIssueTriggerEvents, ev) {
			return fmt.Errorf("unknown issues event %q, expected one of %v", ev, allIssueTriggerEvents)
		}
	}
	return nil
}
