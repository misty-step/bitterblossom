package events

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// Reader consumes newline-delimited event streams.
type Reader struct {
	scanner *bufio.Scanner
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
		line := strings.TrimSpace(r.scanner.Text())
		if line == "" {
			continue
		}
		return UnmarshalEvent([]byte(line))
	}
	if err := r.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}
