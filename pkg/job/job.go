package job

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusRunning    Status = "running"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusRetrying   Status = "retrying"
)

type Priority int

const (
	PriorityLow    Priority = 1
	PriorityNormal Priority = 5
	PriorityHigh   Priority = 10
)

type Job struct {
	ID          primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Type        string             `json:"type" bson:"type"`
	Payload     map[string]any     `json:"payload" bson:"payload"`
	Status      Status             `json:"status" bson:"status"`
	Priority    Priority           `json:"priority" bson:"priority"`
	CreatedAt   time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at" bson:"updated_at"`
	ScheduledAt time.Time          `json:"scheduled_at" bson:"scheduled_at"`
	StartedAt   *time.Time         `json:"started_at,omitempty" bson:"started_at,omitempty"`
	CompletedAt *time.Time         `json:"completed_at,omitempty" bson:"completed_at,omitempty"`
	RetryCount  int                `json:"retry_count" bson:"retry_count"`
	MaxRetries  int                `json:"max_retries" bson:"max_retries"`
	WorkerID    string             `json:"worker_id,omitempty" bson:"worker_id,omitempty"`
	Error       string             `json:"error,omitempty" bson:"error,omitempty"`
}

type Processor interface {
	Process(ctx context.Context, job *Job) error
	Type() string
}

type ProcessorRegistry map[string]Processor

func (r ProcessorRegistry) Register(processor Processor) {
	r[processor.Type()] = processor
}

func (r ProcessorRegistry) Get(jobType string) (Processor, bool) {
	processor, exists := r[jobType]
	return processor, exists
}