package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"distributed-job-processor/internal/config"
	"distributed-job-processor/internal/election"
	"distributed-job-processor/internal/loadbalancer"
	"distributed-job-processor/internal/logger"
	"distributed-job-processor/internal/metrics"
	"distributed-job-processor/internal/queue"
	"distributed-job-processor/internal/retry"
	"distributed-job-processor/internal/storage"
	"distributed-job-processor/internal/worker"
	"distributed-job-processor/pkg/job"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Server struct {
	config       *config.Config
	storage      *storage.MongoStorage
	queue        *queue.RedisQueue
	workerPool   *worker.Pool
	election     *election.BullyElection
	loadBalancer loadbalancer.LoadBalancer
	httpServer   *http.Server
	registry     job.ProcessorRegistry
}

func New(cfg *config.Config) (*Server, error) {
	mongoStorage, err := storage.NewMongoStorage(
		cfg.MongoDB.URI,
		cfg.MongoDB.Database,
		cfg.MongoDB.Timeout,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create MongoDB storage: %w", err)
	}

	redisQueue := queue.NewRedisQueue(
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Redis.DB,
	)

	retryPolicy := retry.CreatePolicy(
		cfg.Retry.Policy,
		cfg.Retry.BaseDelay,
		cfg.Retry.MaxDelay,
		cfg.Retry.Multiplier,
		cfg.Retry.JitterFactor,
	)

	workerPool := worker.NewPool(
		cfg.Server.WorkerCount,
		cfg.Server.NodeID,
		redisQueue,
		mongoStorage,
		retryPolicy,
	)

	bullyElection := election.NewBullyElection(
		cfg.Server.NodeID,
		mongoStorage,
		cfg.Election.Timeout,
		cfg.Election.Interval,
	)

	factory := &loadbalancer.LoadBalancerFactory{}
	lb := factory.Create(cfg.LoadBalancer.Strategy)

	return &Server{
		config:       cfg,
		storage:      mongoStorage,
		queue:        redisQueue,
		workerPool:   workerPool,
		election:     bullyElection,
		loadBalancer: lb,
		registry:     make(job.ProcessorRegistry),
	}, nil
}

func (s *Server) Start(ctx context.Context) error {
	logger.WithFields(logrus.Fields{
		"node_id":      s.config.Server.NodeID,
		"worker_count": s.config.Server.WorkerCount,
		"strategy":     s.loadBalancer.GetStrategy(),
	}).Info("Starting distributed job processor server")

	if s.config.Metrics.Enabled {
		go metrics.StartMetricsServer(s.config.Metrics.Port)
	}

	s.election.Start()
	s.workerPool.Start()

	s.setupHTTPServer()

	s.election.AddCallback(func(isLeader bool, leaderID string) {
		if isLeader {
			metrics.RecordLeaderElection(s.config.Server.NodeID, "elected")
			logger.Info("This node is now the leader")
		} else {
			logger.WithField("leader_id", leaderID).Info("Leadership changed")
		}
		metrics.UpdateNodeInfo(s.config.Server.NodeID, "1.0.0", isLeader)
	})

	logger.WithField("port", s.config.Server.Port).Info("HTTP server starting")
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) {
	logger.Info("Shutting down server")

	if s.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		s.httpServer.Shutdown(shutdownCtx)
	}

	s.workerPool.Stop()
	s.election.Stop()
	s.queue.Close()
	s.storage.Close()

	logger.Info("Server shutdown complete")
}

func (s *Server) RegisterProcessor(processor job.Processor) {
	s.workerPool.RegisterProcessor(processor)
	s.registry.Register(processor)
}

func (s *Server) setupHTTPServer() {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	api := router.Group("/api/v1")
	{
		api.POST("/jobs", s.createJob)
		api.GET("/jobs/:id", s.getJob)
		api.GET("/jobs", s.listJobs)
		api.DELETE("/jobs/:id", s.deleteJob)
		api.GET("/stats", s.getStats)
		api.GET("/workers", s.getWorkers)
		api.GET("/nodes", s.getNodes)
		api.GET("/leader", s.getLeader)
	}

	router.GET("/health", s.healthCheck)

	s.httpServer = &http.Server{
		Addr:         ":" + s.config.Server.Port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
}

func (s *Server) createJob(c *gin.Context) {
	var j job.Job
	if err := c.ShouldBindJSON(&j); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if j.MaxRetries == 0 {
		j.MaxRetries = s.config.Retry.MaxRetries
	}

	if err := s.storage.CreateJob(c.Request.Context(), &j); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := s.queue.Push(c.Request.Context(), &j); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	metrics.RecordJobStart(j.Type)
	c.JSON(http.StatusCreated, j)
}

func (s *Server) getJob(c *gin.Context) {
	id := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	j, err := s.storage.GetJob(c.Request.Context(), objID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, j)
}

func (s *Server) listJobs(c *gin.Context) {
	filter := storage.JobFilter{
		Limit: 50,
	}

	if status := c.Query("status"); status != "" {
		filter.Status = []job.Status{job.Status(status)}
	}

	if jobType := c.Query("type"); jobType != "" {
		filter.Type = jobType
	}

	jobs, err := s.storage.GetJobs(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, jobs)
}

func (s *Server) deleteJob(c *gin.Context) {
	id := c.Param("id")
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	if err := s.storage.DeleteJob(c.Request.Context(), objID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Job deleted successfully"})
}

func (s *Server) getStats(c *gin.Context) {
	jobStats, err := s.storage.GetJobStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	queueStats, err := s.queue.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"jobs":  jobStats,
		"queue": queueStats,
		"node": gin.H{
			"id":        s.config.Server.NodeID,
			"is_leader": s.election.IsLeader(),
			"strategy":  s.loadBalancer.GetStrategy(),
		},
	})
}

func (s *Server) getWorkers(c *gin.Context) {
	workers := s.workerPool.GetWorkerStats()
	c.JSON(http.StatusOK, workers)
}

func (s *Server) getNodes(c *gin.Context) {
	nodes, err := s.storage.GetNodes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, nodes)
}

func (s *Server) getLeader(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"leader_id": s.election.GetLeader(),
		"is_leader": s.election.IsLeader(),
	})
}

func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"node_id": s.config.Server.NodeID,
		"time":    time.Now().UTC(),
	})
}