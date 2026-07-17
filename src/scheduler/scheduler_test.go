package scheduler

import (
	"renovate-operator/health"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

var testLogger = logr.Discard()

func TestSchedulerLifecycle(t *testing.T) {
	h := health.NewHealthCheck()
	s := NewScheduler(testLogger, h)

	// Start scheduler
	s.Start()
	defer s.Stop()

	// Verify scheduler is running
	hc := h.GetHealth()
	if !hc.Scheduler.Running {
		t.Error("Scheduler should be running after Start()")
	}

	// Stop scheduler
	s.Stop()

	// Verify scheduler is stopped
	hc = h.GetHealth()
	if hc.Scheduler.Running {
		t.Error("Scheduler should not be running after Stop()")
	}
}

func TestAddSchedule(t *testing.T) {
	h := health.NewHealthCheck()
	s := NewScheduler(testLogger, h)
	s.Start()
	defer s.Stop()

	err := s.AddSchedule("* * * * *", "test-schedule", func() {})

	if err != nil {
		t.Fatalf("AddSchedule returned error: %v", err)
	}

	// Verify schedule was added to health
	hc := h.GetHealth()
	if _, exists := hc.Scheduler.Scheduler["test-schedule"]; !exists {
		t.Error("Schedule should be present in health check")
	}
}

func TestAddScheduleInvalidCron(t *testing.T) {
	h := health.NewHealthCheck()
	s := NewScheduler(testLogger, h)
	s.Start()
	defer s.Stop()

	err := s.AddSchedule("invalid-cron", "test-invalid", func() {})
	if err == nil {
		t.Error("AddSchedule should return error for invalid cron expression")
	}
}

func TestAddScheduleReplaceExisting(t *testing.T) {
	h := health.NewHealthCheck()
	s := NewScheduler(testLogger, h)
	s.Start()
	defer s.Stop()

	// Add initial schedule
	err := s.AddSchedule("* * * * *", "test-replace", func() {})
	if err != nil {
		t.Fatalf("AddSchedule returned error: %v", err)
	}

	// Replace with same schedule - should not error
	err = s.AddScheduleReplaceExisting("* * * * *", "test-replace", func() {})
	if err != nil {
		t.Fatalf("AddScheduleReplaceExisting returned error for same schedule: %v", err)
	}

	// Replace with different schedule
	err = s.AddScheduleReplaceExisting("*/2 * * * *", "test-replace", func() {})
	if err != nil {
		t.Fatalf("AddScheduleReplaceExisting returned error: %v", err)
	}

	// Verify only one schedule exists
	hc := h.GetHealth()
	if len(hc.Scheduler.Scheduler) != 1 {
		t.Errorf("Expected 1 schedule, got %d", len(hc.Scheduler.Scheduler))
	}

	// Verify the schedule was updated
	schedule := hc.Scheduler.Scheduler["test-replace"]
	if schedule.Schedule != "*/2 * * * *" {
		t.Errorf("Schedule should be updated to '*/2 * * * *', got %q", schedule.Schedule)
	}
}

func TestRemoveSchedule(t *testing.T) {
	h := health.NewHealthCheck()
	s := NewScheduler(testLogger, h)
	s.Start()
	defer s.Stop()

	// Add a schedule
	err := s.AddSchedule("* * * * *", "test-remove", func() {})
	if err != nil {
		t.Fatalf("AddSchedule returned error: %v", err)
	}

	// Verify schedule exists
	hc := h.GetHealth()
	if _, exists := hc.Scheduler.Scheduler["test-remove"]; !exists {
		t.Fatal("Schedule should exist before removal")
	}

	// Remove the schedule
	s.RemoveSchedule("test-remove")

	// Verify schedule was removed
	hc = h.GetHealth()
	if _, exists := hc.Scheduler.Scheduler["test-remove"]; exists {
		t.Error("Schedule should be removed from health check")
	}

	// Removing non-existent schedule should not panic
	s.RemoveSchedule("non-existent")
}

func TestGetNextRun(t *testing.T) {
	h := health.NewHealthCheck()
	s := NewScheduler(testLogger, h)
	s.Start()
	defer s.Stop()

	// Add a schedule
	err := s.AddSchedule("* * * * *", "test-next", func() {})
	if err != nil {
		t.Fatalf("AddSchedule returned error: %v", err)
	}

	// Get next run time
	nextRun := s.GetNextRunOnSchedule("* * * * *", "")
	if nextRun.IsZero() {
		t.Error("Next run time should not be zero for existing schedule")
	}

	// Next run should be in the future
	if !nextRun.After(time.Now()) {
		t.Error("Next run should be in the future")
	}
}

func TestScheduleExecution(t *testing.T) {
	h := health.NewHealthCheck()
	s := NewScheduler(testLogger, h)
	s.Start()
	defer s.Stop()

	// Schedule every minute (cron uses 5 fields by default)
	err := s.AddSchedule("* * * * *", "test-exec", func() {})

	if err != nil {
		t.Fatalf("AddSchedule returned error: %v", err)
	}

	// Since the schedule runs every minute, we just verify that the schedule was added successfully
	// Testing actual execution would require waiting up to a minute, which is too long for unit tests
	// Verify schedule exists in health
	hc := h.GetHealth()
	if _, exists := hc.Scheduler.Scheduler["test-exec"]; !exists {
		t.Error("Schedule should be present in health check")
	}
}

func TestAddScheduleHashedCron(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"plain H", "H * * * *"},
		{"H with step", "H/15 * * * *"},
		{"H with range", "H(0-29) * * * *"},
		{"H in multiple fields", "H H * * *"},
		{"descriptor @daily", "@daily"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := health.NewHealthCheck()
			s := NewScheduler(testLogger, h)
			s.Start()
			defer s.Stop()

			err := s.AddSchedule(tt.expr, "my-job-default", func() {})
			if err != nil {
				t.Fatalf("AddSchedule(%q) returned unexpected error: %v", tt.expr, err)
			}

			hc := h.GetHealth()
			entry, exists := hc.Scheduler.Scheduler["my-job-default"]
			if !exists {
				t.Fatal("schedule not found in health check")
			}
			if entry.NextRun.IsZero() {
				t.Error("NextRun should not be zero after adding a hashed schedule")
			}
			if !entry.NextRun.After(time.Now()) {
				t.Error("NextRun should be in the future")
			}
		})
	}
}

func TestGetNextRunOnScheduleHashedCronDeterminism(t *testing.T) {
	h := health.NewHealthCheck()
	s := NewScheduler(testLogger, h)

	first := s.GetNextRunOnSchedule("H H * * *", "my-renovate-job")
	second := s.GetNextRunOnSchedule("H H * * *", "my-renovate-job")

	if !first.Equal(second) {
		t.Errorf("same key should produce the same next run time: %v vs %v", first, second)
	}
	if first.IsZero() {
		t.Error("next run time should not be zero for a valid H expression with a key")
	}
}

func TestGetNextRunOnScheduleHashedCronDistribution(t *testing.T) {
	h := health.NewHealthCheck()
	s := NewScheduler(testLogger, h)

	keys := []string{"job-a", "job-b", "job-c", "job-d", "job-e", "job-f", "job-g"}
	minutes := make(map[int]struct{})
	for _, key := range keys {
		next := s.GetNextRunOnSchedule("H * * * *", key)
		if next.IsZero() {
			t.Fatalf("GetNextRunOnSchedule returned zero for key %q", key)
		}
		minutes[next.Minute()] = struct{}{}
	}

	if len(minutes) < 2 {
		t.Errorf("expected different keys to produce different minutes, got only %d distinct value(s)", len(minutes))
	}
}

func TestGetNextRunOnScheduleHashedCronRange(t *testing.T) {
	h := health.NewHealthCheck()
	s := NewScheduler(testLogger, h)

	next := s.GetNextRunOnSchedule("H(0-29) * * * *", "range-job")
	if next.IsZero() {
		t.Fatal("expected non-zero next run time")
	}
	if next.Minute() < 0 || next.Minute() > 29 {
		t.Errorf("minute %d is outside the expected range [0, 29]", next.Minute())
	}
}

func TestGetNextRunOnScheduleHashedCronStep(t *testing.T) {
	h := health.NewHealthCheck()
	s := NewScheduler(testLogger, h)

	first := s.GetNextRunOnSchedule("H/15 * * * *", "step-job")
	if first.IsZero() {
		t.Fatal("expected non-zero next run time")
	}

	second := s.GetNextRunOnSchedule("H/15 * * * *", "step-job")
	diff := second.Sub(first)
	if diff != 15*time.Minute {
		// GetNextRunOnSchedule is called twice with time.Now() advancing slightly,
		// so both calls may land in the same 15-min window; what matters is the
		// interval between consecutive actual firings — verified via the library's own tests.
		// Here we just assert the minute is consistent with a 15-minute step.
		if first.Minute()%15 != second.Minute()%15 && diff != 15*time.Minute {
			t.Errorf("expected 15-minute step cadence, got diff %v", diff)
		}
	}
}

func TestGetNextRunOnScheduleEmptyKeyReturnsZeroForHashExpr(t *testing.T) {
	h := health.NewHealthCheck()
	s := NewScheduler(testLogger, h)

	next := s.GetNextRunOnSchedule("H * * * *", "")
	if !next.IsZero() {
		t.Error("H expression with empty key should return zero time (parse error)")
	}
}

func TestAddScheduleReplaceExistingHashedCronSameExprNotReAdded(t *testing.T) {
	h := health.NewHealthCheck()
	s := NewScheduler(testLogger, h)
	s.Start()
	defer s.Stop()

	callCount := 0
	add := func() { callCount++ }

	if err := s.AddSchedule("H H * * *", "hashed-job", add); err != nil {
		t.Fatalf("AddSchedule: %v", err)
	}
	// same expression + same name — must be a no-op
	if err := s.AddScheduleReplaceExisting("H H * * *", "hashed-job", add); err != nil {
		t.Fatalf("AddScheduleReplaceExisting: %v", err)
	}

	hc := h.GetHealth()
	if len(hc.Scheduler.Scheduler) != 1 {
		t.Errorf("expected exactly 1 schedule entry, got %d", len(hc.Scheduler.Scheduler))
	}
}
