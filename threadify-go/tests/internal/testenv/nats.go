package testenv

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type NatsContainer struct {
	testcontainers.Container
	URI string
}

func StartNatsContainer(ctx context.Context) (*NatsContainer, error) {
	req := testcontainers.ContainerRequest{
		Image:        "nats:latest",
		ExposedPorts: []string{"4222/tcp"},
		Cmd:          []string{"-js"},
		WaitingFor: wait.ForLog("Server is ready").
			WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start nats container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get nats host: %w", err)
	}

	port, err := container.MappedPort(ctx, "4222/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get nats mapped port: %w", err)
	}

	return &NatsContainer{Container: container, URI: fmt.Sprintf("nats://%s:%s", host, port.Port())}, nil
}

func (n *NatsContainer) Terminate(ctx context.Context) error {
	if n == nil || n.Container == nil {
		return nil
	}
	return n.Container.Terminate(ctx)
}
