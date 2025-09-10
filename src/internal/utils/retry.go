package utils

import "time"

const (
	// DefaultRetryAttempts is the default number of retry attempts for operations
	DefaultRetryAttempts = 5
	// DefaultRetrySleep is the default sleep duration between retry attempts
	DefaultRetrySleep = time.Second
)

func Retry(attempts int, sleep time.Duration, fn func() error) error {
	var err error
	for range attempts {
		err = fn()
		if err == nil {
			return nil
		}
		time.Sleep(sleep)
		sleep *= 2 // exponential backoff
	}
	return err
}
