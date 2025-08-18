package storage

import (
	"context"
	"fmt"
	"time"

	"distributed-job-processor/pkg/job"
	"distributed-job-processor/internal/logger"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	JobsCollection = "jobs"
	NodesCollection = "nodes"
)

type MongoStorage struct {
	client   *mongo.Client
	database *mongo.Database
	jobs     *mongo.Collection
	nodes    *mongo.Collection
}

func NewMongoStorage(uri, dbName string, timeout time.Duration) (*MongoStorage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	clientOptions := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err = client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	database := client.Database(dbName)
	jobs := database.Collection(JobsCollection)
	nodes := database.Collection(NodesCollection)

	storage := &MongoStorage{
		client:   client,
		database: database,
		jobs:     jobs,
		nodes:    nodes,
	}

	if err := storage.createIndexes(ctx); err != nil {
		logger.Error("Failed to create indexes:", err)
	}

	return storage, nil
}

func (m *MongoStorage) createIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "status", Value: 1}},
			Options: options.Index().SetName("status_idx"),
		},
		{
			Keys: bson.D{{Key: "type", Value: 1}},
			Options: options.Index().SetName("type_idx"),
		},
		{
			Keys: bson.D{{Key: "worker_id", Value: 1}},
			Options: options.Index().SetName("worker_id_idx"),
		},
		{
			Keys: bson.D{{Key: "created_at", Value: 1}},
			Options: options.Index().SetName("created_at_idx"),
		},
		{
			Keys: bson.D{{Key: "scheduled_at", Value: 1}},
			Options: options.Index().SetName("scheduled_at_idx"),
		},
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "scheduled_at", Value: 1},
			},
			Options: options.Index().SetName("status_scheduled_idx"),
		},
	}

	_, err := m.jobs.Indexes().CreateMany(ctx, indexes)
	return err
}

func (m *MongoStorage) CreateJob(ctx context.Context, j *job.Job) error {
	if j.ID.IsZero() {
		j.ID = primitive.NewObjectID()
	}
	
	j.CreatedAt = time.Now()
	j.UpdatedAt = j.CreatedAt
	
	if j.ScheduledAt.IsZero() {
		j.ScheduledAt = j.CreatedAt
	}

	if j.Status == "" {
		j.Status = job.StatusPending
	}

	if j.Priority == 0 {
		j.Priority = job.PriorityNormal
	}

	_, err := m.jobs.InsertOne(ctx, j)
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	return nil
}

func (m *MongoStorage) UpdateJob(ctx context.Context, j *job.Job) error {
	j.UpdatedAt = time.Now()

	filter := bson.M{"_id": j.ID}
	update := bson.M{"$set": j}

	result, err := m.jobs.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("job not found: %s", j.ID.Hex())
	}

	return nil
}

func (m *MongoStorage) GetJob(ctx context.Context, id primitive.ObjectID) (*job.Job, error) {
	var j job.Job
	filter := bson.M{"_id": id}

	err := m.jobs.FindOne(ctx, filter).Decode(&j)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("job not found: %s", id.Hex())
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	return &j, nil
}

func (m *MongoStorage) GetJobs(ctx context.Context, filter JobFilter) ([]*job.Job, error) {
	mongoFilter := bson.M{}

	if len(filter.Status) > 0 {
		mongoFilter["status"] = bson.M{"$in": filter.Status}
	}

	if filter.Type != "" {
		mongoFilter["type"] = filter.Type
	}

	if filter.WorkerID != "" {
		mongoFilter["worker_id"] = filter.WorkerID
	}

	if filter.CreatedAfter != nil || filter.CreatedBefore != nil {
		createdFilter := bson.M{}
		if filter.CreatedAfter != nil {
			createdFilter["$gte"] = filter.CreatedAfter
		}
		if filter.CreatedBefore != nil {
			createdFilter["$lte"] = filter.CreatedBefore
		}
		mongoFilter["created_at"] = createdFilter
	}

	opts := options.Find()
	if filter.Limit > 0 {
		opts.SetLimit(int64(filter.Limit))
	}
	if filter.Offset > 0 {
		opts.SetSkip(int64(filter.Offset))
	}
	opts.SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := m.jobs.Find(ctx, mongoFilter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find jobs: %w", err)
	}
	defer cursor.Close(ctx)

	var jobs []*job.Job
	for cursor.Next(ctx) {
		var j job.Job
		if err := cursor.Decode(&j); err != nil {
			return nil, fmt.Errorf("failed to decode job: %w", err)
		}
		jobs = append(jobs, &j)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return jobs, nil
}

func (m *MongoStorage) DeleteJob(ctx context.Context, id primitive.ObjectID) error {
	filter := bson.M{"_id": id}
	result, err := m.jobs.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}

	if result.DeletedCount == 0 {
		return fmt.Errorf("job not found: %s", id.Hex())
	}

	return nil
}

func (m *MongoStorage) GetJobStats(ctx context.Context) (*JobStats, error) {
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id": "$status",
				"count": bson.M{"$sum": 1},
			},
		},
	}

	cursor, err := m.jobs.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate job stats: %w", err)
	}
	defer cursor.Close(ctx)

	stats := &JobStats{}
	statusCounts := make(map[string]int64)

	for cursor.Next(ctx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode stats: %w", err)
		}
		statusCounts[result.ID] = result.Count
		stats.Total += result.Count
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	stats.Pending = statusCounts[string(job.StatusPending)]
	stats.Running = statusCounts[string(job.StatusRunning)]
	stats.Completed = statusCounts[string(job.StatusCompleted)]
	stats.Failed = statusCounts[string(job.StatusFailed)]
	stats.Retrying = statusCounts[string(job.StatusRetrying)]

	return stats, nil
}

func (m *MongoStorage) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	return m.client.Disconnect(ctx)
}

type NodeInfo struct {
	ID         string    `bson:"_id" json:"id"`
	Address    string    `bson:"address" json:"address"`
	IsLeader   bool      `bson:"is_leader" json:"is_leader"`
	LastSeen   time.Time `bson:"last_seen" json:"last_seen"`
	WorkerCount int      `bson:"worker_count" json:"worker_count"`
	Version    string    `bson:"version" json:"version"`
}

func (m *MongoStorage) RegisterNode(ctx context.Context, node *NodeInfo) error {
	filter := bson.M{"_id": node.ID}
	update := bson.M{
		"$set": bson.M{
			"address":      node.Address,
			"is_leader":    node.IsLeader,
			"last_seen":    time.Now(),
			"worker_count": node.WorkerCount,
			"version":      node.Version,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := m.nodes.UpdateOne(ctx, filter, update, opts)
	return err
}

func (m *MongoStorage) GetNodes(ctx context.Context) ([]*NodeInfo, error) {
	filter := bson.M{
		"last_seen": bson.M{
			"$gte": time.Now().Add(-5 * time.Minute),
		},
	}

	cursor, err := m.nodes.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var nodes []*NodeInfo
	for cursor.Next(ctx) {
		var node NodeInfo
		if err := cursor.Decode(&node); err != nil {
			return nil, err
		}
		nodes = append(nodes, &node)
	}

	return nodes, cursor.Err()
}