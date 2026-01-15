package metricStore

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	projectRuns = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "renovate_operator_project_executions_total",
			Help: "Total number of executed Renovate projects",
		},
		[]string{"namespace", "job", "project", "status"})
)

func Register(registry ctrlmetrics.RegistererGatherer) {
	registry.MustRegister(projectRuns)
}

func CaptureRenovateProjectExecution(namespace, job, project, status string) {
	projectRuns.WithLabelValues(
		namespace,
		job,
		project,
		status,
	).Inc()
}
