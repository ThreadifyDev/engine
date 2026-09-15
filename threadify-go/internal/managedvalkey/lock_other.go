//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package managedvalkey

import (
	"fmt"
	"os"
)

func lockFile(file *os.File) error {
	return fmt.Errorf("managed Valkey storage locking is unsupported on this platform; use external Valkey")
}
