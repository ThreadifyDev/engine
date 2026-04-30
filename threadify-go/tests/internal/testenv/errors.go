package testenv

import "errors"

// ErrDockerUnavailable indicates that the local environment cannot talk to Docker,
// so testcontainers-based integration tests cannot run.
var ErrDockerUnavailable = errors.New("docker unavailable")
