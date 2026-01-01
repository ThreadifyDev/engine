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
			ConsumerGroup       string `yaml:"consumer_group"`
			BlockTimeoutSeconds int    `yaml:"block_timeout_seconds"`
			BatchSize           int    `yaml:"batch_size"`
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
			BlockTimeout:  time.Duration(config.Archiver.Streams.BlockTimeoutSeconds) * time.Second,
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

	// Create Postgres writer
	pgWriter := archiver.NewPostgresWriter(db)

	log.Println("Archiver service starting...")
	log.Printf("Consumer group: %s\n", archiverConfig.Streams.ConsumerGroup)
	log.Printf("Configured queues: %v\n", getQueueNames(archiverConfig.Buffers))

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal, stopping archiver...")
		cancel()
	}()

	// Setup streams and consumer groups
	streamSetup := archiver.NewStreamSetup(valkeyClient)
	streams := archiver.GetRequiredStreams()
	if err := streamSetup.EnsureAllStreams(ctx, archiverConfig.Streams.ConsumerGroup, streams); err != nil {
		log.Fatalf("Failed to setup streams: %v", err)
	}
	log.Println("All streams and consumer groups ready")

	// Create archiver instance
	adapter := archiver.NewValkeyStreamAdapter(valkeyClient)
	archiverInstance := archiver.NewArchiver(archiverConfig, adapter, valkeyClient, "archiver-1")

	// Register queues with Postgres write functions
	for queueName, bufferCfg := range archiverConfig.Buffers {
		streamName := "streams:" + queueName
		log.Printf("Registering queue: %s (stream: %s)\n", queueName, streamName)

		// Create write function based on queue type
		var writeFunc archiver.WriteFunc
		switch queueName {
		case "thread_step_state":
			writeFunc = pgWriter.WriteThreadStepState
		case "thread_metadata":
			writeFunc = pgWriter.WriteThreadMetadata
		case "thread_access":
			writeFunc = pgWriter.WriteThreadAccess
		case "validation_results":
			writeFunc = pgWriter.WriteValidationResults
		default:
			log.Printf("Warning: Unknown queue type %s, using default handler\n", queueName)
			writeFunc = func(ctx context.Context, events []archiver.StreamEvent) error {
				log.Printf("Skipping %d events from unknown queue %s\n", len(events), queueName)
				return nil
			}
		}

		archiverInstance.RegisterQueue(
			queueName,
			bufferCfg.Size,
			bufferCfg.FlushInterval,
			writeFunc,
		)
	}

	// Start activity worker pool for partitioned streams
	if config.Archiver.ActivityStreams.Enabled {
		workerPool := archiver.NewActivityWorkerPool(
			1, // instanceID - should be configurable for multiple instances
			config.Archiver.ActivityStreams.NumPartitions,
			config.Archiver.ActivityStreams.WorkersPerInstance,
			archiverConfig.Streams.ConsumerGroup,
			valkeyClient,
			pgWriter,
			archiverConfig.Streams.BatchSize,
			archiverConfig.Streams.BlockTimeout,
			config.Archiver.ActivityStreams.TrimEnabled,
			int64(config.Archiver.ActivityStreams.TrimMaxLen),
		)

		go func() {
			workerPool.Start(ctx)
			workerPool.Wait()
		}()

		log.Printf("Activity worker pool started: %d partitions, %d workers per instance",
			config.Archiver.ActivityStreams.NumPartitions,
			config.Archiver.ActivityStreams.WorkersPerInstance)
	}

	log.Println("Archiver service ready - starting consumers...")

	// Start the archiver (blocks until context is cancelled)
	archiverInstance.Start(ctx)

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

func getQueueNames(buffers map[string]archiver.BufferConfig) []string {
	names := make([]string, 0, len(buffers))
	for name := range buffers {
		names = append(names, name)
	}
	return names
}
