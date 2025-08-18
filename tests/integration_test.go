package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"distributed-job-processor/internal/config"
	"distributed-job-processor/internal/server"
	"distributed-job-processor/pkg/job"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TestProcessor struct {
	processCount int
}

func (t *TestProcessor) Process(ctx context.Context, j *job.Job) error {
	t.processCount++
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (t *TestProcessor) Type() string {
	return "test"
}

func setupTestServer(t *testing.T) (*server.Server, func()) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:        "0",
			Host:        "127.0.0.1",
			WorkerCount: 2,
			NodeID:      "test-node",
		},
		MongoDB: config.MongoDBConfig{
			URI:      "mongodb://localhost:27017",
			Database: "jobprocessor_test",
			Timeout:  10 * time.Second,
		},
		Redis: config.RedisConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       1,
		},
		Election: config.ElectionConfig{
			Algorithm: "bully",
			Timeout:   5 * time.Second,
			Interval:  10 * time.Second,
		},
		LoadBalancer: config.LoadBalancerConfig{
			Strategy: "round_robin",
		},
		Retry: config.RetryConfig{
			Policy:       "exponential",
			MaxRetries:   3,
			BaseDelay:    100 * time.Millisecond,
			MaxDelay:     1 * time.Second,
			Multiplier:   2.0,
			JitterFactor: 0.1,
		},
		Metrics: config.MetricsConfig{
			Enabled: false,
			Port:    "0",
		},
		Security: config.SecurityConfig{
			TLSEnabled:  false,
			AuthEnabled: false,
		},
	}

	srv, err := server.New(cfg)
	require.NoError(t, err)

	processor := &TestProcessor{}
	srv.RegisterProcessor(processor)

	cleanup := func() {
		srv.Shutdown(context.Background())
	}

	return srv, cleanup
}

func TestJobCreationAndProcessing(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	testJob := &job.Job{
		Type:     "test",
		Priority: job.PriorityNormal,
		Payload: map[string]any{
			"message": "hello world",
		},
		MaxRetries: 3,
	}

	srv.RegisterProcessor(&TestProcessor{})
	srv.setupHTTPServer()
	router := srv.Router()

	ctx := context.Background()
	go srv.Start(ctx)

	time.Sleep(2 * time.Second)

	jobJSON, err := json.Marshal(testJob)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/api/v1/jobs", strings.NewReader(string(jobJSON)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var createdJob job.Job
	err = json.Unmarshal(w.Body.Bytes(), &createdJob)
	require.NoError(t, err)

	assert.Equal(t, "test", createdJob.Type)
	assert.Equal(t, job.PriorityNormal, createdJob.Priority)
	assert.NotEmpty(t, createdJob.ID)

	time.Sleep(1 * time.Second)

	getReq := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/jobs/%s", createdJob.ID.Hex()), nil)
	getW := httptest.NewRecorder()

	router.ServeHTTP(getW, getReq)

	assert.Equal(t, http.StatusOK, getW.Code)

	var retrievedJob job.Job
	err = json.Unmarshal(getW.Body.Bytes(), &retrievedJob)
	require.NoError(t, err)

	assert.Equal(t, createdJob.ID, retrievedJob.ID)
	assert.Equal(t, "test", retrievedJob.Type)
}

func TestHealthCheck(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	srv.setupHTTPServer()
	router := srv.Router()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response["status"])
	assert.Equal(t, "test-node", response["node_id"])
}

func TestJobListing(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()
	go srv.Start(ctx)

	time.Sleep(1 * time.Second)

	for i := 0; i < 3; i++ {
		testJob := &job.Job{
			Type:     "test",
			Priority: job.PriorityNormal,
			Payload: map[string]any{
				"index": i,
			},
			MaxRetries: 1,
		}

		jobJSON, err := json.Marshal(testJob)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/api/v1/jobs", strings.NewReader(string(jobJSON)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
	}

	time.Sleep(500 * time.Millisecond)

	listReq := httptest.NewRequest("GET", "/api/v1/jobs", nil)
	listW := httptest.NewRecorder()

	srv.ServeHTTP(listW, listReq)

	assert.Equal(t, http.StatusOK, listW.Code)

	var jobs []*job.Job
	err := json.Unmarshal(listW.Body.Bytes(), &jobs)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(jobs), 3)
}

func TestSystemStats(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var stats map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &stats)
	require.NoError(t, err)

	assert.Contains(t, stats, "jobs")
	assert.Contains(t, stats, "queue")
	assert.Contains(t, stats, "node")

	nodeStats := stats["node"].(map[string]any)
	assert.Equal(t, "test-node", nodeStats["id"])
	assert.Equal(t, "round_robin", nodeStats["strategy"])
}

func TestLoadBalancingStrategies(t *testing.T) {
	strategies := []string{"round_robin", "least_loaded", "random", "priority"}

	for _, strategy := range strategies {
		t.Run(strategy, func(t *testing.T) {
			factory := &loadbalancer.LoadBalancerFactory{}
			balancer := factory.Create(strategy)
			
			assert.NotNil(t, balancer)
			assert.Equal(t, strategy, balancer.GetStrategy())
		})
	}
}

func TestRetryPolicies(t *testing.T) {
	policies := []string{"fixed", "exponential"}

	for _, policy := range policies {
		t.Run(policy, func(t *testing.T) {
			cfg := &config.Config{
				Retry: config.RetryConfig{
					Policy:       policy,
					MaxRetries:   3,
					BaseDelay:    100 * time.Millisecond,
					MaxDelay:     1 * time.Second,
					Multiplier:   2.0,
					JitterFactor: 0.1,
				},
			}

			assert.Equal(t, policy, cfg.Retry.Policy)
			assert.Equal(t, 3, cfg.Retry.MaxRetries)
		})
	}
}

func TestJobPriorities(t *testing.T) {
	priorities := []job.Priority{
		job.PriorityLow,
		job.PriorityNormal,
		job.PriorityHigh,
	}

	for _, priority := range priorities {
		t.Run(fmt.Sprintf("priority_%d", priority), func(t *testing.T) {
			testJob := &job.Job{
				ID:       primitive.NewObjectID(),
				Type:     "test",
				Priority: priority,
				Status:   job.StatusPending,
			}

			assert.Equal(t, priority, testJob.Priority)
			assert.Equal(t, "test", testJob.Type)
			assert.Equal(t, job.StatusPending, testJob.Status)
		})
	}
}

func TestJobStatuses(t *testing.T) {
	statuses := []job.Status{
		job.StatusPending,
		job.StatusRunning,
		job.StatusCompleted,
		job.StatusFailed,
		job.StatusRetrying,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			testJob := &job.Job{
				ID:     primitive.NewObjectID(),
				Type:   "test",
				Status: status,
			}

			assert.Equal(t, status, testJob.Status)
		})
	}
}

func BenchmarkJobCreation(b *testing.B) {
	srv, cleanup := setupTestServer(&testing.T{})
	defer cleanup()

	testJob := &job.Job{
		Type:     "test",
		Priority: job.PriorityNormal,
		Payload: map[string]any{
			"benchmark": true,
		},
		MaxRetries: 1,
	}

	jobJSON, _ := json.Marshal(testJob)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/api/v1/jobs", strings.NewReader(string(jobJSON)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)
	}
}