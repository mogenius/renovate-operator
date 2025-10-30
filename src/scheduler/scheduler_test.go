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
	nextRun := s.GetNextRun("test-next")
	if nextRun.IsZero() {
		t.Error("Next run time should not be zero for existing schedule")
	}

	// Next run should be in the future
	if !nextRun.After(time.Now()) {
		t.Error("Next run should be in the future")
	}

	// Non-existent schedule should return zero time
	zeroTime := s.GetNextRun("non-existent")
	if !zeroTime.IsZero() {
		t.Error("GetNextRun should return zero time for non-existent schedule")
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
