package webhook

// WebhookBody holds the description / body of the changed issue or PR.
// Previous is only populated by providers that send diff payloads (GitLab).
type WebhookBody struct {
	Current  string
	Previous string
}

// WebhookPayload is the normalized, provider-agnostic representation of an
// incoming webhook event. Provider-specific parse functions fill this struct;
// isValidWebhookPayload applies shared validation, and handleWebhookTrigger
// handles the scheduling logic.
type WebhookPayload struct {
	Provider  string      // display name for log messages ("GitHub", "GitLab", …)
	Project   string      // repository full name (owner/repo or namespace/repo)
	Event     string      // raw event-type header value; empty when not applicable
	Action    string      // normalized action: "edited", "closed", or "reopened"
	EventType string      // normalized object kind: "issue" or "pull_request"
	Body      WebhookBody // description / body of the issue or PR
}

// isValidWebhookPayload validates the normalized payload using shared rules.
// Structural checks (nil guards, unsupported event types, action name mapping)
// are handled by each provider's parse function before this call.
func isValidWebhookPayload(p WebhookPayload) (bool, string) {
	switch p.EventType {
	case "issue":
		if p.Action != "edited" {
			return false, "issue action is not edited"
		}
		if p.Body.Current == "" {
			return false, "no issue body"
		}
		if !verifyRenovateDescriptionChange(p.Body.Current) {
			return false, "not a valid renovate checkbox change"
		}
		return true, ""

	case "pull_request":
		if p.Body.Current == "" {
			return false, "no pull request body"
		}
		if !isRenovateContent(p.Body.Current) {
			return false, "not a Renovate pull request"
		}
		switch p.Action {
		case "closed", "reopened":
			return true, ""
		case "edited":
			if !hasCheckboxBeenChecked(p.Body.Current) {
				return false, "no checked checkbox"
			}
			return true, ""
		default:
			return false, "pull request action is not edited, closed, or reopened"
		}

	default:
		return false, "unsupported event type: " + p.EventType
	}
}
