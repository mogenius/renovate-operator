package utils

import "time"

// DefaultRetryAttempts is the default number of retry attempts for operations
var DefaultRetryAttempts = 5

// DefaultRetrySleep is the default sleep duration between retry attempts
var DefaultRetrySleep = time.Second

func Retry(attempts int, sleep time.Duration, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		time.Sleep(sleep)
		sleep *= 2 // exponential backoff
	}
	return err
}
