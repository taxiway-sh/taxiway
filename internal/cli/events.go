package cli

import (
	"github.com/taxiway-sh/taxiway/internal/event"
)

// makeExecSink creates a JSONLSink that records events to a JSONL file.
// Returns a closer func that must be called when the sink is no longer needed.
func makeExecSink(jsonlPath string) (event.Sink, func(), error) {
	jsonlSink, err := event.NewJSONLSink(jsonlPath)
	if err != nil {
		return event.DiscardSink{}, func() {}, err
	}
	return jsonlSink, func() { jsonlSink.Close() }, nil
}
