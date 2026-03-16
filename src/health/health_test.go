package health

import (
	"sync"
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

func TestConcurrentSchedulerHealthUpdates(t *testing.T) {
	h := NewHealthCheck()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		name := "job-" + string(rune('a'+i%26))
		go func() {
			defer wg.Done()
			h.SetSchedulerHealth(func(s *SchedulerHealth) *SchedulerHealth {
				s.Scheduler[name] = SingleSchedulerHealth{
					Name:      name,
					IsRunning: true,
				}
				return s
			})
		}()
	}
	wg.Wait()
}

func TestConcurrentExecutorHealthUpdates(t *testing.T) {
	h := NewHealthCheck()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		name := "job-" + string(rune('a'+i%26))
		go func() {
			defer wg.Done()
			h.SetExecutorHealth(func(e *ExecutorHealth) *ExecutorHealth {
				e.Executor[name] = SingleExecutorHealth{
					IsRunning: true,
				}
				return e
			})
		}()
	}
	wg.Wait()
}
