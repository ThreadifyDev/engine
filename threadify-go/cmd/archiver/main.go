package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/viper"
	"github.com/threadify/engine/internal/archiver"
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

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read viper config: %v", err)
	}

	redisHost := viper.GetString("redis.host")
	redisPort := viper.GetInt("redis.port")
	redisPassword := viper.GetString("redis.password")
	redisDB := viper.GetInt("redis.db")

	valkeyClient, err := database.NewValkeyService(redisHost, redisPort, redisPassword, redisDB)
	if err != nil {
		log.Fatalf("Failed to connect to Valkey: %v", err)
	}
	defer valkeyClient.Close()
	log.Println("Connected to Valkey")

	// Initialize Postgres
	pgURL := viper.GetString("postgres.url")
	maxConns := viper.GetInt("postgres.max_connections")
	db, err := database.NewPostgresDB(pgURL, maxConns)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer db.Close()
	log.Println("Connected to Postgres")

	log.Println("Archiver service starting...")
	log.Printf("Consumer group: %s\n", archiverConfig.Streams.ConsumerGroup)

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
		log.Println("Connected to NATS")

		// Create NATS consumer for general archival
		natsConsumer, err := archiver.NewNATSConsumer(
			nc,
			db,
			archiverConfig.Streams.BatchSize,
			archiverConfig.Streams.BlockTimeout,
			"archiver-nats-1",
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
			log.Println("NATS archival consumer started")
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
			log.Println("Step state archival consumer started")
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal, stopping archiver...")
		cancel()
	}()

	log.Println("Archiver service ready - using NATS JetStream for all archival")

	// Block until shutdown signal
	<-ctx.Done()

	log.Println("Archiver service stopped")
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
