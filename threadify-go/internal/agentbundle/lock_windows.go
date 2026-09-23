package agentbundle

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"time"
)

func lockCache(ctx context.Context, path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	overlap := &windows.Overlapped{}
	for {
		err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlap)
		if err == nil {
			return func() { windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, overlap); f.Close() }, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
