package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"distributed-job-processor/pkg/job"
	"distributed-job-processor/internal/logger"
	"distributed-job-processor/internal/queue"
	"distributed-job-processor/internal/storage"
	"distributed-job-processor/internal/retry"

	"github.com/sirupsen/logrus"
)

type Pool struct {
	workers     []*Worker
	queue       *queue.RedisQueue
	storage     storage.Storage
	registry    job.ProcessorRegistry
	retryPolicy retry.Policy
	workerCount int
	nodeID      string
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

type Worker struct {
	id       string
	nodeID   string
	isActive bool
	jobCount int64
	mu       sync.RWMutex
}

func NewPool(workerCount int, nodeID string, q *queue.RedisQueue, s storage.Storage, retryPolicy retry.Policy) *Pool {
	ctx, cancel := context.WithCancel(context.Background())
	
	pool := &Pool{
		workers:     make([]*Worker, workerCount),
		queue:       q,
		storage:     s,
		registry:    make(job.ProcessorRegistry),
		retryPolicy: retryPolicy,
		workerCount: workerCount,
		nodeID:      nodeID,
		ctx:         ctx,
		cancel:      cancel,
	}

	for i := 0; i < workerCount; i++ {
		pool.workers[i] = &Worker{
			id:       pool.generateWorkerID(i),
			nodeID:   nodeID,
			isActive: false,
			jobCount: 0,
		}
	}

	return pool
}

func (p *Pool) Start() {
	logger.WithFields(logrus.Fields{
		"worker_count": p.workerCount,
		"node_id":      p.nodeID,
	}).Info("Starting worker pool")

	for i, worker := range p.workers {
		p.wg.Add(1)
		go p.runWorker(i, worker)
	}
}

func (p *Pool) Stop() {
	logger.Info("Stopping worker pool")
	p.cancel()
	p.wg.Wait()
	logger.Info("Worker pool stopped")
}

func (p *Pool) RegisterProcessor(processor job.Processor) {
	p.registry.Register(processor)
	logger.WithField("job_type", processor.Type()).Info("Registered job processor")
}

func (p *Pool) runWorker(workerIndex int, worker *Worker) {
	defer p.wg.Done()
	
	logger.WithFields(logrus.Fields{
		"worker_id": worker.id,
		"node_id":   worker.nodeID,
	}).Info("Starting worker")

	for {
		select {
		case <-p.ctx.Done():
			logger.WithField("worker_id", worker.id).Info("Worker shutting down")
			return
		default:
			p.processJob(worker)
		}
	}
}

func (p *Pool) processJob(worker *Worker) {
	j, err := p.queue.Pop(p.ctx)
	if err != nil {
		logger.WithError(err).Error("Failed to pop job from queue")
		time.Sleep(1 * time.Second)
		return
	}

	if j == nil {
		return
	}

	worker.mu.Lock()
	worker.isActive = true
	worker.jobCount++
	worker.mu.Unlock()

	defer func() {
		worker.mu.Lock()
		worker.isActive = false
		worker.mu.Unlock()
	}()

	j.WorkerID = worker.id
	j.Status = job.StatusRunning
	now := time.Now()
	j.StartedAt = &now
	j.UpdatedAt = now

	logger.WithFields(logrus.Fields{
		"job_id":    j.ID.Hex(),
		"job_type":  j.Type,
		"worker_id": worker.id,
	}).Info("Processing job")

	if err := p.storage.UpdateJob(p.ctx, j); err != nil {
		logger.WithError(err).Error("Failed to update job status to running")
	}

	processor, exists := p.registry.Get(j.Type)
	if !exists {
		p.handleJobFailure(j, "No processor found for job type: "+j.Type)
		return
	}

	if err := processor.Process(p.ctx, j); err != nil {
		p.handleJobError(j, err)
		return
	}

	p.handleJobSuccess(j)
}

func (p *Pool) handleJobSuccess(j *job.Job) {
	j.Status = job.StatusCompleted
	now := time.Now()
	j.CompletedAt = &now
	j.UpdatedAt = now

	if err := p.storage.UpdateJob(p.ctx, j); err != nil {
		logger.WithError(err).Error("Failed to update job status to completed")
	}

	if err := p.queue.Complete(p.ctx, j); err != nil {
		logger.WithError(err).Error("Failed to move job to completed queue")
	}

	logger.WithFields(logrus.Fields{
		"job_id":   j.ID.Hex(),
		"job_type": j.Type,
		"duration": time.Since(*j.StartedAt),
	}).Info("Job completed successfully")
}

func (p *Pool) handleJobError(j *job.Job, err error) {
	j.Error = err.Error()
	j.RetryCount++
	j.UpdatedAt = time.Now()

	if j.RetryCount < j.MaxRetries {
		delay := p.retryPolicy.NextDelay(j.RetryCount)
		j.Status = job.StatusRetrying
		
		logger.WithFields(logrus.Fields{
			"job_id":      j.ID.Hex(),
			"retry_count": j.RetryCount,
			"delay":       delay,
			"error":       err.Error(),
		}).Warn("Job failed, retrying")

		if err := p.storage.UpdateJob(p.ctx, j); err != nil {
			logger.WithError(err).Error("Failed to update job for retry")
		}

		if err := p.queue.Retry(p.ctx, j, delay); err != nil {
			logger.WithError(err).Error("Failed to retry job")
		}
	} else {
		p.handleJobFailure(j, err.Error())
	}
}

func (p *Pool) handleJobFailure(j *job.Job, errorMsg string) {
	j.Status = job.StatusFailed
	j.Error = errorMsg
	j.UpdatedAt = time.Now()

	if err := p.storage.UpdateJob(p.ctx, j); err != nil {
		logger.WithError(err).Error("Failed to update job status to failed")
	}

	if err := p.queue.Fail(p.ctx, j); err != nil {
		logger.WithError(err).Error("Failed to move job to failed queue")
	}

	logger.WithFields(logrus.Fields{
		"job_id":   j.ID.Hex(),
		"job_type": j.Type,
		"error":    errorMsg,
	}).Error("Job failed permanently")
}

func (p *Pool) GetWorkerStats() []WorkerStats {
	stats := make([]WorkerStats, len(p.workers))
	
	for i, worker := range p.workers {
		worker.mu.RLock()
		stats[i] = WorkerStats{
			ID:       worker.id,
			NodeID:   worker.nodeID,
			IsActive: worker.isActive,
			JobCount: worker.jobCount,
		}
		worker.mu.RUnlock()
	}
	
	return stats
}

func (p *Pool) GetLeastLoadedWorker() *Worker {
	var leastLoaded *Worker
	minJobs := int64(-1)

	for _, worker := range p.workers {
		worker.mu.RLock()
		if !worker.isActive && (minJobs == -1 || worker.jobCount < minJobs) {
			minJobs = worker.jobCount
			leastLoaded = worker
		}
		worker.mu.RUnlock()
	}

	return leastLoaded
}

func (p *Pool) generateWorkerID(index int) string {
	return fmt.Sprintf("%s-worker-%d", p.nodeID, index)
}

type WorkerStats struct {
	ID       string `json:"id"`
	NodeID   string `json:"node_id"`
	IsActive bool   `json:"is_active"`
	JobCount int64  `json:"job_count"`
}