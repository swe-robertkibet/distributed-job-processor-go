package loadbalancer

import (
	"sync"

	"distributed-job-processor/internal/worker"
	"distributed-job-processor/pkg/job"
)

type RoundRobinBalancer struct {
	current int
	mu      sync.Mutex
}

func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{
		current: 0,
	}
}

func (r *RoundRobinBalancer) SelectWorker(workers []worker.WorkerStats, job *job.Job) *worker.WorkerStats {
	if len(workers) == 0 {
		return nil
	}

	availableWorkers := make([]worker.WorkerStats, 0)
	for _, w := range workers {
		if !w.IsActive {
			availableWorkers = append(availableWorkers, w)
		}
	}

	if len(availableWorkers) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	selected := &availableWorkers[r.current%len(availableWorkers)]
	r.current++

	return selected
}

func (r *RoundRobinBalancer) GetStrategy() string {
	return "round_robin"
}