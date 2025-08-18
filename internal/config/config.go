package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Server   ServerConfig
	MongoDB  MongoDBConfig
	Redis    RedisConfig
	Election ElectionConfig
	LoadBalancer LoadBalancerConfig
	Retry    RetryConfig
	Metrics  MetricsConfig
	Security SecurityConfig
}

type ServerConfig struct {
	Port         string
	Host         string
	WorkerCount  int
	NodeID       string
}

type MongoDBConfig struct {
	URI      string
	Database string
	Timeout  time.Duration
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type ElectionConfig struct {
	Algorithm string
	Timeout   time.Duration
	Interval  time.Duration
}

type LoadBalancerConfig struct {
	Strategy string
}

type RetryConfig struct {
	Policy       string
	MaxRetries   int
	BaseDelay    time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	JitterFactor float64
}

type MetricsConfig struct {
	Enabled bool
	Port    string
}

type SecurityConfig struct {
	TLSEnabled bool
	CertFile   string
	KeyFile    string
	AuthEnabled bool
	JWTSecret   string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	return &Config{
		Server: ServerConfig{
			Port:        getEnv("SERVER_PORT", "8080"),
			Host:        getEnv("SERVER_HOST", "0.0.0.0"),
			WorkerCount: getEnvInt("WORKER_COUNT", 10),
			NodeID:      getEnv("NODE_ID", "node-1"),
		},
		MongoDB: MongoDBConfig{
			URI:      getEnv("MONGODB_URI", "mongodb://localhost:27017"),
			Database: getEnv("MONGODB_DATABASE", "jobprocessor"),
			Timeout:  getEnvDuration("MONGODB_TIMEOUT", 30*time.Second),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Election: ElectionConfig{
			Algorithm: getEnv("ELECTION_ALGORITHM", "bully"),
			Timeout:   getEnvDuration("ELECTION_TIMEOUT", 10*time.Second),
			Interval:  getEnvDuration("ELECTION_INTERVAL", 30*time.Second),
		},
		LoadBalancer: LoadBalancerConfig{
			Strategy: getEnv("LOAD_BALANCER_STRATEGY", "round_robin"),
		},
		Retry: RetryConfig{
			Policy:       getEnv("RETRY_POLICY", "exponential"),
			MaxRetries:   getEnvInt("MAX_RETRIES", 3),
			BaseDelay:    getEnvDuration("RETRY_BASE_DELAY", 1*time.Second),
			MaxDelay:     getEnvDuration("RETRY_MAX_DELAY", 60*time.Second),
			Multiplier:   getEnvFloat("RETRY_MULTIPLIER", 2.0),
			JitterFactor: getEnvFloat("RETRY_JITTER_FACTOR", 0.1),
		},
		Metrics: MetricsConfig{
			Enabled: getEnvBool("METRICS_ENABLED", true),
			Port:    getEnv("METRICS_PORT", "9090"),
		},
		Security: SecurityConfig{
			TLSEnabled:  getEnvBool("TLS_ENABLED", false),
			CertFile:    getEnv("TLS_CERT_FILE", ""),
			KeyFile:     getEnv("TLS_KEY_FILE", ""),
			AuthEnabled: getEnvBool("AUTH_ENABLED", false),
			JWTSecret:   getEnv("JWT_SECRET", "your-secret-key"),
		},
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}