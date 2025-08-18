package metrics

import (
	"net/http"
	"time"

	"distributed-job-processor/internal/logger"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	JobsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "jobs_total",
			Help: "Total number of jobs processed",
		},
		[]string{"status", "type"},
	)

	JobDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "job_duration_seconds",
			Help:    "Job processing duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"type"},
	)

	WorkersActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "workers_active",
			Help: "Number of active workers",
		},
		[]string{"node_id"},
	)

	QueueSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "queue_size",
			Help: "Number of jobs in queue",
		},
		[]string{"queue_type"},
	)

	LeaderElections = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "leader_elections_total",
			Help: "Total number of leader elections",
		},
		[]string{"node_id", "result"},
	)

	NodeInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "node_info",
			Help: "Node information",
		},
		[]string{"node_id", "version", "is_leader"},
	)

	RetryAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "retry_attempts_total",
			Help: "Total number of job retry attempts",
		},
		[]string{"type", "attempt"},
	)
)

func init() {
	prometheus.MustRegister(JobsTotal)
	prometheus.MustRegister(JobDuration)
	prometheus.MustRegister(WorkersActive)
	prometheus.MustRegister(QueueSize)
	prometheus.MustRegister(LeaderElections)
	prometheus.MustRegister(NodeInfo)
	prometheus.MustRegister(RetryAttempts)
}

func StartMetricsServer(port string) {
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/health", healthHandler)
	
	logger.WithField("port", port).Info("Starting metrics server")
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.WithError(err).Fatal("Failed to start metrics server")
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func RecordJobStart(jobType string) {
	JobsTotal.WithLabelValues("started", jobType).Inc()
}

func RecordJobCompletion(jobType string, duration time.Duration) {
	JobsTotal.WithLabelValues("completed", jobType).Inc()
	JobDuration.WithLabelValues(jobType).Observe(duration.Seconds())
}

func RecordJobFailure(jobType string) {
	JobsTotal.WithLabelValues("failed", jobType).Inc()
}

func RecordJobRetry(jobType string, attempt int) {
	RetryAttempts.WithLabelValues(jobType, string(rune(attempt))).Inc()
}

func UpdateWorkerCount(nodeID string, count float64) {
	WorkersActive.WithLabelValues(nodeID).Set(count)
}

func UpdateQueueSize(queueType string, size float64) {
	QueueSize.WithLabelValues(queueType).Set(size)
}

func RecordLeaderElection(nodeID, result string) {
	LeaderElections.WithLabelValues(nodeID, result).Inc()
}

func UpdateNodeInfo(nodeID, version string, isLeader bool) {
	leaderStr := "false"
	if isLeader {
		leaderStr = "true"
	}
	NodeInfo.WithLabelValues(nodeID, version, leaderStr).Set(1)
}