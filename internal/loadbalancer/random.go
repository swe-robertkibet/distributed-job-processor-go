package loadbalancer

import (
	"math/rand"
	"time"

	"distributed-job-processor/internal/worker"
	"distributed-job-processor/pkg/job"
)

type RandomBalancer struct {
	rng *rand.Rand
}

func NewRandomBalancer() *RandomBalancer {
	return &RandomBalancer{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (r *RandomBalancer) SelectWorker(workers []worker.WorkerStats, job *job.Job) *worker.WorkerStats {
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

	index := r.rng.Intn(len(availableWorkers))
	return &availableWorkers[index]
}

func (r *RandomBalancer) GetStrategy() string {
	return "random"
}