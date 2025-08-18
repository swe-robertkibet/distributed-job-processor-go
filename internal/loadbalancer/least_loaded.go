package loadbalancer

import (
	"distributed-job-processor/internal/worker"
	"distributed-job-processor/pkg/job"
)

type LeastLoadedBalancer struct{}

func NewLeastLoadedBalancer() *LeastLoadedBalancer {
	return &LeastLoadedBalancer{}
}

func (l *LeastLoadedBalancer) SelectWorker(workers []worker.WorkerStats, job *job.Job) *worker.WorkerStats {
	if len(workers) == 0 {
		return nil
	}

	var selected *worker.WorkerStats
	minLoad := int64(-1)

	for i := range workers {
		w := &workers[i]
		if !w.IsActive && (minLoad == -1 || w.JobCount < minLoad) {
			minLoad = w.JobCount
			selected = w
		}
	}

	return selected
}

func (l *LeastLoadedBalancer) GetStrategy() string {
	return "least_loaded"
}