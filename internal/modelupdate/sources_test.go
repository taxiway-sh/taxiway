package modelupdate

import (
	"strings"
	"testing"
)

func fixtureSources() Sources {
	return Sources{
		Codex:        []byte(`{"models":[{"slug":"gpt-6-sol","visibility":"list","input_modalities":["text"],"model_specialty":null},{"slug":"gpt-secret","visibility":"hide","input_modalities":["text"],"model_specialty":null}]}`),
		ChatGPT:      []byte("# Models\n## GPT-5.5 retirement\nOn October 14, 2026, GPT-5.5 will retire from ChatGPT, ChatGPT Work, and Codex.\nThis retirement does not apply to the OpenAI API.\n## Recommended models\nReplace `gpt-5.5`\nchoose `gpt-6-sol` when available.\n"),
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
		{"suffix", "## GPT-6 Sol retirement\nOn November 1, 2026, GPT-6 Sol will retire from ChatGPT, ChatGPT Work, and Codex.\nThis retirement does not apply to the OpenAI API.\n", false},
		{"missing scope", "## GPT-6 Sol retirement\nOn November 1, 2026, GPT-6 Sol will retire from ChatGPT, ChatGPT Work, and Codex.\n", true},
		{"unknown model", "## GPT-6 Mystery retirement\nOn November 1, 2026, GPT-6 Mystery will retire from ChatGPT, ChatGPT Work, and Codex.\nThis retirement does not apply to the OpenAI API.\n", true},
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
	const exclusion = "This retirement does not apply to the OpenAI API.\n"
	s.ChatGPT = []byte(strings.Replace(string(s.ChatGPT), exclusion, exclusion+"On November 1, 2026, GPT-6 Sol will be retired from Codex.\n", 1))
	if _, err := Discover(s); err == nil {
		t.Fatal("unrecognized second announcement ignored")
	}
}

// liveRetirementSection is the ChatGPT models page section published once the
// GPT-5.5 retirement took effect, as the Markdown source wraps it, up to the
// next heading.
const liveRetirementSection = "## GPT-5.5 retirement\n\nEffective October 14, 2026, at noon PDT (19:00 UTC), GPT-5.5 retires from\nCodex for users signed in with ChatGPT. The OpenAI API and Codex sessions\nauthenticated with an API key aren't affected.\n\nFor Codex with ChatGPT sign-in, choose an available replacement:\n\n- On Plus, Pro, Business, Enterprise, and Edu plans, choose **GPT-6.1 Sol**\n  (`gpt-6.1-sol`) when available.\n- On Free and Go plans, choose **GPT-6 Luna** (`gpt-6-luna`) in the desktop app\n  when available.\n\nReplace `gpt-5.5` in workspace defaults, saved model settings, managed\nconfigurations, custom agents, scheduled tasks, and scripts that select a model.\n\nSee [workspace model availability](https://learn.chatgpt.com/docs/enterprise/workspace-model-availability#prepare-for-the-gpt-55-retirement)\nfor administrator guidance.\n\n<ContentModeSwitch group=\"codex-surface\" id=\"app\">\n\n\n\n  \n\n\n"

// effectiveNotice is a ChatGPT models page holding the live section.
const effectiveNotice = "# Models\n" + liveRetirementSection + "## Choose a model\nChoose `gpt-6-sol`.\n"

// effectiveSentences is the effective notice and its API exclusion alone.
const effectiveSentences = "Effective October 14, 2026, at noon PDT (19:00 UTC), GPT-5.5 retires from Codex for users signed in with ChatGPT. The OpenAI API and Codex sessions authenticated with an API key aren't affected.\n"

func TestDiscoverReadsEachKnownChatGPTRetirementForm(t *testing.T) {
	for _, test := range []struct{ name, chatgpt string }{
		{"announced", string(fixtureSources().ChatGPT)},
		{"effective", effectiveNotice},
		{"effective with CRLF", strings.ReplaceAll(effectiveNotice, "\n", "\r\n")},
		{"effective without guidance", "# Models\n## GPT-5.5 retirement\n" + effectiveSentences + "## Choose a model\nChoose `gpt-6-sol`.\n"},
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

func TestDiscoverRejectsEffectiveNoticeUnderAnotherHeading(t *testing.T) {
	for _, test := range []struct{ name, chatgpt string }{
		{"other heading", "# Models\n## Codex updates\n" + effectiveSentences + "## Choose a model\nchoose `gpt-6-sol` when available.\n"},
		{"other heading without exclusion", "# Models\n## Codex updates\nEffective October 14, 2026, GPT-5.5 retires from Codex for users signed in\nwith ChatGPT.\n## Choose a model\nchoose `gpt-6-sol` when available.\n"},
		{"postponed heading", "# Models\n## GPT-5.5 retirement (postponed, dates below are no longer valid)\n" + effectiveSentences + "## Choose a model\nchoose `gpt-6-sol` when available.\n"},
		{"tentative heading", "# Models\n## Tentative Codex plan, subject to change\n" + effectiveSentences + "## Choose a model\nchoose `gpt-6-sol` when available.\n"},
		{"preamble before any heading", "Draft, not yet confirmed:\n" + effectiveSentences + "# Models\n## Choose a model\nchoose `gpt-6-sol` when available.\n"},
		{"preamble opening like a heading", "GPT-5.5 retirement\n" + effectiveSentences + "# Models\n## Choose a model\nchoose `gpt-6-sol` when available.\n"},
		{"notice split across paragraphs", "# Models\n## Codex updates\nEffective October 14, 2026.\n\nGPT-5.5 retires from Codex for users signed in with ChatGPT.\n## Choose a model\nchoose `gpt-6-sol` when available.\n"},
		{"zero-width space in effective", "# Models\n## Codex updates\nEffec" + string(rune(0x200b)) + "tive October 14, 2026, GPT-5.5 retires from Codex for users signed in with ChatGPT.\n## Choose a model\nchoose `gpt-6-sol` when available.\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := fixtureSources()
			s.ChatGPT = []byte(test.chatgpt)
			if _, err := Discover(s); err == nil || !strings.Contains(err.Error(), "unrecognized retirement notice") {
				t.Fatalf("error = %v, want unrecognized retirement notice", err)
			}
		})
	}
}

func TestDiscoverReadsHeadingCaseInsensitively(t *testing.T) {
	s := fixtureSources()
	s.ChatGPT = []byte("# Models\n## gpt-5.5 Retirement\n" + effectiveSentences + "## Choose a model\nchoose `gpt-6-sol` when available.\n")
	d, err := Discover(s)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Retirements["gpt-5.5"]; got.Date != "2026-10-14" {
		t.Fatalf("gpt-5.5 retirement = %#v", got)
	}
}

func TestDiscoverRejectsNearMissEffectiveNoticeUnderAnyHeading(t *testing.T) {
	const rest = " retires from Codex for users signed in with ChatGPT. The OpenAI API and Codex sessions authenticated with an API key aren't affected.\n"
	for _, test := range []struct{ name, section string }{
		{"a.m. time", "## Codex updates\nEffective October 14, 2026, at 10 a.m. PT, GPT-5.5" + rest},
		{"no comma before time", "## Codex updates\nEffective October 14, 2026 at noon PT, GPT-5.5" + rest},
		{"tentatively", "## Codex updates\nEffective October 14, 2026, tentatively, GPT-5.5" + rest},
		{"at the earliest", "## Codex updates\nEffective October 14, 2026, at the earliest, GPT-5.5" + rest},
		{"long qualifier", "## Codex updates\nEffective October 14, 2026, unless " + strings.Repeat("the rollout schedule changes again, ", 8) + "GPT-5.5" + rest},
		{"lowercase", "## Codex updates\neffective October 14, 2026, GPT-5.5" + rest},
		{"heading only", "## Effective October 14, 2026, GPT-5.5 retires from Codex for users signed in with ChatGPT\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := fixtureSources()
			s.ChatGPT = []byte("# Models\n" + test.section + "\n## Choose a model\nchoose `gpt-6-sol` when available.\n")
			_, err := Discover(s)
			if err == nil || !strings.Contains(err.Error(), "unrecognized retirement notice") {
				t.Fatalf("error = %v, want unrecognized retirement notice", err)
			}
		})
	}
}

func TestDiscoverReadsEffectiveNoticeTypographyAndTimes(t *testing.T) {
	const exclusion = " The OpenAI API and Codex sessions authenticated with an API key aren't affected.\n"
	for _, test := range []struct{ name, notice string }{
		{"non-breaking space in date", "Effective October" + string(rune(0x00a0)) + "14, 2026, GPT-5.5 retires from Codex for users signed in with ChatGPT."},
		{"non-breaking hyphen in model", "Effective October 14, 2026, GPT" + string(rune(0x2011)) + "5.5 retires from Codex for users signed in with ChatGPT."},
		{"morning time with conversion", "Effective October 14, 2026, at 9:30 AM EDT (13:30 UTC), GPT-5.5 retires from Codex for users signed in with ChatGPT."},
		{"zero-width characters", "Effective October 14, 2026," + string(rune(0x200d)) + " GPT-5.5 retires" + string(rune(0xfeff)) + " from Codex for users signed in with ChatGPT."},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := fixtureSources()
			s.ChatGPT = []byte("# Models\n## GPT-5.5 retirement\n" + test.notice + exclusion + "\n## Choose a model\nchoose `gpt-6-sol` when available.\n")
			d, err := Discover(s)
			if err != nil {
				t.Fatal(err)
			}
			if got := d.Retirements["gpt-5.5"]; got.Date != "2026-10-14" || got.Scope != "subscription" {
				t.Fatalf("gpt-5.5 retirement = %#v", got)
			}
		})
	}
}

func TestDiscoverSkipsDeprecatedModelsSummary(t *testing.T) {
	s := fixtureSources()
	s.ChatGPT = []byte("# Models\n## Recommended models\nChoose `gpt-6-sol`.\n\n## Deprecated Codex models\n\nGPT-5.5 retires from Codex with ChatGPT sign-in on October 14, 2026, at noon\nPDT (19:00 UTC). Replace `gpt-5.5` with a model available to your account and\nclient. API-key sessions aren't affected. See\n[GPT-5.5 retirement](https://learn.chatgpt.com/docs/models#gpt-55-retirement) for plan-specific\nreplacements and the migration checklist.\n")
	d, err := Discover(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Retirements["gpt-5.5"]; ok {
		t.Fatalf("summary treated as a notice: %#v", d.Retirements)
	}
	if len(d.Retirements) != 1 {
		t.Fatalf("retirements = %#v", d.Retirements)
	}
}

func TestDiscoverRejectsAmbiguousEffectiveRetirement(t *testing.T) {
	const notice = "Effective October 14, 2026, at noon PDT (19:00 UTC), GPT-5.5 retires from Codex for users signed in with ChatGPT."
	const exclusion = " The OpenAI API and Codex sessions authenticated with an API key aren't affected.\n"
	const announced = "On October 14, 2026, GPT-5.5 will retire from ChatGPT, ChatGPT Work, and Codex.\nThis retirement does not apply to the OpenAI API."
	_, liveBody, _ := strings.Cut(liveRetirementSection, "\n")
	const unknown = "unrecognized retirement notice"
	for _, test := range []struct{ name, section, want string }{
		{"no sign-in scope", "Effective October 14, 2026, GPT-5.5 retires from Codex." + exclusion, unknown},
		{"API scope included", "Effective October 14, 2026, GPT-5.5 retires from Codex for users signed in with ChatGPT and from the OpenAI API." + exclusion, unknown},
		{"no API exclusion", notice + "\n", unknown},
		{"other form's exclusion", notice + " This retirement does not apply to the OpenAI API.\n", unknown},
		{"qualified exclusion", notice + " Until December 1, 2026, The OpenAI API and Codex sessions authenticated with an API key aren't affected.\n", unknown},
		{"exclusion for another model", notice + " Replace `gpt-5.5`. For GPT-5.4: The OpenAI API and Codex sessions authenticated with an API key aren't affected.\n", unknown},
		{"second notice", notice + exclusion + "\nOn November 1, 2026, GPT-6 Sol will retire from ChatGPT, ChatGPT Work, and Codex.\nThis retirement does not apply to the OpenAI API.\n", unknown},
		{"trailing text, effective form", notice + exclusion + " Details follow.\n", unknown},
		{"trailing text, announced form", announced + " Details follow.\n", unknown},
		{"also retired", notice + exclusion + "\nGPT-5.4 is also retired.\n", unknown},
		{"retires next month", notice + exclusion + "\nGPT-5.4 retires next month.\n", unknown},
		{"discontinued", notice + exclusion + "\nGPT-5.4 is discontinued.\n", unknown},
		{"deprecated", notice + exclusion + "\nGPT-5.4 is deprecated.\n", unknown},
		{"qualifier paragraph", "Subject to change.\n\n" + notice + exclusion, unknown},
		{"colon qualifier", "Unless postponed: " + notice + exclusion, unknown},
		{"proposed", "Proposed: " + notice + exclusion, unknown},
		{"question qualifier", "Still undecided? " + notice + exclusion, unknown},
		{"tentative", "Tentative. " + notice + exclusion, unknown},
		{"qualified notice", "Unless postponed, " + notice + exclusion, unknown},
		{"qualified announced form", "Update: " + announced + "\n", unknown},
		{"list", "- " + notice + exclusion, unknown},
		{"code fence", "```\n" + notice + exclusion + "```\n", unknown},
		{"changed guidance", strings.Replace(liveBody, "`gpt-6.1-sol`", "`gpt-6-sol`", 1), unknown},
		{"guidance of another model", "## GPT-6 Sol retirement\n" + strings.Replace(liveBody, "GPT-5.5 retires", "GPT-6 Sol retires", 1), unknown},
		{"heading of another model", "## GPT-6 Sol retirement\n" + notice + exclusion, unknown},
		{"ambiguous date", "Effective mid-October 2026, GPT-5.5 retires from Codex for users signed in with ChatGPT." + exclusion, unknown},
		{"at the earliest", "Effective October 14, 2026, at the earliest, GPT-5.5 retires from Codex for users signed in with ChatGPT." + exclusion, unknown},
		{"at a later date", "Effective October 14, 2026, at a later date, GPT-5.5 retires from Codex for users signed in with ChatGPT." + exclusion, unknown},
		{"midnight", "Effective October 14, 2026, at midnight PT, GPT-5.5 retires from Codex for users signed in with ChatGPT." + exclusion, unknown},
		{"hour without AM or PM", "Effective October 14, 2026, at 10 PDT, GPT-5.5 retires from Codex for users signed in with ChatGPT." + exclusion, unknown},
		{"invalid hour", "Effective October 14, 2026, at 13 PM PDT, GPT-5.5 retires from Codex for users signed in with ChatGPT." + exclusion, unknown},
		{"unknown zone", "Effective October 14, 2026, at noon XYZ, GPT-5.5 retires from Codex for users signed in with ChatGPT." + exclusion, unknown},
		{"next UTC day", "Effective October 14, 2026, at 11 PM PDT (06:00 UTC), GPT-5.5 retires from Codex for users signed in with ChatGPT." + exclusion, "another UTC date"},
		{"next UTC day without conversion", "Effective October 14, 2026, at 11 PM PT, GPT-5.5 retires from Codex for users signed in with ChatGPT." + exclusion, "another UTC date"},
		{"UTC conversion mismatch", "Effective October 14, 2026, at noon PDT (20:00 UTC), GPT-5.5 retires from Codex for users signed in with ChatGPT." + exclusion, "does not match"},
		{"unknown model", "## GPT-6 Mystery retirement\nEffective October 14, 2026, GPT-6 Mystery retires from Codex for users signed in with ChatGPT." + exclusion, "ambiguous retirement model"},
	} {
		t.Run(test.name, func(t *testing.T) {
			section := test.section
			if !strings.HasPrefix(section, "## ") {
				section = "## GPT-5.5 retirement\n" + section
			}
			s := fixtureSources()
			s.ChatGPT = []byte("# Models\n" + section + "\n## Choose a model\nchoose `gpt-6-sol` when available.\n")
			_, err := Discover(s)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
