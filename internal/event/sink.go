package event

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Sink receives parsed events.
type Sink interface {
	Handle(ctx context.Context, ev Event) error
	Close() error
}

// DiscardSink drops all events.
type DiscardSink struct{}

func (DiscardSink) Handle(_ context.Context, _ Event) error { return nil }
func (DiscardSink) Close() error                            { return nil }

// MultiSink fans out to multiple sinks. Errors from individual sinks are joined.
type MultiSink struct {
	sinks []Sink
}

func NewMultiSink(sinks ...Sink) *MultiSink {
	return &MultiSink{sinks: sinks}
}

func (m *MultiSink) Handle(ctx context.Context, ev Event) error {
	var errs []error
	for _, s := range m.sinks {
		if err := s.Handle(ctx, ev); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("sink errors: %v", errs)
	}
	return nil
}

func (m *MultiSink) Close() error {
	var errs []error
	for _, s := range m.sinks {
		if err := s.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// JSONLSink appends newline-delimited JSON events to a file.
type JSONLSink struct {
	mu   sync.Mutex
	path string
	f    *os.File
}

// NewJSONLSink creates (or appends to) the JSONL file at path.
func NewJSONLSink(path string) (*JSONLSink, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &JSONLSink{path: path, f: f}, nil
}

func (s *JSONLSink) Handle(_ context.Context, ev Event) error {
	type record struct {
		Type      Type           `json:"type"`
		LabName   string         `json:"lab_name,omitempty"`
		Source    string         `json:"source,omitempty"`
		Timestamp time.Time      `json:"ts"`
		Fields    map[string]any `json:"fields,omitempty"`
	}
	r := record{
		Type:      ev.Type,
		LabName:   ev.LabName,
		Source:    ev.Source,
		Timestamp: ev.Timestamp,
		Fields:    ev.Fields,
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = fmt.Fprintf(s.f, "%s\n", b)
	return err
}

func (s *JSONLSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f != nil {
		err := s.f.Close()
		s.f = nil
		return err
	}
	return nil
}

// StderrSink pretty-prints events to stderr (for TTY use).
type StderrSink struct {
	w io.Writer
}

func NewStderrSink() *StderrSink {
	return &StderrSink{w: os.Stderr}
}

func (s *StderrSink) Handle(_ context.Context, ev Event) error {
	_, err := fmt.Fprintf(s.w, "  \033[1;34m%s\033[0m\n", ev.Format())
	return err
}

func (s *StderrSink) Close() error { return nil }

// CollectorSink accumulates events in memory (for testing).
type CollectorSink struct {
	mu     sync.Mutex
	Events []Event
}

func (c *CollectorSink) Handle(_ context.Context, ev Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Events = append(c.Events, ev)
	return nil
}

func (c *CollectorSink) Close() error { return nil }

// SplitOutput reads from r line by line; lines matching LAB_AGENT_EVENT are
// parsed and emitted to sink (if non-nil), all other lines are written to out.
// Returns when r is closed.
func SplitOutput(ctx context.Context, r io.Reader, out io.Writer, sink Sink, source string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if ev, ok := Parse(line); ok {
			ev.Source = source
			if sink != nil {
				_ = sink.Handle(ctx, ev)
			}
			// Do not forward event lines to out
		} else {
			if out != nil {
				_, _ = fmt.Fprintln(out, line)
			}
		}
	}
}
