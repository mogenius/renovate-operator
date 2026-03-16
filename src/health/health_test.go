package health

import (
	"fmt"
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

func TestConcurrentSchedulerHealthWrites(t *testing.T) {
	h := NewHealthCheck()

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)

	for i := range n {
		go func(id int) {
			defer wg.Done()
			h.SetSchedulerHealth(func(s *SchedulerHealth) *SchedulerHealth {
				s.Scheduler[fmt.Sprintf("repo-%d", id)] = SingleSchedulerHealth{
					Name:      fmt.Sprintf("repo-%d", id),
					Schedule:  "* * * * *",
					IsRunning: true,
				}
				return s
			})
		}(i)
	}

	wg.Wait()

	health := h.GetHealth()
	if len(health.Scheduler.Scheduler) != n {
		t.Errorf("expected %d scheduler entries, got %d", n, len(health.Scheduler.Scheduler))
	}
}

func TestConcurrentExecutorHealthWrites(t *testing.T) {
	h := NewHealthCheck()

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)

	for i := range n {
		go func(id int) {
			defer wg.Done()
			h.SetExecutorHealth(func(e *ExecutorHealth) *ExecutorHealth {
				e.Executor[fmt.Sprintf("exec-%d", id)] = SingleExecutorHealth{
					IsRunning: true,
				}
				return e
			})
		}(i)
	}

	wg.Wait()

	health := h.GetHealth()
	if len(health.Executor.Executor) != n {
		t.Errorf("expected %d executor entries, got %d", n, len(health.Executor.Executor))
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	h := NewHealthCheck()

	var wg sync.WaitGroup
	wg.Add(300)

	for i := range 100 {
		go func(id int) {
			defer wg.Done()
			h.SetSchedulerHealth(func(s *SchedulerHealth) *SchedulerHealth {
				s.Scheduler[fmt.Sprintf("repo-%d", id)] = SingleSchedulerHealth{
					Name:      fmt.Sprintf("repo-%d", id),
					IsRunning: true,
				}
				return s
			})
		}(i)

		go func(id int) {
			defer wg.Done()
			h.SetExecutorHealth(func(e *ExecutorHealth) *ExecutorHealth {
				e.Executor[fmt.Sprintf("exec-%d", id)] = SingleExecutorHealth{
					IsRunning: true,
				}
				return e
			})
		}(i)

		go func() {
			defer wg.Done()
			health := h.GetHealth()
			// Iterate both maps like json.Encode would
			for range health.Scheduler.Scheduler {
			}
			for range health.Executor.Executor {
			}
		}()
	}

	wg.Wait()
}
