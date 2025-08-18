package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"distributed-job-processor/pkg/job"
	"distributed-job-processor/internal/logger"

	"github.com/redis/go-redis/v9"
)

const (
	JobQueue         = "jobs:queue"
	ProcessingQueue  = "jobs:processing"
	CompletedQueue   = "jobs:completed"
	FailedQueue      = "jobs:failed"
	DelayedQueue     = "jobs:delayed"
)

type RedisQueue struct {
	client *redis.Client
}

func NewRedisQueue(addr, password string, db int) *RedisQueue {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	return &RedisQueue{
		client: rdb,
	}
}

func (q *RedisQueue) Push(ctx context.Context, j *job.Job) error {
	data, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	var queueName string
	if j.ScheduledAt.After(time.Now()) {
		queueName = DelayedQueue
		score := float64(j.ScheduledAt.Unix())
		return q.client.ZAdd(ctx, queueName, redis.Z{
			Score:  score,
			Member: data,
		}).Err()
	} else {
		queueName = JobQueue
		return q.client.LPush(ctx, queueName, data).Err()
	}
}

func (q *RedisQueue) Pop(ctx context.Context) (*job.Job, error) {
	q.moveDelayedJobs(ctx)

	result, err := q.client.BRPop(ctx, 1*time.Second, JobQueue).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var j job.Job
	if err := json.Unmarshal([]byte(result[1]), &j); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	if err := q.moveToProcessing(ctx, &j); err != nil {
		logger.Error("Failed to move job to processing queue:", err)
	}

	return &j, nil
}

func (q *RedisQueue) Complete(ctx context.Context, j *job.Job) error {
	if err := q.removeFromProcessing(ctx, j); err != nil {
		return err
	}

	data, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return q.client.LPush(ctx, CompletedQueue, data).Err()
}

func (q *RedisQueue) Fail(ctx context.Context, j *job.Job) error {
	if err := q.removeFromProcessing(ctx, j); err != nil {
		return err
	}

	data, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return q.client.LPush(ctx, FailedQueue, data).Err()
}

func (q *RedisQueue) Retry(ctx context.Context, j *job.Job, delay time.Duration) error {
	if err := q.removeFromProcessing(ctx, j); err != nil {
		return err
	}

	j.ScheduledAt = time.Now().Add(delay)
	return q.Push(ctx, j)
}

func (q *RedisQueue) moveToProcessing(ctx context.Context, j *job.Job) error {
	data, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return q.client.LPush(ctx, ProcessingQueue, data).Err()
}

func (q *RedisQueue) removeFromProcessing(ctx context.Context, j *job.Job) error {
	data, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return q.client.LRem(ctx, ProcessingQueue, 1, data).Err()
}

func (q *RedisQueue) moveDelayedJobs(ctx context.Context) {
	now := float64(time.Now().Unix())
	jobs, err := q.client.ZRangeByScore(ctx, DelayedQueue, &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%f", now),
	}).Result()

	if err != nil || len(jobs) == 0 {
		return
	}

	pipe := q.client.Pipeline()
	for _, jobData := range jobs {
		pipe.LPush(ctx, JobQueue, jobData)
		pipe.ZRem(ctx, DelayedQueue, jobData)
	}
	
	_, err = pipe.Exec(ctx)
	if err != nil {
		logger.Error("Failed to move delayed jobs:", err)
	}
}

func (q *RedisQueue) GetStats(ctx context.Context) (map[string]int64, error) {
	pipe := q.client.Pipeline()
	
	queueLen := pipe.LLen(ctx, JobQueue)
	processingLen := pipe.LLen(ctx, ProcessingQueue)
	completedLen := pipe.LLen(ctx, CompletedQueue)
	failedLen := pipe.LLen(ctx, FailedQueue)
	delayedLen := pipe.ZCard(ctx, DelayedQueue)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]int64{
		"pending":    queueLen.Val(),
		"processing": processingLen.Val(),
		"completed":  completedLen.Val(),
		"failed":     failedLen.Val(),
		"delayed":    delayedLen.Val(),
	}, nil
}

func (q *RedisQueue) Close() error {
	return q.client.Close()
}