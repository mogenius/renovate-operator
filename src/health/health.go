package health

import (
	"maps"
	"sync"
	"time"
)

type SchedulerHealth struct {
	Running   bool                             `json:"running"`
	Scheduler map[string]SingleSchedulerHealth `json:"scheduler"`
}
type SingleSchedulerHealth struct {
	Name       string    `json:"name"`
	NextRun    time.Time `json:"nextRun"`
	Schedule   string    `json:"schedule"`
	LastUpdate time.Time `json:"lastUpdate"`
	IsRunning  bool      `json:"isRunning"`
}

type ExecutorHealth struct {
	Running  bool                            `json:"running"`
	Executor map[string]SingleExecutorHealth `json:"executor"`
}
type SingleExecutorHealth struct {
	IsRunning  bool      `json:"isRunning"`
	LastUpdate time.Time `json:"lastUpdate"`
}
type ApplicationHealth struct {
	Scheduler SchedulerHealth `json:"scheduler"`
	Executor  ExecutorHealth  `json:"executor"`
	Healthy   bool            `json:"healthy"`
}

type HealthCheck interface {
	GetHealth() *ApplicationHealth
	SetExecutorHealth(func(health *ExecutorHealth) *ExecutorHealth)
	SetSchedulerHealth(func(health *SchedulerHealth) *SchedulerHealth)
}
type healthcheck struct {
	mu     sync.RWMutex
	health *ApplicationHealth
}

func NewHealthCheck() HealthCheck {
	return &healthcheck{
		health: &ApplicationHealth{
			Scheduler: SchedulerHealth{
				Running:   false,
				Scheduler: make(map[string]SingleSchedulerHealth),
			},
			Executor: ExecutorHealth{
				Running:  false,
				Executor: make(map[string]SingleExecutorHealth),
			},
			Healthy: false,
		},
	}
}

func (h *healthcheck) GetHealth() *ApplicationHealth {
	h.mu.RLock()
	defer h.mu.RUnlock()
	cp := *h.health
	cp.Scheduler.Scheduler = maps.Clone(h.health.Scheduler.Scheduler)
	cp.Executor.Executor = maps.Clone(h.health.Executor.Executor)
	return &cp
}

func (h *healthcheck) SetExecutorHealth(fn func(health *ExecutorHealth) *ExecutorHealth) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e := *fn(&h.health.Executor)
	lastUpdate := time.Now()
	for key := range e.Executor {
		entry := e.Executor[key]
		entry.LastUpdate = lastUpdate
		e.Executor[key] = entry
	}
	h.health.Executor = e
}

func (h *healthcheck) SetSchedulerHealth(fn func(health *SchedulerHealth) *SchedulerHealth) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := *fn(&h.health.Scheduler)
	lastUpdate := time.Now()
	for key := range s.Scheduler {
		entry := s.Scheduler[key]
		entry.LastUpdate = lastUpdate
		s.Scheduler[key] = entry
	}
	h.health.Scheduler = s
}
