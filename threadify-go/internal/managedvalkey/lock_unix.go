//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package managedvalkey

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}
