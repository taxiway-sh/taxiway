// Package event implements the LAB_AGENT_EVENT line protocol.
//
// Wire format: a line on stdout/stderr is an event iff it matches:
//
//	^LAB_AGENT_EVENT\s+(.+)$
//
// The captured group is a single-line JSON object.
package event

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Type is the value of the "type" field in an event payload.
type Type string

const (
	TypePhase  Type = "phase"
	TypeMetric Type = "metric"
	TypeLog    Type = "log"
	TypeExit   Type = "exit"
)

const prefix = "LAB_AGENT_EVENT"

// Event represents a single parsed event.
type Event struct {
	Type      Type            `json:"type"`
	LabName   string          `json:"lab_name,omitempty"`
	Source    string          `json:"-"` // "stdout" | "stderr"
	Timestamp time.Time       `json:"ts"`
	Raw       json.RawMessage `json:"-"` // original payload
	Fields    map[string]any  `json:"-"` // parsed payload
}

// Parse returns (event, true) if line starts with "LAB_AGENT_EVENT", else (_, false).
// Unknown type values are stored in Fields; never returns an error for valid JSON.
func Parse(line string) (Event, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, prefix) {
		return Event{}, false
	}
	rest := strings.TrimSpace(line[len(prefix):])
	if rest == "" {
		return Event{}, false
	}

	// rest must be a JSON object
	if !strings.HasPrefix(rest, "{") {
		return Event{}, false
	}

	var fields map[string]any
	if err := json.Unmarshal([]byte(rest), &fields); err != nil {
		// malformed JSON — not an event
		return Event{}, false
	}

	ev := Event{
		Raw:    json.RawMessage(rest),
		Fields: fields,
	}

	if t, ok := fields["type"].(string); ok {
		ev.Type = Type(t)
	}
	if labName, ok := fields["lab_name"].(string); ok {
		ev.LabName = labName
	}
	if tsStr, ok := fields["ts"].(string); ok {
		if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
			ev.Timestamp = t
		}
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}

	return ev, true
}

// Format returns a human-readable single-line representation of the event.
func (e Event) Format() string {
	switch e.Type {
	case TypePhase:
		name, _ := e.Fields["name"].(string)
		status, _ := e.Fields["status"].(string)
		if name != "" && status != "" {
			return fmt.Sprintf("[phase] %s → %s", name, status)
		}
		// infra/trace/events.sh emits the compact "phase" field.
		phase, _ := e.Fields["phase"].(string)
		if phase != "" {
			return fmt.Sprintf("[phase] %s", phase)
		}
	case TypeMetric:
		name, _ := e.Fields["name"].(string)
		value, _ := e.Fields["value"]
		return fmt.Sprintf("[metric] %s = %v", name, value)
	case TypeLog:
		msg, _ := e.Fields["msg"].(string)
		return fmt.Sprintf("[log] %s", msg)
	case TypeExit:
		code, _ := e.Fields["code"]
		return fmt.Sprintf("[exit] code=%v", code)
	}
	return fmt.Sprintf("[%s] %s", e.Type, string(e.Raw))
}
