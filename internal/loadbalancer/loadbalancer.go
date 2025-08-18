package loadbalancer

import (
	"distributed-job-processor/internal/worker"
	"distributed-job-processor/pkg/job"
)

type LoadBalancer interface {
	SelectWorker(workers []worker.WorkerStats, job *job.Job) *worker.WorkerStats
	GetStrategy() string
}

type LoadBalancerFactory struct{}

func (f *LoadBalancerFactory) Create(strategy string) LoadBalancer {
	switch strategy {
	case "round_robin":
		return NewRoundRobinBalancer()
	case "least_loaded":
		return NewLeastLoadedBalancer()
	case "random":
		return NewRandomBalancer()
	case "priority":
		return NewPriorityBalancer()
	default:
		return NewRoundRobinBalancer()
	}
}