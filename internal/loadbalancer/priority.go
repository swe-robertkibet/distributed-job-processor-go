package loadbalancer

import (
	"distributed-job-processor/internal/worker"
	"distributed-job-processor/pkg/job"
)

type PriorityBalancer struct{}

func NewPriorityBalancer() *PriorityBalancer {
	return &PriorityBalancer{}
}

func (p *PriorityBalancer) SelectWorker(workers []worker.WorkerStats, j *job.Job) *worker.WorkerStats {
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

	switch j.Priority {
	case job.PriorityHigh:
		return p.selectByLeastLoad(availableWorkers)
	case job.PriorityNormal:
		return p.selectRoundRobin(availableWorkers)
	case job.PriorityLow:
		return p.selectByMostLoad(availableWorkers)
	default:
		return p.selectByLeastLoad(availableWorkers)
	}
}

func (p *PriorityBalancer) selectByLeastLoad(workers []worker.WorkerStats) *worker.WorkerStats {
	var selected *worker.WorkerStats
	minLoad := int64(-1)

	for i := range workers {
		w := &workers[i]
		if minLoad == -1 || w.JobCount < minLoad {
			minLoad = w.JobCount
			selected = w
		}
	}

	return selected
}

func (p *PriorityBalancer) selectRoundRobin(workers []worker.WorkerStats) *worker.WorkerStats {
	if len(workers) == 0 {
		return nil
	}
	return &workers[0]
}

func (p *PriorityBalancer) selectByMostLoad(workers []worker.WorkerStats) *worker.WorkerStats {
	var selected *worker.WorkerStats
	maxLoad := int64(-1)

	for i := range workers {
		w := &workers[i]
		if w.JobCount > maxLoad {
			maxLoad = w.JobCount
			selected = w
		}
	}

	return selected
}

func (p *PriorityBalancer) GetStrategy() string {
	return "priority"
}