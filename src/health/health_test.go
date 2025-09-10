package health

import (
	"testing"
)

func TestSetExecutorHealth(t *testing.T) {
	h := NewHealthCheck()
	h.SetExecutorHealth(func(e *ExecutorHealth) *ExecutorHealth {
		e.Running = true
		return e
	})
	if !h.GetHealth().Executor.Running {
		t.Error("Executor health not set to running")
	}
}

func TestSetSchedulerHealth(t *testing.T) {
	h := NewHealthCheck()
	h.SetSchedulerHealth(func(s *SchedulerHealth) *SchedulerHealth {
		s.Running = true
		return s
	})
	if !h.GetHealth().Scheduler.Running {
		t.Error("Scheduler health not set to running")
	}
}
