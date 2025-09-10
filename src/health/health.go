package health

import "time"

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
	return h.health
}

func (h *healthcheck) SetExecutorHealth(fn func(health *ExecutorHealth) *ExecutorHealth) {
	e := *fn(&h.health.Executor)
	lastUpdate := time.Now()
	for key := range e.Executor {
		s := e.Executor[key]
		s.LastUpdate = lastUpdate
		e.Executor[key] = s
	}
	h.health.Executor = e
}

func (h *healthcheck) SetSchedulerHealth(fn func(health *SchedulerHealth) *SchedulerHealth) {
	e := *fn(&h.health.Scheduler)
	lastUpdate := time.Now()
	for key := range e.Scheduler {
		s := e.Scheduler[key]
		s.LastUpdate = lastUpdate
		e.Scheduler[key] = s
	}
	h.health.Scheduler = e
}
