package fly

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAPIErrorFormatting(t *testing.T) {
	t.Parallel()

	if got := (APIError{StatusCode: 500}).Error(); got != "fly api error: status 500" {
		t.Fatalf("APIError.Error() = %q", got)
	}
	if got := (APIError{StatusCode: 400, Body: " bad request "}).Error(); got != "fly api error: status 400: bad request" {
		t.Fatalf("APIError.Error() with body = %q", got)
	}
}

func TestClientListWrappedAndDecodeErrors(t *testing.T) {
	t.Parallel()

	wrapped := mustClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"machines":[{"id":"m2","name":"thorn","state":"started"}]}`), nil
	}), WithRetry(0, time.Millisecond))
	machines, err := wrapped.List(context.Background(), "test-app")
	if err != nil {
		t.Fatalf("List() wrapped response error = %v", err)
	}
	if len(machines) != 1 || machines[0].ID != "m2" {
		t.Fatalf("machines = %+v", machines)
	}

	invalid := mustClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{`), nil
	}), WithRetry(0, time.Millisecond))
	if _, err := invalid.List(context.Background(), "test-app"); err == nil {
		t.Fatal("List() expected decode error")
	}
}

func TestClientJSONDecodeAndMarshalErrors(t *testing.T) {
	t.Parallel()

	decodeClient := mustClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.Method {
		case http.MethodGet:
			return jsonResponse(http.StatusOK, `{"bad":`), nil
		case http.MethodPost:
			return jsonResponse(http.StatusOK, `{"bad":`), nil
		default:
			return jsonResponse(http.StatusOK, `{}`), nil
		}
	}), WithRetry(0, time.Millisecond))

	if _, err := decodeClient.Status(context.Background(), "app", "m1"); err == nil {
		t.Fatal("Status() expected decode response error")
	}
	if _, err := decodeClient.Exec(context.Background(), "app", "m1", ExecRequest{Command: []string{"echo"}}); err == nil {
		t.Fatal("Exec() expected decode response error")
	}

	marshalClient := mustClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{}`), nil
	}), WithRetry(0, time.Millisecond))
	_, err := marshalClient.Create(context.Background(), CreateRequest{
		App:  "app",
		Name: "name",
		Config: map[string]any{
			"bad": make(chan int),
		},
	})
	if err == nil {
		t.Fatal("Create() expected marshal payload error")
	}
}

func TestClientRequestBuildAndTransientErrorHandling(t *testing.T) {
	t.Parallel()

	badURLClient := mustClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `[]`), nil
	}), WithBaseURL("://invalid"), WithRetry(0, time.Millisecond))
	if _, err := badURLClient.List(context.Background(), "app"); err == nil {
		t.Fatal("List() expected request build error with invalid base URL")
	}

	attempts := 0
	delays := 0
	retryClient := mustClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, timeoutErr{}
		}
		return jsonResponse(http.StatusOK, `[]`), nil
	}),
		WithRetry(2, 5*time.Millisecond),
		WithSleepFn(func(time.Duration) { delays++ }),
	)
	if _, err := retryClient.List(context.Background(), "app"); err != nil {
		t.Fatalf("List() retry expected success, got %v", err)
	}
	if attempts != 2 || delays != 1 {
		t.Fatalf("attempts=%d delays=%d, want attempts=2 delays=1", attempts, delays)
	}

	canceledAttempts := 0
	noRetryClient := mustClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		canceledAttempts++
		return nil, context.Canceled
	}),
		WithRetry(3, time.Millisecond),
		WithSleepFn(func(time.Duration) { t.Fatal("sleep should not be called for context cancellation") }),
	)
	if _, err := noRetryClient.List(context.Background(), "app"); !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want context.Canceled", err)
	}
	if canceledAttempts != 1 {
		t.Fatalf("canceledAttempts = %d, want 1", canceledAttempts)
	}

	if isTransientError(context.Canceled) {
		t.Fatal("context.Canceled should not be transient")
	}
	if isTransientError(context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded should not be transient")
	}
	if !isTransientError(timeoutErr{}) {
		t.Fatal("net.Error should be transient")
	}
	if !isTransientError(errors.New("boom")) {
		t.Fatal("generic error should be treated as transient")
	}
}

func TestClientValidationAndMockMethods(t *testing.T) {
	t.Parallel()

	client := mustClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{}`), nil
	}), WithRetry(0, time.Millisecond))

	if err := client.Destroy(context.Background(), "app", ""); !errors.Is(err, ErrMissingMachineID) {
		t.Fatalf("Destroy() error = %v, want ErrMissingMachineID", err)
	}
	if _, err := client.List(context.Background(), " "); !errors.Is(err, ErrMissingApp) {
		t.Fatalf("List() error = %v, want ErrMissingApp", err)
	}
	if _, err := client.Exec(context.Background(), "", "m1", ExecRequest{Command: []string{"echo"}}); !errors.Is(err, ErrMissingApp) {
		t.Fatalf("Exec() missing app error = %v", err)
	}

	mock := &MockMachineClient{}
	if err := mock.Destroy(context.Background(), "app", "m1"); !errors.Is(err, ErrMockNotImplemented) {
		t.Fatalf("Destroy() error = %v, want ErrMockNotImplemented", err)
	}
	if _, err := mock.List(context.Background(), "app"); !errors.Is(err, ErrMockNotImplemented) {
		t.Fatalf("List() error = %v, want ErrMockNotImplemented", err)
	}
	if _, err := mock.Status(context.Background(), "app", "m1"); !errors.Is(err, ErrMockNotImplemented) {
		t.Fatalf("Status() error = %v, want ErrMockNotImplemented", err)
	}
	if _, err := mock.Exec(context.Background(), "app", "m1", ExecRequest{Command: []string{"echo"}}); !errors.Is(err, ErrMockNotImplemented) {
		t.Fatalf("Exec() error = %v, want ErrMockNotImplemented", err)
	}

	mock.DestroyFn = func(context.Context, string, string) error { return nil }
	mock.ListFn = func(context.Context, string) ([]Machine, error) { return []Machine{{ID: "x"}}, nil }
	mock.StatusFn = func(context.Context, string, string) (Machine, error) { return Machine{ID: "y"}, nil }
	mock.ExecFn = func(context.Context, string, string, ExecRequest) (ExecResult, error) {
		return ExecResult{ExitCode: 0, Stdout: "ok"}, nil
	}
	if err := mock.Destroy(context.Background(), "app", "m1"); err != nil {
		t.Fatalf("Destroy() configured error = %v", err)
	}
	if got, err := mock.List(context.Background(), "app"); err != nil || len(got) != 1 {
		t.Fatalf("List() configured = %v, %v", got, err)
	}
	if got, err := mock.Status(context.Background(), "app", "m1"); err != nil || got.ID != "y" {
		t.Fatalf("Status() configured = %+v, %v", got, err)
	}
	if got, err := mock.Exec(context.Background(), "app", "m1", ExecRequest{Command: []string{"echo"}}); err != nil || !strings.Contains(got.Stdout, "ok") {
		t.Fatalf("Exec() configured = %+v, %v", got, err)
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestDoRequestReadAndCloseErrors(t *testing.T) {
	t.Parallel()

	readErrClient := mustClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(errorReader{}),
		}, nil
	}), WithRetry(0, time.Millisecond))
	if _, err := readErrClient.List(context.Background(), "app"); err == nil {
		t.Fatal("List() expected read body error")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
