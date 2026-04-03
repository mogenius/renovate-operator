package types

import (
	api "renovate-operator/api/v1alpha1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RenovateStatusUpdate struct {
	Status               api.RenovateProjectStatus
	Priority             int32
	RenovateResultStatus *string
	PRActivity           *api.PRActivity
	LogIssues            *api.LogIssues
	LastRun              *v1.Time
	Duration             *string
}
