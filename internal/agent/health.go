package agent

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/misty-step/bitterblossom/internal/clients"
)

// HealthSnapshot captures lightweight host health.
type HealthSnapshot struct {
	CPUPercent     string `json:"cpu_percent"`
	MemoryPercent  string `json:"memory_percent"`
	DiskPercent    string `json:"disk_percent"`
	ClaudeRunning  bool   `json:"claude_running"`
	LoopIteration  int    `json:"loop_iteration"`
	WorkspaceBytes uint64 `json:"workspace_bytes"`
}

// HealthCollector emits health snapshots.
type HealthCollector interface {
	Collect(ctx context.Context, iteration int, claudeRunning bool) HealthSnapshot
}

// SystemHealthCollector gathers process and filesystem data using stdlib + ps.
type SystemHealthCollector struct {
	Workspace string
	Runner    clients.Runner
	CPUCores  int
}

// Collect gathers one health snapshot.
func (c *SystemHealthCollector) Collect(ctx context.Context, iteration int, claudeRunning bool) HealthSnapshot {
	cpu := c.percentSum(ctx, "%cpu", c.CPUCores)
	memory := c.percentSum(ctx, "%mem", 1)
	workspace := estimateWorkspaceSize(c.Workspace)
	return HealthSnapshot{
		CPUPercent:     cpu,
		MemoryPercent:  memory,
		DiskPercent:    diskPercent(c.Workspace),
		ClaudeRunning:  claudeRunning,
		LoopIteration:  iteration,
		WorkspaceBytes: workspace,
	}
}

func (c *SystemHealthCollector) percentSum(ctx context.Context, format string, divisor int) string {
	if c.Runner == nil {
		return "unknown"
	}
	out, _, err := c.Runner.Run(ctx, "ps", "-A", "-o", format+"=")
	if err != nil {
		return "unknown"
	}
	total := 0.0
	seen := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		v, err := strconv.ParseFloat(line, 64)
		if err != nil {
			continue
		}
		seen = true
		total += v
	}
	if !seen {
		return "unknown"
	}
	if divisor <= 0 {
		divisor = 1
	}
	pct := total / float64(divisor)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return fmt.Sprintf("%.0f%%", math.Round(pct))
}

func diskPercent(path string) string {
	if path == "" {
		return "unknown"
	}
	root := path
	for {
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}
	if root == "" {
		root = "/"
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		if err := syscall.Statfs(root, &st); err != nil {
			return "unknown"
		}
	}
	if st.Blocks == 0 {
		return "unknown"
	}
	used := st.Blocks - st.Bavail
	pct := float64(used) * 100 / float64(st.Blocks)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return fmt.Sprintf("%.0f%%", math.Round(pct))
}

func estimateWorkspaceSize(path string) uint64 {
	if path == "" {
		return 0
	}
	var total uint64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil || info.IsDir() {
			return nil
		}
		if info.Size() > 0 {
			total += uint64(info.Size())
		}
		return nil
	})
	return total
}
