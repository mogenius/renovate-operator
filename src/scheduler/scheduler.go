package scheduler

import (
	"renovate-operator/health"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
)

/*
Scheduler is the interface for scheduling periodic tasks using cron expressions.
It allows adding, removing, and querying scheduled tasks.
*/
type Scheduler interface {
	// Starts the scheduler.
	Start()
	// Stops the scheduler.
	Stop()
	// Adds a new schedule with the given cron expression, name, and function to execute.
	AddSchedule(expr string, name string, fn func()) error
	// Adds a new schedule, replacing any existing schedule with the same name.
	AddScheduleReplaceExisting(expr string, name string, fn func()) error
	// Removes a schedule by name.
	RemoveSchedule(name string)
	// Gets the next run time for a schedule by name.
	GetNextRun(name string) time.Time
}

type scheduler struct {
	cronManager *cron.Cron
	entries     map[string]schedulerEntry
	mu          sync.RWMutex
	health      health.HealthCheck
	logger      logr.Logger
}

type schedulerEntry struct {
	entryId  cron.EntryID
	schedule string
}

func NewScheduler(logger logr.Logger, health health.HealthCheck) Scheduler {
	cronManager := cron.New(
		cron.WithLogger(logger))
	return &scheduler{
		cronManager: cronManager,
		entries:     make(map[string]schedulerEntry),
		health:      health,
		mu:          sync.RWMutex{},
		logger:      logger,
	}
}

func (s *scheduler) Start() {
	defer s.health.SetSchedulerHealth(func(e *health.SchedulerHealth) *health.SchedulerHealth {
		e.Running = true
		return e
	})
	s.cronManager.Start()
}

func (s *scheduler) Stop() {
	defer s.health.SetSchedulerHealth(func(e *health.SchedulerHealth) *health.SchedulerHealth {
		e.Running = false
		return e
	})
	s.cronManager.Stop()
}

// Adds a new schedule, does NOT cleanly remove existing ones with the same name
func (s *scheduler) AddSchedule(expr string, name string, fn func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, err := s.cronManager.AddFunc(expr, s.execute(name, expr, fn))
	if err != nil {
		return err
	}
	s.entries[name] = schedulerEntry{
		entryId:  id,
		schedule: expr,
	}
	// setting health status
	s.health.SetSchedulerHealth(func(e *health.SchedulerHealth) *health.SchedulerHealth {
		e.Scheduler[name] = health.SingleSchedulerHealth{
			Name:      name,
			NextRun:   s.getNextRunInternal(name),
			Schedule:  expr,
			IsRunning: true,
		}
		return e
	})
	return nil
}

// Adds a new schedule, if one with the same name already exists, it will be replaced
func (s *scheduler) AddScheduleReplaceExisting(expr string, name string, fn func()) error {
	s.mu.Lock()
	entry, exists := s.entries[name]
	s.mu.Unlock()

	if exists {
		if entry.schedule == expr {
			return nil // Schedule already exists with the same expression
		}
		// If the schedule exists but with a different expression, remove it first
		s.RemoveSchedule(name)
	}
	return s.AddSchedule(expr, name, fn)
}
func (s *scheduler) RemoveSchedule(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.entries[name]; ok {
		s.cronManager.Remove(entry.entryId)
		delete(s.entries, name)
	}
	s.health.SetSchedulerHealth(func(e *health.SchedulerHealth) *health.SchedulerHealth {
		delete(e.Scheduler, name)
		return e
	})
}

func (s *scheduler) GetNextRun(name string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getNextRunInternal(name)
}

func (s *scheduler) getNextRunInternal(name string) time.Time {
	if entry, ok := s.entries[name]; ok {
		// Find the entry with matching ID
		for _, cronEntry := range s.cronManager.Entries() {
			if cronEntry.ID == entry.entryId {
				return cronEntry.Next
			}
		}
	}
	return time.Time{} // return zero time if not found
}

// execute the cron expression while also adapting the health status in this time
func (s *scheduler) execute(key string, schedule string, fn func()) func() {
	return func() {
		s.health.SetSchedulerHealth(func(e *health.SchedulerHealth) *health.SchedulerHealth {
			e.Scheduler[key] = health.SingleSchedulerHealth{
				Name:      key,
				NextRun:   s.getNextRunInternal(key),
				Schedule:  schedule,
				IsRunning: true,
			}
			return e
		})

		defer s.health.SetSchedulerHealth(func(e *health.SchedulerHealth) *health.SchedulerHealth {
			e.Scheduler[key] = health.SingleSchedulerHealth{
				Name:      key,
				NextRun:   s.getNextRunInternal(key),
				Schedule:  schedule,
				IsRunning: false,
			}
			return e
		})

		s.logger.Info("executing schedule", "schedule", key)
		fn()
		s.logger.Info("schedule executed", "schedule", key)
	}
}
