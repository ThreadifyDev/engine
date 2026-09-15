//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package broker

import (
	"fmt"
	"os"
)

func lockFile(file *os.File) error {
	return fmt.Errorf("embedded NATS storage locking is unsupported on this platform; use external NATS")
}
