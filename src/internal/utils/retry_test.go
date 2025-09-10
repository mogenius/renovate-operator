package utils

import (
	"errors"
	"testing"
	"time"
)

func TestRetrySuccess(t *testing.T) {
	calls := 0
	err := Retry(3, time.Millisecond, func() error {
		calls++
		if calls < 2 {
			return errors.New("fail")
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Errorf("expected 2 calls and nil error, got %d, %v", calls, err)
	}
}

func TestRetryFail(t *testing.T) {
	calls := 0
	err := Retry(2, time.Millisecond, func() error {
		calls++
		return errors.New("fail")
	})
	if err == nil || calls != 2 {
		t.Errorf("expected 2 calls and error, got %d, %v", calls, err)
	}
}
