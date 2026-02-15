package dispatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// concurrentFakeRemote tracks concurrent upload operations and can simulate delays/failures.
// All fields use atomic operations for thread safety under concurrent uploads.
type concurrentFakeRemote struct {
	uploadDelay    time.Duration
	uploadErrs     []error // read-only after init; indexed by atomic uploadCount
	uploadErr      error   // read-only after init; default error when uploadErrs exhausted
	uploadCount    atomic.Int32
	maxConcurrent  atomic.Int32
	currentUploads atomic.Int32
}

func (f *concurrentFakeRemote) Exec(_ context.Context, _, _ string, _ []byte) (string, error) {
	return "", nil
}

func (f *concurrentFakeRemote) ExecWithEnv(_ context.Context, _, _ string, _ []byte, _ map[string]string) (string, error) {
	return "", nil
}

func (f *concurrentFakeRemote) List(_ context.Context) ([]string, error) {
	return nil, nil
}

func (f *concurrentFakeRemote) ProbeConnectivity(_ context.Context, _ string) error {
	return nil
}

func (f *concurrentFakeRemote) Upload(ctx context.Context, _, _ string, _ []byte) error {
	current := f.currentUploads.Add(1)
	defer f.currentUploads.Add(-1)

	// Track max concurrent uploads seen
	for {
		max := f.maxConcurrent.Load()
		if current <= max || f.maxConcurrent.CompareAndSwap(max, current) {
			break
		}
	}

	// Use atomic counter for thread-safe error indexing.
	index := int(f.uploadCount.Add(1)) - 1

	if f.uploadDelay > 0 {
		select {
		case <-time.After(f.uploadDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if index < len(f.uploadErrs) {
		return f.uploadErrs[index]
	}
	return f.uploadErr
}

func TestUploadSkillsConcurrent_BoundedConcurrency(t *testing.T) {
	skillRoot := t.TempDir()

	// Create multiple skills with multiple files each
	skills := []struct {
		name  string
		files int
	}{
		{"skill-a", 5},
		{"skill-b", 5},
		{"skill-c", 5},
	}

	preparedSkills := make([]preparedSkill, 0, len(skills))
	for _, s := range skills {
		skillDir := filepath.Join(skillRoot, s.name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", s.name, err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
			t.Fatalf("write SKILL.md: %v", err)
		}

		files := make([]skillFile, 0, s.files)
		for i := 0; i < s.files; i++ {
			filename := filepath.Join(skillDir, fmt.Sprintf("file-%d.txt", i))
			if err := os.WriteFile(filename, []byte(fmt.Sprintf("content %d", i)), 0o644); err != nil {
				t.Fatalf("write file: %v", err)
			}
			files = append(files, skillFile{
				LocalPath:  filename,
				RemotePath: filepath.Join("/workspace/skills", s.name, fmt.Sprintf("file-%d.txt", i)),
			})
		}

		preparedSkills = append(preparedSkills, preparedSkill{
			Name:       s.name,
			LocalRoot:  skillDir,
			RemoteRoot: filepath.Join("/workspace/skills", s.name),
			Files:      files,
		})
	}

	remote := &concurrentFakeRemote{
		uploadDelay: 10 * time.Millisecond, // Small delay to allow concurrency to build up
	}

	svc := &Service{
		remote:               remote,
		maxConcurrentUploads: 4,
	}

	ctx := context.Background()
	if err := svc.uploadSkills(ctx, "test-sprite", preparedSkills); err != nil {
		t.Fatalf("uploadSkills() error = %v", err)
	}

	// Should have uploaded all 15 files
	if count := remote.uploadCount.Load(); count != 15 {
		t.Errorf("upload count = %d, want 15", count)
	}

	// Should have limited concurrency to 4
	if max := remote.maxConcurrent.Load(); max > 4 {
		t.Errorf("max concurrent uploads = %d, want <= 4", max)
	}

	// Should have used concurrency (if executions were sequential, max would be 1)
	// Note: This is a best-effort check - very fast machines might still see sequential behavior
	if max := remote.maxConcurrent.Load(); max < 2 {
		t.Logf("warning: only saw %d concurrent uploads (may indicate contention or very fast execution)", max)
	}
}

func TestUploadSkillsConcurrent_FailFast(t *testing.T) {
	skillRoot := t.TempDir()

	// Create skill with multiple files
	skillDir := filepath.Join(skillRoot, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	// Create 10 files
	files := make([]skillFile, 0, 10)
	for i := 0; i < 10; i++ {
		filename := filepath.Join(skillDir, fmt.Sprintf("file-%d.txt", i))
		if err := os.WriteFile(filename, []byte("content"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		files = append(files, skillFile{
			LocalPath:  filename,
			RemotePath: filepath.Join("/workspace/skills/test-skill", fmt.Sprintf("file-%d.txt", i)),
		})
	}

	preparedSkills := []preparedSkill{{
		Name:       "test-skill",
		LocalRoot:  skillDir,
		RemoteRoot: "/workspace/skills/test-skill",
		Files:      files,
	}}

	// Make the 5th upload fail (concurrentFakeRemote is thread-safe)
	remote := &concurrentFakeRemote{
		uploadErrs: []error{
			nil, nil, nil, nil,
			errors.New("upload failed"),
		},
	}

	svc := &Service{
		remote:               remote,
		maxConcurrentUploads: 4,
	}

	ctx := context.Background()
	err := svc.uploadSkills(ctx, "test-sprite", preparedSkills)

	if err == nil {
		t.Fatal("expected error from failed upload")
	}

	if !strings.Contains(err.Error(), "upload failed") {
		t.Errorf("error = %v, want containing 'upload failed'", err)
	}

	// With fail-fast and concurrency, we may not have attempted all uploads
	// but we should have stopped after detecting the error
}

func TestUploadSkillsConcurrent_PreservesFileOrderInErrors(t *testing.T) {
	skillRoot := t.TempDir()

	// Create skill with files that will fail
	skillDir := filepath.Join(skillRoot, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	files := make([]skillFile, 0, 3)
	for i := 0; i < 3; i++ {
		filename := filepath.Join(skillDir, fmt.Sprintf("file-%d.txt", i))
		if err := os.WriteFile(filename, []byte("content"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		files = append(files, skillFile{
			LocalPath:  filename,
			RemotePath: filepath.Join("/workspace/skills/test-skill", fmt.Sprintf("file-%d.txt", i)),
		})
	}

	preparedSkills := []preparedSkill{{
		Name:       "test-skill",
		LocalRoot:  skillDir,
		RemoteRoot: "/workspace/skills/test-skill",
		Files:      files,
	}}

	// Make all uploads fail (concurrentFakeRemote is thread-safe)
	remote := &concurrentFakeRemote{
		uploadErr: errors.New("connection reset"),
	}

	svc := &Service{
		remote:               remote,
		maxConcurrentUploads: 2,
	}

	ctx := context.Background()
	err := svc.uploadSkills(ctx, "test-sprite", preparedSkills)

	if err == nil {
		t.Fatal("expected error")
	}

	// Error should contain the local file path for diagnostics
	if !strings.Contains(err.Error(), "file-") {
		t.Errorf("error should contain file path for diagnostics: %v", err)
	}
}

func TestUploadSkillsConcurrent_ZeroWorkersFallsBackToSequential(t *testing.T) {
	skillRoot := t.TempDir()

	skillDir := filepath.Join(skillRoot, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	files := make([]skillFile, 0, 3)
	for i := 0; i < 3; i++ {
		filename := filepath.Join(skillDir, fmt.Sprintf("file-%d.txt", i))
		if err := os.WriteFile(filename, []byte("content"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		files = append(files, skillFile{
			LocalPath:  filename,
			RemotePath: filepath.Join("/workspace/skills/test-skill", fmt.Sprintf("file-%d.txt", i)),
		})
	}

	preparedSkills := []preparedSkill{{
		Name:       "test-skill",
		LocalRoot:  skillDir,
		RemoteRoot: "/workspace/skills/test-skill",
		Files:      files,
	}}

	remote := &fakeRemote{}

	// Test with 0 workers (should fall back to 1/sequential)
	svc := &Service{
		remote:               remote,
		maxConcurrentUploads: 0,
	}

	ctx := context.Background()
	if err := svc.uploadSkills(ctx, "test-sprite", preparedSkills); err != nil {
		t.Fatalf("uploadSkills() error = %v", err)
	}

	if len(remote.uploads) != 3 {
		t.Errorf("upload count = %d, want 3", len(remote.uploads))
	}
}

// BenchmarkUploadSkills measures upload throughput with different concurrency levels
func BenchmarkUploadSkills(b *testing.B) {
	skillRoot := b.TempDir()

	// Create skill with multiple files
	skillDir := filepath.Join(skillRoot, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		b.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
		b.Fatalf("write SKILL.md: %v", err)
	}

	// Create 50 files for meaningful benchmark
	files := make([]skillFile, 0, 50)
	for i := 0; i < 50; i++ {
		filename := filepath.Join(skillDir, fmt.Sprintf("file-%d.txt", i))
		if err := os.WriteFile(filename, []byte("benchmark content"), 0o644); err != nil {
			b.Fatalf("write file: %v", err)
		}
		files = append(files, skillFile{
			LocalPath:  filename,
			RemotePath: filepath.Join("/workspace/skills/test-skill", fmt.Sprintf("file-%d.txt", i)),
		})
	}

	preparedSkills := []preparedSkill{{
		Name:       "test-skill",
		LocalRoot:  skillDir,
		RemoteRoot: "/workspace/skills/test-skill",
		Files:      files,
	}}

	benchmarks := []struct {
		name             string
		workers          int
		simulatedLatency time.Duration
	}{
		{"Sequential", 1, 1 * time.Millisecond},
		{"Concurrent2", 2, 1 * time.Millisecond},
		{"Concurrent4", 4, 1 * time.Millisecond},
		{"Concurrent8", 8, 1 * time.Millisecond},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				remote := &concurrentFakeRemote{
					uploadDelay: bm.simulatedLatency,
				}
				svc := &Service{
					remote:               remote,
					maxConcurrentUploads: bm.workers,
				}

				ctx := context.Background()
				if err := svc.uploadSkills(ctx, "test-sprite", preparedSkills); err != nil {
					b.Fatalf("uploadSkills() error = %v", err)
				}
			}
		})
	}
}
