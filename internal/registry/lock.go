package registry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// WithLock executes fn while holding an exclusive lock for the registry path.
//
// The lock is taken on a sibling ".lock" file so registry Save() can use atomic
// rename without dropping the lock.
//
// Lock acquisition uses non-blocking flock with exponential backoff, respecting
// ctx cancellation. If ctx has a deadline, the lock attempt will time out
// accordingly; if no deadline is set, it retries indefinitely until the lock is
// acquired or ctx is cancelled.
func WithLock(ctx context.Context, path string, fn func() error) error {
	if fn == nil {
		return fmt.Errorf("registry lock: fn is nil")
	}
	validated, err := validateRegistryPath(path)
	if err != nil {
		return err
	}
	lockPath := validated + ".lock"

	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return fmt.Errorf("registry lock: create dir: %w", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("registry lock: open %q: %w", lockPath, err)
	}
	defer func() { _ = f.Close() }()

	if err := flockWithBackoff(ctx, f); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	return fn()
}

// WithLockedRegistry loads the registry under lock, calls fn, then saves it.
//
// If fn returns an error, the registry is not saved.
func WithLockedRegistry(ctx context.Context, path string, fn func(*Registry) error) error {
	return WithLock(ctx, path, func() error {
		reg, err := Load(path)
		if err != nil {
			return err
		}
		if err := fn(reg); err != nil {
			return err
		}
		return reg.Save(path)
	})
}

const (
	lockInitialBackoff = 10 * time.Millisecond
	lockMaxBackoff     = 500 * time.Millisecond
)

// flockWithBackoff attempts a non-blocking exclusive flock, retrying with
// exponential backoff until the lock is acquired or ctx is done.
func flockWithBackoff(ctx context.Context, f *os.File) error {
	backoff := lockInitialBackoff
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("registry lock: %w", ctx.Err())
		default:
		}

		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return fmt.Errorf("registry lock: flock: %w", err)
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("registry lock: %w", ctx.Err())
		case <-timer.C:
		}

		backoff *= 2
		if backoff > lockMaxBackoff {
			backoff = lockMaxBackoff
		}
	}
}
