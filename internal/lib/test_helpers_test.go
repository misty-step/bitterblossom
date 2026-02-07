package lib

import (
	"context"
	"sync"
)

type mockRunner struct {
	mu       sync.Mutex
	requests []RunRequest
	results  []RunResult
	errors   []error
}

func (m *mockRunner) Run(_ context.Context, req RunRequest) (RunResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
	idx := len(m.requests) - 1

	var result RunResult
	if idx < len(m.results) {
		result = m.results[idx]
	}
	if idx < len(m.errors) && m.errors[idx] != nil {
		return result, m.errors[idx]
	}
	return result, nil
}

func (m *mockRunner) Requests() []RunRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RunRequest, len(m.requests))
	copy(out, m.requests)
	return out
}
