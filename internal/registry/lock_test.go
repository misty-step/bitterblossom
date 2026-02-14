package registry

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWithLock_BasicExclusion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "registry.toml")
	ctx := context.Background()

	executed := false
	if err := WithLock(ctx, path, func() error {
		executed = true
		return nil
	}); err != nil {
		t.Fatalf("WithLock() error = %v", err)
	}
	if !executed {
		t.Fatal("fn was not executed")
	}
}

func TestWithLock_ContextCancellation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "registry.toml")
	lockPath := path + ".lock"

	// Hold the lock from another file descriptor.
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("Flock() error = %v", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	// Try to acquire the lock with a short-lived context.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = WithLock(ctx, path, func() error {
		t.Fatal("fn should not be called when lock is held")
		return nil
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected context deadline error, got %q", err.Error())
	}
}

func TestWithLock_AlreadyCancelledContext(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "registry.toml")
	lockPath := path + ".lock"

	// Hold the lock.
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("Flock() error = %v", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	// Already-cancelled context should fail on first retry.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = WithLock(ctx, path, func() error {
		t.Fatal("fn should not be called")
		return nil
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context canceled error, got %q", err.Error())
	}
}

func TestWithLock_AcquiresAfterRelease(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "registry.toml")
	lockPath := path + ".lock"

	// Hold lock, release after 30ms.
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("Flock() error = %v", err)
	}

	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	executed := false
	if err := WithLock(ctx, path, func() error {
		executed = true
		return nil
	}); err != nil {
		t.Fatalf("WithLock() error = %v", err)
	}
	if !executed {
		t.Fatal("fn should have executed after lock release")
	}
}

func TestWithLockedRegistry_ContextAware(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "registry.toml")
	lockPath := path + ".lock"

	// Hold the lock.
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("Flock() error = %v", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = WithLockedRegistry(ctx, path, func(reg *Registry) error {
		t.Fatal("fn should not be called")
		return nil
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected context deadline error, got %q", err.Error())
	}
}

func TestWithLock_NilFn(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "registry.toml")
	err := WithLock(context.Background(), path, nil)
	if err == nil || !strings.Contains(err.Error(), "fn is nil") {
		t.Fatalf("expected nil fn error, got %v", err)
	}
}
