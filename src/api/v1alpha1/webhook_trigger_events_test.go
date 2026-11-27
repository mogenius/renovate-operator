package v1alpha1

import (
	"reflect"
	"testing"
)

func TestParseWebhookTriggerEvents(t *testing.T) {
	tests := []struct {
		value   string
		want    WebhookTriggerEvents
		wantErr bool
	}{
		{value: "", want: DefaultWebhookTriggerEvents()},
		{value: `{}`, want: WebhookTriggerEvents{}},
		{value: `{"pullRequest":["edited","merged"],"issues":["edited"]}`, want: WebhookTriggerEvents{
			PullRequest: []PullRequestTriggerEvent{PullRequestEdited, PullRequestMerged},
			Issues:      []IssueTriggerEvent{IssueEdited},
		}},
		{value: `{"pullRequest":["pushed"]}`, wantErr: true},
		{value: `{"issues":["merged"]}`, wantErr: true},
		{value: `{"push":["defaultBranch"]}`, wantErr: true},
		{value: `pullRequest.merged`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := ParseWebhookTriggerEvents(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
