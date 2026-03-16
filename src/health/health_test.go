package health

import (
	"encoding/json"
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
	t.Parallel()

	h := NewHealthCheck()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		name := "job-" + string(rune('a'+i%26))
		go func(n string) {
			defer wg.Done()
			h.SetSchedulerHealth(func(s *SchedulerHealth) *SchedulerHealth {
				if s.Scheduler == nil {
					s.Scheduler = make(map[string]SingleSchedulerHealth)
				}
				s.Scheduler[n] = SingleSchedulerHealth{
					Name:      n,
					IsRunning: true,
				}
				return s
			})
		}(name)
	}
	wg.Wait()
}

func TestConcurrentExecutorHealthUpdates(t *testing.T) {
	t.Parallel()

	h := NewHealthCheck()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		name := "job-" + string(rune('a'+i%26))
		go func(n string) {
			defer wg.Done()
			h.SetExecutorHealth(func(e *ExecutorHealth) *ExecutorHealth {
				if e.Executor == nil {
					e.Executor = make(map[string]SingleExecutorHealth)
				}
				e.Executor[n] = SingleExecutorHealth{
					IsRunning: true,
				}
				return e
			})
		}(name)
	}
	wg.Wait()
}

func TestConcurrentReadWriteHealth(t *testing.T) {
	t.Parallel()

	h := NewHealthCheck()
	var wg sync.WaitGroup

	// Writers: update scheduler and executor health concurrently
	for i := 0; i < 50; i++ {
		wg.Add(2)
		name := "job-" + string(rune('a'+i%26))
		go func(n string) {
			defer wg.Done()
			h.SetSchedulerHealth(func(s *SchedulerHealth) *SchedulerHealth {
				if s.Scheduler == nil {
					s.Scheduler = make(map[string]SingleSchedulerHealth)
				}
				s.Scheduler[n] = SingleSchedulerHealth{
					Name:      n,
					IsRunning: true,
				}
				return s
			})
		}(name)
		go func(n string) {
			defer wg.Done()
			h.SetExecutorHealth(func(e *ExecutorHealth) *ExecutorHealth {
				if e.Executor == nil {
					e.Executor = make(map[string]SingleExecutorHealth)
				}
				e.Executor[n] = SingleExecutorHealth{
					IsRunning: true,
				}
				return e
			})
		}(name)
	}

	// Readers: simulate what the HTTP health endpoint does (json.Marshal iterates the maps)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snapshot := h.GetHealth()
			// Iterate the returned maps, as json.Encode would in the health endpoint.
			_, _ = json.Marshal(snapshot)
		}()
	}

	wg.Wait()
}
