package modelupdate

import (
	"strings"
	"testing"
	"time"
)

const existing = `defaults:
  anthropic:
    opus: claude-old
models:
  - name: claude-old
    provider: anthropic
    upstream: claude-old
    forward_client_headers: true
  - name: gpt-5.5
    provider: chatgpt
    upstream: gpt-5.5
    api: responses
  - name: gpt-5.5-api
    provider: openai
    upstream: gpt-5.5
`

func TestPreparePreservesDefaultsOldModelsAndAPIRoute(t *testing.T) {
	out, report, changed, err := Prepare([]byte(existing), fixtureSources(), time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected additions")
	}
	for _, want := range []string{"opus: claude-old", "name: claude-old", "name: claude-opus-5-5", "name: gpt-6-sol", "retirement_date: \"2026-10-14\""} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("missing %q in %s", want, out)
		}
	}
	if !strings.Contains(report, "OpenAI API retirement is explicitly excluded") {
		t.Fatal(report)
	}
	again, _, changed, err := Prepare(out, fixtureSources(), time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC))
	if err != nil || changed || string(again) != string(out) {
		t.Fatalf("second run changed: %v %v", changed, err)
	}
	if strings.Count(string(out), "status: deprecated") != 1 {
		t.Fatal("API route incorrectly retired")
	}
}

func TestPrepareWarnsOnRetiredDefaultAndLeavesDefault(t *testing.T) {
	s := fixtureSources()
	s.Deprecations = []byte(strings.ReplaceAll(string(s.Deprecations), "claude-sonnet-4-5-20250929", "claude-old"))
	out, report, _, err := Prepare([]byte(existing), s, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "opus: claude-old") || !strings.Contains(report, "DEFAULT REQUIRES REVIEW") {
		t.Fatalf("%s\n%s", out, report)
	}
}

func TestPrepareReturnsNoOutputOnPartialFailure(t *testing.T) {
	s := fixtureSources()
	s.Releases = nil
	out, _, changed, err := Prepare([]byte(existing), s, time.Now())
	if err == nil || changed || out != nil {
		t.Fatal("partial update escaped")
	}
}

func TestPrepareRejectsInvalidExistingLifecycleMetadata(t *testing.T) {
	invalid := strings.Replace(existing, "    api: responses", "    api: responses\n    status: maybe", 1)
	if _, _, _, err := Prepare([]byte(invalid), fixtureSources(), time.Now()); err == nil {
		t.Fatal("invalid status accepted")
	}
	invalid = strings.Replace(existing, "    api: responses", "    api: responses\n    retirement_date: sometime", 1)
	if _, _, _, err := Prepare([]byte(invalid), fixtureSources(), time.Now()); err == nil {
		t.Fatal("invalid date accepted")
	}
}
