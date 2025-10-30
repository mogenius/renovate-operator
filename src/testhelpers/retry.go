package testhelpers

import (
	"testing"
	"time"

	"renovate-operator/internal/utils"
)

// WithShortRetries temporarily sets utils.DefaultRetryAttempts and DefaultRetrySleep
// for the duration of fn and restores the original values afterwards.
func WithShortRetries(t *testing.T, attempts int, sleep time.Duration, fn func()) {
	oldAttempts := utils.DefaultRetryAttempts
	oldSleep := utils.DefaultRetrySleep
	utils.DefaultRetryAttempts = attempts
	utils.DefaultRetrySleep = sleep
	defer func() {
		utils.DefaultRetryAttempts = oldAttempts
		utils.DefaultRetrySleep = oldSleep
	}()
	fn()
}
