package modelupdate

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
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

// fixtureCatalog holds the lifecycle state, comments and spacing the formatting
// tests below depend on, so they never read the live catalog.
const fixtureCatalog = "testdata/models.yaml"

// gpt55Lifecycle is the fixture's deprecated gpt-5.5 entry, which fixtureSources
// retires on 2026-10-14.
const gpt55Lifecycle = "    status: deprecated\n    retirement_date: '2026-10-14'\n    replacement: gpt-6.1-sol\n"

func readCatalog(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// replaceOnce fails the test unless old occurs exactly once, so a fixture edit
// cannot silently turn an expectation into a no-op.
func replaceOnce(t *testing.T, data []byte, old, replacement string) []byte {
	t.Helper()
	if n := bytes.Count(data, []byte(old)); n != 1 {
		t.Fatalf("%q occurs %d times, want 1", old, n)
	}
	return bytes.Replace(data, []byte(old), []byte(replacement), 1)
}

func TestPrepareLeavesUpToDateCatalogByteForByte(t *testing.T) {
	original := readCatalog(t, fixtureCatalog)
	out, _, changed, err := Prepare(original, fixtureSources(), time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	if err != nil || changed || !bytes.Equal(original, out) {
		t.Fatalf("unchanged catalog rewritten: changed=%v err=%v", changed, err)
	}
}

func TestPrepareChangesOnlyRetiredModel(t *testing.T) {
	original := readCatalog(t, fixtureCatalog)
	out, _, changed, err := Prepare(original, fixtureSources(), time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC))
	expected := replaceOnce(t, original, gpt55Lifecycle, strings.Replace(gpt55Lifecycle, "status: deprecated", "status: retired", 1))
	if err != nil || !changed || !bytes.Equal(expected, out) {
		t.Fatalf("retirement changed unrelated formatting: changed=%v err=%v\n%s", changed, err, out)
	}
}

func TestPreparePreservesInlineCommentsAndSpacing(t *testing.T) {
	commented := strings.Replace(gpt55Lifecycle, "status: deprecated", "status: deprecated  # keep this comment", 1)
	original := replaceOnce(t, readCatalog(t, fixtureCatalog), gpt55Lifecycle, commented)
	out, _, changed, err := Prepare(original, fixtureSources(), time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC))
	expected := replaceOnce(t, original, "status: deprecated  # keep this comment", "status: retired  # keep this comment")
	if err != nil || !changed || !bytes.Equal(out, expected) {
		t.Fatalf("comment or spacing changed: changed=%v err=%v\n%s", changed, err, out)
	}
}

func TestPrepareAddsRetirementFieldsWithoutReformatting(t *testing.T) {
	original := replaceOnce(t, readCatalog(t, fixtureCatalog), gpt55Lifecycle, "    status: active\n")
	out, _, changed, err := Prepare(original, fixtureSources(), time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	expected := replaceOnce(t, original,
		"    api: responses\n    status: active\n    source: https://learn.chatgpt.com/docs/models\n\n  - name: gpt-5.4",
		"    api: responses\n    status: deprecated\n    source: https://learn.chatgpt.com/docs/models\n    retirement_date: \"2026-10-14\"\n\n  - name: gpt-5.4")
	if err != nil || !changed || !bytes.Equal(out, expected) {
		t.Fatalf("new retirement changed unrelated text: changed=%v err=%v\n%s", changed, err, out)
	}
}

// syntheticModel is an active chatgpt route to gpt-5.5, which fixtureSources
// retires on 2026-10-14. lifecycle holds its status line and any retirement
// fields Prepare adds after its last line.
func syntheticModel(dash, indent, lifecycle string) string {
	return dash + "- name: zz-fixture-gpt-5.5\n" +
		indent + "provider: chatgpt\n" +
		indent + "upstream: gpt-5.5\n" +
		indent + "api: responses\n" +
		indent + lifecycle +
		indent + "source: https://learn.chatgpt.com/docs/models\n"
}

// withSyntheticRetiringModel appends syntheticModel to the catalog's models,
// at the last model's indentation, so Prepare must patch the catalog whatever
// models it lists. It returns the catalog and the entry's indentation.
func withSyntheticRetiringModel(t *testing.T, catalog []byte) ([]byte, string, string) {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal(catalog, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		t.Fatal("catalog is not a single mapping")
	}
	root := doc.Content[0]
	models := get(root, "models")
	if models == nil || models.Kind != yaml.SequenceNode || models.Style&yaml.FlowStyle != 0 || len(models.Content) == 0 {
		t.Fatal("catalog has no block sequence of models")
	}
	last := models.Content[len(models.Content)-1]
	indent := strings.Repeat(" ", last.Column-1)
	dash := strings.Repeat(" ", last.Column-3)
	// Insert before the next top-level key, or at the end of the file, so a
	// multiline value ending the last model is never cut.
	lines := strings.SplitAfter(string(catalog), "\n")
	at := len(lines)
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i+1] == models && i+2 < len(root.Content) {
			at = root.Content[i+2].Line - 1
		}
	}
	if at > 0 && lines[at-1] != "" && !strings.HasSuffix(lines[at-1], "\n") {
		lines[at-1] += "\n"
	}
	entry := "\n" + syntheticModel(dash, indent, "status: active\n")
	out := strings.Join(lines[:at], "") + entry + strings.Join(lines[at:], "")
	return []byte(out), dash, indent
}

// The live catalog changes with every reviewed update, so this test asserts
// only what holds for any valid catalog: with a synthetic entry that always
// needs a lifecycle change, Prepare accepts and patches the catalog's own
// formatting, and its proposal is a fixed point.
func TestPrepareIsIdempotentOnLiveCatalog(t *testing.T) {
	original, dash, indent := withSyntheticRetiringModel(t, readCatalog(t, "../../infra/gateway/litellm/models.yaml"))
	for _, test := range []struct {
		now    time.Time
		status string
	}{
		{time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), "deprecated"},
		{time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), "retired"},
	} {
		now := test.now
		t.Run(now.Format("2006-01-02"), func(t *testing.T) {
			out, report, changed, err := Prepare(original, fixtureSources(), now)
			if err != nil || !changed {
				t.Fatalf("synthetic retirement not applied: changed=%v err=%v", changed, err)
			}
			want := syntheticModel(dash, indent, "status: "+test.status+"\n") + indent + "retirement_date: \"2026-10-14\"\n"
			if !bytes.Contains(out, []byte(want)) || !strings.Contains(report, "Marked `zz-fixture-gpt-5.5` "+test.status) {
				t.Fatalf("synthetic route not patched as %s:\n%s\n%s", test.status, out, report)
			}
			again, _, changed, err := Prepare(out, fixtureSources(), now)
			if err != nil || changed || !bytes.Equal(out, again) {
				t.Fatalf("proposal is not a fixed point: changed=%v err=%v\n%s", changed, err, again)
			}
		})
	}
}
