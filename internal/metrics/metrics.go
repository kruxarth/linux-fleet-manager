package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	Nodes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "fleet_nodes",
		Help: "Fleet nodes by effective state",
	}, []string{"state"})

	Heartbeats = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fleet_heartbeats_total",
		Help: "Accepted agent heartbeats",
	})

	Commands = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fleet_commands_total",
		Help: "Commands by terminal or enqueue status",
	}, []string{"status"})

	CommandDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "fleet_command_duration_seconds",
		Help:    "Reported command duration from agents",
		Buckets: prometheus.DefBuckets,
	})

	Enrollments = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fleet_enrollments_total",
		Help: "Enrollment attempts",
	}, []string{"result"})
)

func SetNodeCounts(m map[string]int) {
	for state, n := range m {
		Nodes.WithLabelValues(state).Set(float64(n))
	}
}
