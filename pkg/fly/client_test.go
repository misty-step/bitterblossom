package fly

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClientHappyPathOperations(t *testing.T) {
	t.Parallel()

	requests := []string{}
	client := mustClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		if got := request.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization header = %q, want Bearer token", got)
		}

		switch request.Method + " " + request.URL.Path {
		case "POST /v1/apps/test-app/machines":
			return jsonResponse(http.StatusOK, `{"id":"m1","name":"bramble","state":"started","metadata":{"persona":"bramble"}}`), nil
		case "GET /v1/apps/test-app/machines":
			return jsonResponse(http.StatusOK, `[{"id":"m1","name":"bramble","state":"started"}]`), nil
		case "GET /v1/apps/test-app/machines/m1":
			return jsonResponse(http.StatusOK, `{"id":"m1","name":"bramble","state":"started"}`), nil
		case "POST /v1/apps/test-app/machines/m1/exec":
			return jsonResponse(http.StatusOK, `{"exit_code":0,"stdout":"ok"}`), nil
		case "DELETE /v1/apps/test-app/machines/m1":
			return jsonResponse(http.StatusNoContent, ``), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
			return nil, nil
		}
	}), WithRetry(0, time.Millisecond))

	created, err := client.Create(context.Background(), CreateRequest{App: "test-app", Name: "bramble"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID != "m1" {
		t.Fatalf("created.ID = %q, want m1", created.ID)
	}

	machines, err := client.List(context.Background(), "test-app")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(machines) != 1 || machines[0].ID != "m1" {
		t.Fatalf("machines = %+v", machines)
	}

	status, err := client.Status(context.Background(), "test-app", "m1")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Name != "bramble" {
		t.Fatalf("status.Name = %q, want bramble", status.Name)
	}

	execResult, err := client.Exec(context.Background(), "test-app", "m1", ExecRequest{Command: []string{"echo", "hi"}})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if execResult.ExitCode != 0 {
		t.Fatalf("execResult.ExitCode = %d, want 0", execResult.ExitCode)
	}

	if err := client.Destroy(context.Background(), "test-app", "m1"); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}

	wantRequests := []string{
		"POST /v1/apps/test-app/machines",
		"GET /v1/apps/test-app/machines",
		"GET /v1/apps/test-app/machines/m1",
		"POST /v1/apps/test-app/machines/m1/exec",
		"DELETE /v1/apps/test-app/machines/m1",
	}
	if !reflect.DeepEqual(requests, wantRequests) {
		t.Fatalf("requests = %v, want %v", requests, wantRequests)
	}
}

func TestClientRetriesTransientStatus(t *testing.T) {
	t.Parallel()

	attempts := 0
	delays := []time.Duration{}
	client := mustClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		if attempts < 3 {
			return jsonResponse(http.StatusServiceUnavailable, `temporary`), nil
		}
		return jsonResponse(http.StatusOK, `[{"id":"m1","name":"bramble"}]`), nil
	}),
		WithRetry(3, 10*time.Millisecond),
		WithSleepFn(func(delay time.Duration) {
			delays = append(delays, delay)
		}),
	)

	machines, err := client.List(context.Background(), "test-app")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(machines) != 1 {
		t.Fatalf("len(machines) = %d, want 1", len(machines))
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	wantDelays := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}
	if !reflect.DeepEqual(delays, wantDelays) {
		t.Fatalf("delays = %v, want %v", delays, wantDelays)
	}
}

func TestClientNoRetryOnPermanentStatus(t *testing.T) {
	t.Parallel()

	attempts := 0
	client := mustClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		return jsonResponse(http.StatusBadRequest, `invalid`), nil
	}), WithRetry(3, time.Millisecond), WithSleepFn(func(time.Duration) {}))

	_, err := client.List(context.Background(), "test-app")
	if err == nil {
		t.Fatal("List() expected error, got nil")
	}
	var apiErr APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want APIError", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("apiErr.StatusCode = %d, want %d", apiErr.StatusCode, http.StatusBadRequest)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestClientValidationErrors(t *testing.T) {
	t.Parallel()

	if _, err := NewClient(""); !errors.Is(err, ErrMissingToken) {
		t.Fatalf("NewClient(\"\") error = %v, want ErrMissingToken", err)
	}

	client := mustClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{}`), nil
	}), WithRetry(0, time.Millisecond))

	if _, err := client.Create(context.Background(), CreateRequest{Name: "bramble"}); !errors.Is(err, ErrMissingApp) {
		t.Fatalf("Create() error = %v, want ErrMissingApp", err)
	}
	if err := client.Destroy(context.Background(), "", "m1"); !errors.Is(err, ErrMissingApp) {
		t.Fatalf("Destroy() error = %v, want ErrMissingApp", err)
	}
	if _, err := client.Status(context.Background(), "app", ""); !errors.Is(err, ErrMissingMachineID) {
		t.Fatalf("Status() error = %v, want ErrMissingMachineID", err)
	}
	if _, err := client.Exec(context.Background(), "app", "m1", ExecRequest{}); !errors.Is(err, ErrMissingCommand) {
		t.Fatalf("Exec() error = %v, want ErrMissingCommand", err)
	}
}

func TestMockMachineClient(t *testing.T) {
	t.Parallel()

	mock := &MockMachineClient{}
	if _, err := mock.Create(context.Background(), CreateRequest{}); !errors.Is(err, ErrMockNotImplemented) {
		t.Fatalf("Create() error = %v, want ErrMockNotImplemented", err)
	}

	called := false
	mock.CreateFn = func(context.Context, CreateRequest) (Machine, error) {
		called = true
		return Machine{ID: "m1"}, nil
	}
	machine, err := mock.Create(context.Background(), CreateRequest{App: "app", Name: "name"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !called || machine.ID != "m1" {
		t.Fatalf("Create() did not return expected machine: %+v", machine)
	}
}

func mustClient(t *testing.T, transport http.RoundTripper, opts ...Option) *Client {
	t.Helper()
	baseOpts := []Option{
		WithBaseURL("https://api.machines.dev/v1"),
		WithHTTPClient(&http.Client{Transport: transport}),
	}
	baseOpts = append(baseOpts, opts...)
	client, err := NewClient("token", baseOpts...)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
