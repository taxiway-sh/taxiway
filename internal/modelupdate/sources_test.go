package modelupdate

import (
	"strings"
	"testing"
)

func fixtureSources() Sources {
	return Sources{
		Codex:        []byte(`{"models":[{"slug":"gpt-6-sol","visibility":"list","input_modalities":["text"],"model_specialty":null},{"slug":"gpt-secret","visibility":"hide","input_modalities":["text"],"model_specialty":null}]}`),
		ChatGPT:      []byte("# Models\n## GPT-5.5 retirement\nOn October 14, 2026, GPT-5.5 will retire from ChatGPT, ChatGPT Work, and Codex\nretirement does not apply to the OpenAI API.\nReplace `gpt-5.5`\nchoose `gpt-6-sol` when available.\n"),
		Anthropic:    []byte("# Models overview\n| Claude API ID | `claude-opus-5-5` | `claude-mythos-preview` |\n"),
		Deprecations: []byte("# Model deprecations\n| API model name | Current state | Deprecated | Tentative retirement date |\n| claude-opus-5-5 | Active | N/A | Not sooner than September 22, 2027 |\n| claude-sonnet-4-5-20250929 | Deprecated | September 30, 2026 | November 30, 2026 |\n| Retirement date | Deprecated model | Recommended replacement |\n| November 30, 2026 | `claude-sonnet-4-5-20250929` | `claude-sonnet-5-5` |\n"),
		LiteLLM:      []byte(`{"gpt-6-sol":{"mode":"chat","litellm_provider":"openai"},"claude-opus-5-5":{"mode":"chat","litellm_provider":"anthropic"}}`),
		Releases:     []byte(`[{"tag_name":"v1.80.0","published_at":"2026-10-01T00:00:00Z","html_url":"https://github.com/BerriAI/litellm/releases/tag/v1.80.0","draft":false,"prerelease":false}]`),
	}
}

func TestDiscoverPublicTextCandidatesAndScopedRetirement(t *testing.T) {
	d, err := Discover(fixtureSources())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Candidates) != 2 {
		t.Fatalf("candidates: %#v", d.Candidates)
	}
	if d.Retirements["gpt-5.5"].Scope != "subscription" {
		t.Fatalf("subscription retirement missing: %#v", d.Retirements)
	}
	if d.Retirements["claude-sonnet-4-5-20250929"].Replacement != "claude-sonnet-5-5" {
		t.Fatal("replacement missing")
	}
}

func TestDiscoverFailsClosedOnEveryMissingSource(t *testing.T) {
	for _, name := range []string{"codex", "chatgpt", "anthropic", "deprecations", "litellm", "releases"} {
		t.Run(name, func(t *testing.T) {
			s := fixtureSources()
			s.Set(name, nil)
			if _, err := Discover(s); err == nil {
				t.Fatal("missing source accepted")
			}
		})
	}
}

func TestDiscoverRejectsSchemaDriftAndTentativeRetirement(t *testing.T) {
	s := fixtureSources()
	s.Codex = []byte(`{"models":[{"slug":"gpt-6-sol"}]}`)
	if _, err := Discover(s); err == nil {
		t.Fatal("missing visibility accepted")
	}
	s = fixtureSources()
	s.Deprecations = []byte(strings.ReplaceAll(string(s.Deprecations), "November 30, 2026", "Not sooner than November 30, 2026"))
	if _, err := Discover(s); err == nil {
		t.Fatal("ambiguous deprecated date accepted")
	}
}

func TestDiscoverRejectsTruncatedDocumentsAndLifecycleRows(t *testing.T) {
	s := fixtureSources()
	s.ChatGPT = []byte("# Models\n")
	if _, err := Discover(s); err == nil {
		t.Fatal("truncated ChatGPT document accepted")
	}
	s = fixtureSources()
	s.Deprecations = append(s.Deprecations, []byte("| claude-old | Deprecated |\n")...)
	if _, err := Discover(s); err == nil {
		t.Fatal("partial lifecycle row accepted")
	}
}

func TestDiscoverChecksEveryChatGPTRetirement(t *testing.T) {
	for _, test := range []struct {
		name, notice string
		wantError    bool
	}{
		{"suffix", "## GPT-6 Sol retirement\nOn November 1, 2026, GPT-6 Sol will retire from ChatGPT, ChatGPT Work, and Codex\nretirement does not apply to the OpenAI API.\n", false},
		{"missing scope", "## GPT-6 Sol retirement\nOn November 1, 2026, GPT-6 Sol will retire from ChatGPT, ChatGPT Work, and Codex\n", true},
		{"unknown model", "## GPT-6 Mystery retirement\nOn November 1, 2026, GPT-6 Mystery will retire from ChatGPT, ChatGPT Work, and Codex\nretirement does not apply to the OpenAI API.\n", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := fixtureSources()
			s.ChatGPT = append(s.ChatGPT, []byte(test.notice)...)
			d, err := Discover(s)
			if test.wantError {
				if err == nil {
					t.Fatal("partial announcement accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(d.Retirements) != 3 || d.Retirements["gpt-6-sol"].Date != "2026-11-01" {
				t.Fatalf("announcement silently lost: %#v", d.Retirements)
			}
		})
	}
}

func TestDiscoverIgnoresHistoricalSummaryLinks(t *testing.T) {
	s := fixtureSources()
	s.ChatGPT = append(s.ChatGPT, []byte("## Deprecated Codex models\nAn older model retired previously. See [retirement notice](https://example.org).\n")...)
	d, err := Discover(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Retirements) != 2 {
		t.Fatal("summary treated as primary announcement")
	}
}

func TestDiscoverRejectsMixedAnnouncementFormatsInOneSection(t *testing.T) {
	s := fixtureSources()
	s.ChatGPT = append(s.ChatGPT, []byte("On November 1, 2026, GPT-6 Sol will be retired from Codex.\n")...)
	if _, err := Discover(s); err == nil {
		t.Fatal("unrecognized second announcement ignored")
	}
}

// effectiveNotice is the ChatGPT models page section published once the
// GPT-5.5 retirement took effect, wrapped as the Markdown source wraps it.
const effectiveNotice = "# Models\n## GPT-5.5 retirement\n\nEffective October 14, 2026, at noon PDT (19:00 UTC), GPT-5.5 retires from\nCodex for users signed in with ChatGPT. The OpenAI API and Codex sessions\nauthenticated with an API key aren't affected.\n\nFor Codex with ChatGPT sign-in, choose an available replacement:\n\n- On Plus, Pro, Business, Enterprise, and Edu plans, choose **GPT-6.1 Sol**\n  (`gpt-6.1-sol`) when available.\n- On Free and Go plans, choose **GPT-6 Luna** (`gpt-6-luna`) in the desktop app\n  when available.\n\nReplace `gpt-5.5` in workspace defaults, saved model settings, managed\nconfigurations, custom agents, scheduled tasks, and scripts that select a model.\n\n## Recommended models\nChoose `gpt-6-sol`.\n"

func TestDiscoverReadsEachKnownChatGPTRetirementForm(t *testing.T) {
	for _, test := range []struct{ name, chatgpt string }{
		{"announced", string(fixtureSources().ChatGPT)},
		{"effective", effectiveNotice},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := fixtureSources()
			s.ChatGPT = []byte(test.chatgpt)
			d, err := Discover(s)
			if err != nil {
				t.Fatal(err)
			}
			want := Retirement{Status: "deprecated", Date: "2026-10-14", Scope: "subscription", Source: ReferenceURLs["chatgpt"]}
			if got := d.Retirements["gpt-5.5"]; got != want {
				t.Fatalf("gpt-5.5 retirement = %#v, want %#v", got, want)
			}
			note := "gpt-5.5 retirement applies only to ChatGPT/Codex subscriptions; OpenAI API retirement is explicitly excluded."
			if !strings.Contains(strings.Join(d.Notes, "\n"), note) {
				t.Fatalf("API exclusion note missing: %q", d.Notes)
			}
		})
	}
}

func TestDiscoverRejectsAmbiguousEffectiveRetirement(t *testing.T) {
	const notice = "Effective October 14, 2026, at noon PDT (19:00 UTC), GPT-5.5 retires from Codex for users signed in with ChatGPT."
	const exclusion = " The OpenAI API and Codex sessions authenticated with an API key aren't affected.\n"
	for _, test := range []struct{ name, section, want string }{
		{"no sign-in scope", "Effective October 14, 2026, GPT-5.5 retires from Codex." + exclusion, "unrecognized retirement notice"},
		{"API scope included", "Effective October 14, 2026, GPT-5.5 retires from Codex for users signed in with ChatGPT and from the OpenAI API." + exclusion, "unrecognized retirement notice"},
		{"no API exclusion", notice + "\n", "API scope unclear"},
		{"other form's exclusion", notice + " This retirement does not apply to the OpenAI API.\n", "API scope unclear"},
		{"second notice", notice + exclusion + "On November 1, 2026, GPT-6 Sol will retire from ChatGPT, ChatGPT Work, and Codex\nretirement does not apply to the OpenAI API.\n", "unrecognized retirement notice"},
		{"second past retirement", notice + exclusion + "GPT-5.4 retired from Codex on August 31, 2026.\n", "unrecognized retirement notice"},
		{"unknown model", "Effective October 14, 2026, GPT-6 Mystery retires from Codex for users signed in with ChatGPT." + exclusion, "ambiguous retirement model"},
		{"ambiguous date", "Effective mid-October 2026, GPT-5.5 retires from Codex for users signed in with ChatGPT." + exclusion, "unrecognized retirement notice"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := fixtureSources()
			s.ChatGPT = []byte("# Models\n## GPT-5.5 retirement\n" + test.section + "Replace `gpt-5.5`\nchoose `gpt-6-sol` when available.\n")
			_, err := Discover(s)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
