package modelupdate

import (
	"strings"
	"testing"
	"time"
)

func TestPinnedVersionRejectsNonFinalOrAmbiguousImages(t *testing.T) {
	for _, image := range []string{"1.2.3-dev", "1.2.3-rc.1", "main", "latest", "1.2", "1.2.3@sha256:abc"} {
		if _, err := pinnedVersion([]byte("image: litellm/litellm:" + image + "\n")); err == nil {
			t.Fatalf("accepted %s", image)
		}
	}
	if _, err := pinnedVersion([]byte("image: litellm/litellm:1.2.3\nimage: litellm/litellm:1.2.4\n")); err == nil {
		t.Fatal("ambiguous pin accepted")
	}
	if pin, err := pinnedVersion([]byte("    image: litellm/litellm:1.106.3\n")); err != nil || pin != "1.106.3" {
		t.Fatalf("%s %v", pin, err)
	}
}

func TestPrepareGatesNewModelsOnPinnedRelease(t *testing.T) {
	before := []byte(existing)
	for _, supported := range []bool{false, true} {
		s := fixtureSources()
		s.ReleaseMetadata = map[string][]byte{"v1.81.0": s.LiteLLM, "v1.80.0-rc.1": s.LiteLLM, "v1.80.0-dev": s.LiteLLM}
		if !supported {
			s.LiteLLM = []byte(`{"gpt-6-sol":{"mode":"chat","litellm_provider":"openai"}}`)
		}
		after, report, _, err := Prepare(before, s, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(after), "name: claude-opus-5-5") != supported {
			t.Fatalf("supported=%v: %s", supported, after)
		}
		if !supported && (!strings.Contains(report, "Skipped claude-opus-5-5: waiting on a LiteLLM release") || !strings.Contains(report, "Earliest final release carrying matching metadata: v1.81.0")) {
			t.Fatal(report)
		}
		if !strings.Contains(report, "/v1.79.0/model_prices_and_context_window.json") || strings.Contains(report, "/main/model_prices_and_context_window.json") {
			t.Fatal("incorrect pinned provenance")
		}
	}
}
