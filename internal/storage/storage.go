package storage

import (
	"context"

	"distributed-job-processor/pkg/job"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Storage interface {
	CreateJob(ctx context.Context, job *job.Job) error
	UpdateJob(ctx context.Context, job *job.Job) error
	GetJob(ctx context.Context, id primitive.ObjectID) (*job.Job, error)
	GetJobs(ctx context.Context, filter JobFilter) ([]*job.Job, error)
	DeleteJob(ctx context.Context, id primitive.ObjectID) error
	GetJobStats(ctx context.Context) (*JobStats, error)
	Close() error
}

type JobFilter struct {
	Status     []job.Status
	Type       string
	WorkerID   string
	CreatedAfter  *primitive.DateTime
	CreatedBefore *primitive.DateTime
	Limit      int
	Offset     int
}

type JobStats struct {
	Total      int64 `json:"total"`
	Pending    int64 `json:"pending"`
	Running    int64 `json:"running"`
	Completed  int64 `json:"completed"`
	Failed     int64 `json:"failed"`
	Retrying   int64 `json:"retrying"`
}