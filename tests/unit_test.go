package tests

import (
	"fmt"
	"testing"
	"time"

	"distributed-job-processor/internal/auth"
	"distributed-job-processor/internal/loadbalancer"
	"distributed-job-processor/internal/retry"
	"distributed-job-processor/internal/worker"
	"distributed-job-processor/pkg/job"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestRoundRobinLoadBalancer(t *testing.T) {
	balancer := loadbalancer.NewRoundRobinBalancer()

	workers := []worker.WorkerStats{
		{ID: "worker-1", IsActive: false, JobCount: 0},
		{ID: "worker-2", IsActive: false, JobCount: 0},
		{ID: "worker-3", IsActive: false, JobCount: 0},
	}

	testJob := &job.Job{
		Type:     "test",
		Priority: job.PriorityNormal,
	}

	selected1 := balancer.SelectWorker(workers, testJob)
	assert.NotNil(t, selected1)
	assert.Equal(t, "worker-1", selected1.ID)

	selected2 := balancer.SelectWorker(workers, testJob)
	assert.NotNil(t, selected2)
	assert.Equal(t, "worker-2", selected2.ID)

	selected3 := balancer.SelectWorker(workers, testJob)
	assert.NotNil(t, selected3)
	assert.Equal(t, "worker-3", selected3.ID)

	selected4 := balancer.SelectWorker(workers, testJob)
	assert.NotNil(t, selected4)
	assert.Equal(t, "worker-1", selected4.ID)
}

func TestLeastLoadedBalancer(t *testing.T) {
	balancer := loadbalancer.NewLeastLoadedBalancer()

	workers := []worker.WorkerStats{
		{ID: "worker-1", IsActive: false, JobCount: 5},
		{ID: "worker-2", IsActive: false, JobCount: 2},
		{ID: "worker-3", IsActive: false, JobCount: 8},
	}

	testJob := &job.Job{
		Type:     "test",
		Priority: job.PriorityNormal,
	}

	selected := balancer.SelectWorker(workers, testJob)
	assert.NotNil(t, selected)
	assert.Equal(t, "worker-2", selected.ID)
	assert.Equal(t, int64(2), selected.JobCount)
}

func TestRandomBalancer(t *testing.T) {
	balancer := loadbalancer.NewRandomBalancer()

	workers := []worker.WorkerStats{
		{ID: "worker-1", IsActive: false, JobCount: 0},
		{ID: "worker-2", IsActive: false, JobCount: 0},
		{ID: "worker-3", IsActive: false, JobCount: 0},
	}

	testJob := &job.Job{
		Type:     "test",
		Priority: job.PriorityNormal,
	}

	selected := balancer.SelectWorker(workers, testJob)
	assert.NotNil(t, selected)
	assert.Contains(t, []string{"worker-1", "worker-2", "worker-3"}, selected.ID)
}

func TestPriorityBalancer(t *testing.T) {
	balancer := loadbalancer.NewPriorityBalancer()

	workers := []worker.WorkerStats{
		{ID: "worker-1", IsActive: false, JobCount: 3},
		{ID: "worker-2", IsActive: false, JobCount: 1},
		{ID: "worker-3", IsActive: false, JobCount: 5},
	}

	highPriorityJob := &job.Job{
		Type:     "test",
		Priority: job.PriorityHigh,
	}

	selected := balancer.SelectWorker(workers, highPriorityJob)
	assert.NotNil(t, selected)
	assert.Equal(t, "worker-2", selected.ID)

	lowPriorityJob := &job.Job{
		Type:     "test",
		Priority: job.PriorityLow,
	}

	selected = balancer.SelectWorker(workers, lowPriorityJob)
	assert.NotNil(t, selected)
	assert.Equal(t, "worker-3", selected.ID)
}

func TestBalancerWithActiveWorkers(t *testing.T) {
	balancer := loadbalancer.NewRoundRobinBalancer()

	workers := []worker.WorkerStats{
		{ID: "worker-1", IsActive: true, JobCount: 0},
		{ID: "worker-2", IsActive: false, JobCount: 0},
		{ID: "worker-3", IsActive: true, JobCount: 0},
	}

	testJob := &job.Job{
		Type:     "test",
		Priority: job.PriorityNormal,
	}

	selected := balancer.SelectWorker(workers, testJob)
	assert.NotNil(t, selected)
	assert.Equal(t, "worker-2", selected.ID)
	assert.False(t, selected.IsActive)
}

func TestFixedRetryPolicy(t *testing.T) {
	policy := retry.NewFixedPolicy(1 * time.Second)

	delay1 := policy.NextDelay(1)
	delay2 := policy.NextDelay(5)
	delay3 := policy.NextDelay(10)

	assert.Equal(t, 1*time.Second, delay1)
	assert.Equal(t, 1*time.Second, delay2)
	assert.Equal(t, 1*time.Second, delay3)
}

func TestExponentialRetryPolicy(t *testing.T) {
	policy := retry.NewExponentialPolicy(1*time.Second, 10*time.Second, 2.0)

	delay1 := policy.NextDelay(1)
	delay2 := policy.NextDelay(2)
	delay3 := policy.NextDelay(3)

	assert.Equal(t, 1*time.Second, delay1)
	assert.Equal(t, 2*time.Second, delay2)
	assert.Equal(t, 4*time.Second, delay3)

	delay10 := policy.NextDelay(10)
	assert.Equal(t, 10*time.Second, delay10)
}

func TestJitterRetryPolicy(t *testing.T) {
	basePolicy := retry.NewFixedPolicy(1 * time.Second)
	jitterPolicy := retry.NewJitterPolicy(basePolicy, 0.1)

	delay := jitterPolicy.NextDelay(1)

	assert.True(t, delay >= 900*time.Millisecond)
	assert.True(t, delay <= 1100*time.Millisecond)
}

func TestCreateRetryPolicy(t *testing.T) {
	fixedPolicy := retry.CreatePolicy("fixed", 1*time.Second, 10*time.Second, 2.0, 0.0)
	assert.IsType(t, &retry.FixedPolicy{}, fixedPolicy)

	expPolicy := retry.CreatePolicy("exponential", 1*time.Second, 10*time.Second, 2.0, 0.0)
	assert.IsType(t, &retry.ExponentialPolicy{}, expPolicy)

	jitterPolicy := retry.CreatePolicy("exponential", 1*time.Second, 10*time.Second, 2.0, 0.1)
	assert.IsType(t, &retry.JitterPolicy{}, jitterPolicy)

	defaultPolicy := retry.CreatePolicy("unknown", 1*time.Second, 10*time.Second, 2.0, 0.0)
	assert.IsType(t, &retry.ExponentialPolicy{}, defaultPolicy)
}

func TestJobProcessor(t *testing.T) {
	registry := make(job.ProcessorRegistry)

	processor := &TestProcessor{}
	registry.Register(processor)

	retrievedProcessor, exists := registry.Get("test")
	assert.True(t, exists)
	assert.Equal(t, processor, retrievedProcessor)

	_, exists = registry.Get("nonexistent")
	assert.False(t, exists)
}

func TestJobStatusTransitions(t *testing.T) {
	j := &job.Job{
		ID:       primitive.NewObjectID(),
		Type:     "test",
		Status:   job.StatusPending,
		Priority: job.PriorityNormal,
	}

	assert.Equal(t, job.StatusPending, j.Status)

	j.Status = job.StatusRunning
	assert.Equal(t, job.StatusRunning, j.Status)

	j.Status = job.StatusCompleted
	assert.Equal(t, job.StatusCompleted, j.Status)
}

func TestAuthManager(t *testing.T) {
	config := &auth.AuthConfig{
		Enabled:   true,
		JWTSecret: "test-secret",
		TokenTTL:  1 * time.Hour,
	}

	authManager := auth.NewAuthManager(config)

	token, err := authManager.GenerateToken("user123", auth.RoleUser)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := authManager.ValidateToken(token)
	assert.NoError(t, err)
	assert.Equal(t, "user123", claims.UserID)
	assert.Equal(t, auth.RoleUser, claims.Role)

	_, err = authManager.ValidateToken("invalid-token")
	assert.Error(t, err)
}

func TestAuthManagerDisabled(t *testing.T) {
	config := &auth.AuthConfig{
		Enabled: false,
	}

	authManager := auth.NewAuthManager(config)

	token, err := authManager.GenerateToken("user123", auth.RoleUser)
	assert.NoError(t, err)
	assert.Empty(t, token)

	claims, err := authManager.ValidateToken("")
	assert.NoError(t, err)
	assert.Equal(t, "system", claims.UserID)
	assert.Equal(t, auth.RoleAdmin, claims.Role)
}

func TestRoleHierarchy(t *testing.T) {
	roles := []auth.Role{auth.RoleAdmin, auth.RoleUser, auth.RoleWorker}
	
	for _, role := range roles {
		assert.NotEmpty(t, string(role))
	}
	
	assert.Equal(t, auth.RoleAdmin, auth.Role("admin"))
	assert.Equal(t, auth.RoleUser, auth.Role("user"))
	assert.Equal(t, auth.RoleWorker, auth.Role("worker"))
}

func BenchmarkRoundRobinSelection(b *testing.B) {
	balancer := loadbalancer.NewRoundRobinBalancer()

	workers := make([]worker.WorkerStats, 100)
	for i := 0; i < 100; i++ {
		workers[i] = worker.WorkerStats{
			ID:       fmt.Sprintf("worker-%d", i),
			IsActive: false,
			JobCount: int64(i),
		}
	}

	testJob := &job.Job{
		Type:     "test",
		Priority: job.PriorityNormal,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balancer.SelectWorker(workers, testJob)
	}
}

func BenchmarkLeastLoadedSelection(b *testing.B) {
	balancer := loadbalancer.NewLeastLoadedBalancer()

	workers := make([]worker.WorkerStats, 100)
	for i := 0; i < 100; i++ {
		workers[i] = worker.WorkerStats{
			ID:       fmt.Sprintf("worker-%d", i),
			IsActive: false,
			JobCount: int64(i),
		}
	}

	testJob := &job.Job{
		Type:     "test",
		Priority: job.PriorityNormal,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balancer.SelectWorker(workers, testJob)
	}
}

func BenchmarkRetryPolicyCalculation(b *testing.B) {
	policy := retry.NewExponentialPolicy(1*time.Second, 60*time.Second, 2.0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		policy.NextDelay(i % 10)
	}
}