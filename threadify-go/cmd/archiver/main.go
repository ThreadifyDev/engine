package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"github.com/threadify/engine/internal/archiver"
	appconfig "github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	"gopkg.in/yaml.v3"
)

// Config represents the archiver configuration from config.yaml
type Config struct {
	Archiver struct {
		Enabled bool `yaml:"enabled"`
		Buffers map[string]struct {
			Size                 int `yaml:"size"`
			FlushIntervalSeconds int `yaml:"flush_interval_seconds"`
		} `yaml:"buffers"`
		ActivityStreams struct {
			Enabled              bool `yaml:"enabled"`
			NumPartitions        int  `yaml:"num_partitions"`
			WorkersPerInstance   int  `yaml:"workers_per_instance"`
			BufferSize           int  `yaml:"buffer_size"`
			FlushIntervalSeconds int  `yaml:"flush_interval_seconds"`
			MaxBufferSize        int  `yaml:"max_buffer_size"`
			TrimEnabled          bool `yaml:"trim_enabled"`
			TrimMaxLen           int  `yaml:"trim_maxlen"`
		} `yaml:"activity_streams"`
		Retry struct {
			MaxAttempts           int `yaml:"max_attempts"`
			InitialBackoffSeconds int `yaml:"initial_backoff_seconds"`
			MaxBackoffSeconds     int `yaml:"max_backoff_seconds"`
		} `yaml:"retry"`
		Streams struct {
			ConsumerGroup            string `yaml:"consumer_group"`
			BlockTimeoutMs           int    `yaml:"block_timeout_ms"`
			BatchSize                int    `yaml:"batch_size"`
			StepStateFlushIntervalMs int    `yaml:"step_state_flush_interval_ms"` // Flush interval for step state archival
		} `yaml:"streams"`
	} `yaml:"archiver"`
}

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config/config.yaml", "Path to config file")
	flag.Parse()

	// Load configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if !config.Archiver.Enabled {
		log.Println("Archiver is disabled in config")
		return
	}

	// Create archiver configuration
	archiverConfig := archiver.ArchiverConfig{
		Buffers: make(map[string]archiver.BufferConfig),
		Retry: archiver.RetryConfig{
			MaxAttempts:    config.Archiver.Retry.MaxAttempts,
			InitialBackoff: time.Duration(config.Archiver.Retry.InitialBackoffSeconds) * time.Second,
			MaxBackoff:     time.Duration(config.Archiver.Retry.MaxBackoffSeconds) * time.Second,
		},
		Streams: archiver.StreamConfig{
			ConsumerGroup: config.Archiver.Streams.ConsumerGroup,
			BlockTimeout:  time.Duration(config.Archiver.Streams.BlockTimeoutMs) * time.Millisecond,
			BatchSize:     config.Archiver.Streams.BatchSize,
		},
	}

	// Convert buffer configs
	for name, bufferCfg := range config.Archiver.Buffers {
		archiverConfig.Buffers[name] = archiver.BufferConfig{
			Size:          bufferCfg.Size,
			FlushInterval: time.Duration(bufferCfg.FlushIntervalSeconds) * time.Second,
		}
	}

	// Initialize Valkey client
	viper.SetConfigFile(*configPath)

	// Enable automatic environment variable support
	viper.AutomaticEnv()
	// Map environment variables with underscores to config keys with dots
	// e.g., REDIS_PASSWORD -> redis.password
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read viper config: %v", err)
	}

	// Load full config for NATS consumer (after viper is initialized)
	fullConfig, err := appconfig.LoadFromViper()
	if err != nil {
		log.Fatalf("Failed to load full config: %v", err)
	}

	redisHost := viper.GetString("redis.host")
	redisPort := viper.GetInt("redis.port")
	redisPassword := viper.GetString("redis.password")
	redisDB := viper.GetInt("redis.db")

	valkeyClient, err := database.NewValkeyService(redisHost, redisPort, redisPassword, redisDB, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if err != nil {
		log.Fatalf("Failed to connect to Valkey: %v", err)
	}
	defer valkeyClient.Close()
	// Connected to Valkey

	// Initialize Postgres
	pgURL := viper.GetString("postgres.url")
	maxConns := viper.GetInt("postgres.max_connections")
	db, err := database.NewPostgresDB(pgURL, maxConns)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer db.Close()
	// Connected to Postgres

	// Archiver service starting

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize NATS consumer for archival (graceful degradation if unavailable)
	natsURL := viper.GetString("nats.url")
	if natsURL == "" {
		natsURL = nats.DefaultURL // Default to nats://localhost:4222
	}
	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Printf("Warning: Failed to connect to NATS - archival will use Redis only: %v", err)
	} else {
		defer nc.Close()
		// Connected to NATS

		// Create NATS consumer for general archival
		natsConsumer, err := archiver.NewNATSConsumer(
			nc,
			db,
			archiverConfig.Streams.BatchSize,
			archiverConfig.Streams.BlockTimeout,
			"archiver-nats-1",
			fullConfig,
		)
		if err != nil {
			log.Printf("Warning: Failed to create NATS consumer: %v", err)
		} else {
			// Start NATS consumer
			go func() {
				if err := natsConsumer.Start(ctx); err != nil {
					log.Printf("NATS consumer error: %v", err)
				}
			}()
			defer natsConsumer.Stop()
			// NATS archival consumer started
		}

		// Create step state consumer
		flushInterval := 5 * time.Second // Default to 5 seconds
		if config.Archiver.Streams.StepStateFlushIntervalMs > 0 {
			flushInterval = time.Duration(config.Archiver.Streams.StepStateFlushIntervalMs) * time.Millisecond
		}

		stepStateConsumer, err := archiver.NewStepStateConsumer(
			nc,
			db,
			archiverConfig.Streams.BatchSize,
			flushInterval,
			"archiver-step-state-1",
		)
		if err != nil {
			log.Printf("Warning: Failed to create step state consumer: %v", err)
		} else {
			// Start step state consumer
			go func() {
				if err := stepStateConsumer.Start(ctx); err != nil {
					log.Printf("Step state consumer error: %v", err)
				}
			}()
			defer stepStateConsumer.Stop()
			// Step state archival consumer started
		}
	}

	// Start Prometheus metrics HTTP server
	metricsPort := 8082
	metricsServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", metricsPort),
		Handler: promhttp.Handler(),
	}

	go func() {
		log.Printf("Prometheus metrics endpoint enabled at http://localhost:%d/metrics", metricsPort)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal, stopping archiver...")

		// Shutdown metrics server
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("Metrics server shutdown error: %v", err)
		}

		cancel()
	}()

	log.Println("Archiver service ready")

	// Block until shutdown signal
	<-ctx.Done()

	// Archiver service stopped
}

func maskPassword(password string) string {
	if len(password) <= 4 {
		return "****"
	}
	return password[:2] + "****" + password[len(password)-2:]
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}
