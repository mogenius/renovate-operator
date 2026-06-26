package scheduler

import (
	"context"
	"renovate-operator/health"
	"renovate-operator/metricStore"
	"sync"
	"time"

	"github.com/go-logr/logr"
	cron "github.com/netresearch/go-cron"
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
	// Adds a new schedule for the given RenovateJob (namespace/job identify it and
	// label its metrics) with the given cron expression and function to execute.
	AddSchedule(expr string, namespace, job string, fn func()) error
	// Adds a new schedule, replacing any existing schedule for the same RenovateJob.
	AddScheduleReplaceExisting(expr string, namespace, job string, fn func()) error
	// Removes a schedule for the given RenovateJob.
	RemoveSchedule(namespace, job string)
	// Gets the next run time for a cron schedule expression.
	GetNextRunOnSchedule(schedule string) time.Time
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

// scheduleName builds the internal schedule key from a RenovateJob's namespace and
// name. It matches RenovateJob.Fullname() ("${name}-${namespace}") so the health-map
// key and cron registration are unchanged from earlier behavior.
func scheduleName(namespace, job string) string {
	return job + "-" + namespace
}

// Adds a new schedule, does NOT cleanly remove existing ones with the same name
func (s *scheduler) AddSchedule(expr string, namespace, job string, fn func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := scheduleName(namespace, job)
	id, err := s.cronManager.AddFunc(expr, s.execute(name, expr, namespace, job, fn))
	if err != nil {
		return err
	}
	s.entries[name] = schedulerEntry{
		entryId:  id,
		schedule: expr,
	}
	nextRun := s.GetNextRunOnSchedule(expr)
	// setting health status
	s.health.SetSchedulerHealth(func(e *health.SchedulerHealth) *health.SchedulerHealth {
		e.Scheduler[name] = health.SingleSchedulerHealth{
			Name:      name,
			NextRun:   nextRun,
			Schedule:  expr,
			IsRunning: true,
		}
		return e
	})
	// emit next planned run metric (Group D); AddScheduleReplaceExisting delegates here.
	if !nextRun.IsZero() {
		metricStore.SetScheduleNextRun(namespace, job, float64(nextRun.Unix()))
	}
	return nil
}

// Adds a new schedule, if one with the same name already exists, it will be replaced
func (s *scheduler) AddScheduleReplaceExisting(expr string, namespace, job string, fn func()) error {
	name := scheduleName(namespace, job)
	s.mu.Lock()
	entry, exists := s.entries[name]
	s.mu.Unlock()

	if exists {
		if entry.schedule == expr {
			return nil // Schedule already exists with the same expression
		}
		// If the schedule exists but with a different expression, remove it first
		s.RemoveSchedule(namespace, job)
	}
	return s.AddSchedule(expr, namespace, job, fn)
}
func (s *scheduler) RemoveSchedule(namespace, job string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := scheduleName(namespace, job)
	if entry, ok := s.entries[name]; ok {
		s.cronManager.Remove(entry.entryId)
		delete(s.entries, name)
	}
	s.health.SetSchedulerHealth(func(e *health.SchedulerHealth) *health.SchedulerHealth {
		delete(e.Scheduler, name)
		return e
	})

}

func (s *scheduler) GetNextRunOnSchedule(schedule string) time.Time {
	parser, err := cron.TryNewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if err != nil {
		return time.Time{}
	}
	sched, err := parser.Parse(schedule)
	if err != nil {
		return time.Time{}
	}
	return sched.Next(time.Now())
}

// execute the cron expression while also adapting the health status in this time.
// key is the internal schedule/health key; ns and job are the RenovateJob's
// namespace and name, used as metric labels.
func (s *scheduler) execute(key string, schedule string, ns, job string, fn func()) func() {
	return func() {
		// The scheduled function is a bare func() with no error return, so the only
		// failure signal available without changing the public Scheduler signature is
		// a panic. Treat a panic as an "error" run and everything else as "success".
		ctx := context.Background()

		s.health.SetSchedulerHealth(func(e *health.SchedulerHealth) *health.SchedulerHealth {
			e.Scheduler[key] = health.SingleSchedulerHealth{
				Name:      key,
				NextRun:   s.GetNextRunOnSchedule(schedule),
				Schedule:  schedule,
				IsRunning: true,
			}
			return e
		})

		nextRun := s.GetNextRunOnSchedule(schedule)
		defer s.health.SetSchedulerHealth(func(e *health.SchedulerHealth) *health.SchedulerHealth {
			e.Scheduler[key] = health.SingleSchedulerHealth{
				Name:      key,
				NextRun:   nextRun,
				Schedule:  schedule,
				IsRunning: false,
			}
			return e
		})
		// refresh next planned run metric (Group D) as the schedule fires.
		if !nextRun.IsZero() {
			metricStore.SetScheduleNextRun(ns, job, float64(nextRun.Unix()))
		}

		s.logger.Info("executing schedule", "schedule", key)
		defer func() {
			if r := recover(); r != nil {
				metricStore.IncScheduleRun(ctx, ns, job, "error")
				panic(r) // preserve existing panic-propagation behavior
			}
			metricStore.IncScheduleRun(ctx, ns, job, "success")
		}()
		fn()
		s.logger.Info("schedule executed", "schedule", key)
	}
}
