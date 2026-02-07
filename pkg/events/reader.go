package events

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Reader consumes newline-delimited event streams.
type Reader struct {
	scanner *bufio.Scanner
	line    int
}

// NewReader builds a JSONL event reader.
func NewReader(r io.Reader) (*Reader, error) {
	if r == nil {
		return nil, errors.New("events: reader cannot be nil")
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	return &Reader{scanner: scanner}, nil
}

// Next returns the next decoded event. io.EOF indicates end-of-stream.
func (r *Reader) Next() (Event, error) {
	for r.scanner.Scan() {
		r.line++
		line := strings.TrimSpace(r.scanner.Text())
		if line == "" {
			continue
		}
		event, err := UnmarshalEvent([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("events: decode line %d: %w", r.line, err)
		}
		return event, nil
	}
	if err := r.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// ReadAll reads all events from the supplied reader.
func ReadAll(input io.Reader) ([]Event, error) {
	reader, err := NewReader(input)
	if err != nil {
		return nil, err
	}

	events := make([]Event, 0)
	for {
		event, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return events, nil
		}
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
}
