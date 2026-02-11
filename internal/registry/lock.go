package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// WithLock executes fn while holding an exclusive lock for the registry path.
//
// The lock is taken on a sibling ".lock" file so registry Save() can use atomic
// rename without dropping the lock.
func WithLock(path string, fn func() error) error {
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

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("registry lock: flock: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	return fn()
}

// WithLockedRegistry loads the registry under lock, calls fn, then saves it.
//
// If fn returns an error, the registry is not saved.
func WithLockedRegistry(path string, fn func(*Registry) error) error {
	return WithLock(path, func() error {
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
