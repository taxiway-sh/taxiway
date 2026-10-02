// Package modelupdate prepares reviewable catalog changes from public documents.
package modelupdate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	CodexURL        = "https://raw.githubusercontent.com/openai/codex/main/codex-rs/models-manager/models.json"
	ChatGPTURL      = "https://learn.chatgpt.com/docs/models.md"
	AnthropicURL    = "https://platform.claude.com/docs/en/models/overview.md"
	DeprecationsURL = "https://platform.claude.com/docs/en/about-claude/model-deprecations.md"
	LiteLLMURL      = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
	ReleasesURL     = "https://api.github.com/repos/BerriAI/litellm/releases?per_page=10"
)

var SourceURLs = map[string]string{"codex": CodexURL, "chatgpt": ChatGPTURL, "anthropic": AnthropicURL, "deprecations": DeprecationsURL, "litellm": LiteLLMURL, "releases": ReleasesURL}

type Sources struct{ Codex, ChatGPT, Anthropic, Deprecations, LiteLLM, Releases []byte }

func (s *Sources) Set(name string, data []byte) error {
	switch name {
	case "codex":
		s.Codex = data
	case "chatgpt":
		s.ChatGPT = data
	case "anthropic":
		s.Anthropic = data
	case "deprecations":
		s.Deprecations = data
	case "litellm":
		s.LiteLLM = data
	case "releases":
		s.Releases = data
	default:
		return fmt.Errorf("unknown source %q", name)
	}
	return nil
}

type Candidate struct{ Name, Provider, Source string }
type Retirement struct{ Status, Date, Replacement, Scope, Source string }
type Discovery struct {
	Candidates  []Candidate
	Retirements map[string]Retirement
	Notes       []string
}

var chatRetirement = regexp.MustCompile(`(?s)On ([A-Z][a-z]+ [0-9]{1,2}, [0-9]{4}), (GPT-[0-9.]+) will retire from ChatGPT, ChatGPT Work, and Codex`)

func clean(s string) string { return strings.Trim(strings.TrimSpace(s), "`") }
func table(line string) []string {
	if !strings.HasPrefix(strings.TrimSpace(line), "|") {
		return nil
	}
	parts := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
	for i := range parts {
		parts[i] = clean(parts[i])
	}
	return parts
}
func date(s string) (string, error) {
	t, e := time.Parse("January 2, 2006", s)
	if e != nil {
		return "", fmt.Errorf("ambiguous retirement date %q", s)
	}
	return t.Format("2006-01-02"), nil
}

// Discover requires every source; missing models never imply retirement.
func Discover(s Sources) (Discovery, error) {
	d := Discovery{Retirements: map[string]Retirement{}}
	for name, data := range map[string][]byte{"codex": s.Codex, "chatgpt": s.ChatGPT, "anthropic": s.Anthropic, "deprecations": s.Deprecations, "litellm": s.LiteLLM, "releases": s.Releases} {
		if len(data) == 0 {
			return d, fmt.Errorf("%s: empty source", name)
		}
	}
	var codex struct {
		Models []struct {
			Slug       string          `json:"slug"`
			Visibility string          `json:"visibility"`
			Modalities []string        `json:"input_modalities"`
			Specialty  json.RawMessage `json:"model_specialty"`
		} `json:"models"`
	}
	if err := json.Unmarshal(s.Codex, &codex); err != nil || len(codex.Models) == 0 {
		return d, fmt.Errorf("codex: invalid models schema")
	}
	if !strings.Contains(string(s.ChatGPT), "# Models") {
		return d, fmt.Errorf("chatgpt: missing Models heading")
	}
	var prices map[string]struct {
		Mode     string `json:"mode"`
		Provider string `json:"litellm_provider"`
	}
	if err := json.Unmarshal(s.LiteLLM, &prices); err != nil || len(prices) == 0 {
		return d, fmt.Errorf("litellm: invalid pricing schema")
	}
	var releases []struct {
		Tag        string `json:"tag_name"`
		Published  string `json:"published_at"`
		URL        string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(s.Releases, &releases); err != nil || len(releases) == 0 {
		return d, fmt.Errorf("litellm releases: invalid schema")
	}
	stable := ""
	for _, r := range releases {
		if r.Tag == "" || r.URL == "" || r.Published == "" {
			return d, fmt.Errorf("litellm releases: missing required field")
		}
		if _, err := time.Parse(time.RFC3339, r.Published); err != nil {
			return d, err
		}
		if !r.Draft && !r.Prerelease && stable == "" {
			stable = r.Tag + " (" + r.URL + ")"
		}
	}
	if stable == "" {
		return d, fmt.Errorf("litellm releases: no stable release")
	}
	d.Notes = append(d.Notes, "Latest public LiteLLM release: "+stable+". Public metadata is not proof that the lab's pinned LiteLLM version supports a model.")
	add := func(id, provider, source string) {
		p, ok := prices[id]
		if !ok {
			p, ok = prices[map[string]string{"chatgpt": "openai/", "anthropic": "anthropic/"}[provider]+id]
		}
		expected := provider
		if expected == "chatgpt" {
			expected = "openai"
		}
		if !ok || p.Mode != "chat" || p.Provider != expected {
			d.Notes = append(d.Notes, "Skipped "+id+": no matching LiteLLM text/chat metadata; manual compatibility review required.")
			return
		}
		d.Candidates = append(d.Candidates, Candidate{id, provider, source})
	}
	seen := map[string]bool{}
	documented := 0
	for _, m := range codex.Models {
		if m.Slug == "" || m.Visibility == "" || len(m.Modalities) == 0 || len(m.Specialty) == 0 {
			return d, fmt.Errorf("codex: required model fields missing")
		}
		if seen[m.Slug] {
			return d, fmt.Errorf("codex: duplicate slug %s", m.Slug)
		}
		seen[m.Slug] = true
		if m.Visibility != "list" && m.Visibility != "hide" {
			return d, fmt.Errorf("codex: unknown visibility %q", m.Visibility)
		}
		text := false
		for _, v := range m.Modalities {
			text = text || v == "text"
		}
		if m.Visibility == "list" && text && string(m.Specialty) == "null" && strings.Contains(string(s.ChatGPT), "`"+m.Slug+"`") {
			documented++
			add(m.Slug, "chatgpt", CodexURL+"; "+ChatGPTURL)
		}
	}
	if documented == 0 {
		return d, fmt.Errorf("chatgpt: no documented public Codex model IDs; partial document or source mismatch")
	}
	if !strings.Contains(string(s.Anthropic), "# Models overview") {
		return d, fmt.Errorf("anthropic: missing heading")
	}
	apiRow := false
	for _, line := range strings.Split(string(s.Anthropic), "\n") {
		p := table(line)
		if len(p) > 1 && p[0] == "Claude API ID" {
			apiRow = true
			for _, id := range p[1:] {
				if !strings.HasPrefix(id, "claude-") {
					return d, fmt.Errorf("anthropic: malformed API ID %q", id)
				}
				if strings.Contains(id, "mythos") || strings.Contains(id, "preview") {
					d.Notes = append(d.Notes, "Skipped "+id+": specialty or restricted-access model.")
					continue
				}
				add(id, "anthropic", AnthropicURL)
			}
		}
	}
	if !apiRow {
		return d, fmt.Errorf("anthropic: missing Claude API ID row")
	}
	if !strings.Contains(string(s.Deprecations), "API model name") || !strings.Contains(string(s.Deprecations), "Current state") {
		return d, fmt.Errorf("anthropic deprecations: missing lifecycle table")
	}
	rows := 0
	for _, line := range strings.Split(string(s.Deprecations), "\n") {
		p := table(line)
		if len(p) > 0 && strings.HasPrefix(p[0], "claude-") && len(p) != 4 {
			return d, fmt.Errorf("anthropic deprecations: partial lifecycle row")
		}
		if len(p) == 4 && strings.HasPrefix(p[0], "claude-") {
			rows++
			status := strings.ToLower(p[1])
			if status != "active" && status != "deprecated" && status != "retired" {
				return d, fmt.Errorf("unknown lifecycle state %q", p[1])
			}
			if status == "active" {
				continue
			}
			r := Retirement{Status: status, Scope: "api", Source: DeprecationsURL}
			if p[3] != "To be announced" {
				v, e := date(p[3])
				if e != nil {
					return d, e
				}
				r.Date = v
			}
			if status == "retired" && r.Date == "" {
				return d, fmt.Errorf("retired model has no date")
			}
			d.Retirements[p[0]] = r
		}
	}
	if rows == 0 {
		return d, fmt.Errorf("anthropic deprecations: empty lifecycle table")
	}
	for _, line := range strings.Split(string(s.Deprecations), "\n") {
		p := table(line)
		if len(p) == 3 && strings.HasPrefix(p[1], "claude-") {
			r, ok := d.Retirements[p[1]]
			if !ok {
				continue
			}
			dt, e := date(p[0])
			if e != nil {
				return d, e
			}
			if dt != r.Date {
				return d, fmt.Errorf("conflicting retirement dates for %s", p[1])
			}
			if !strings.HasPrefix(p[2], "claude-") {
				return d, fmt.Errorf("invalid replacement")
			}
			r.Replacement = p[2]
			d.Retirements[p[1]] = r
		}
	}
	for _, match := range chatRetirement.FindAllStringSubmatch(string(s.ChatGPT), -1) {
		if !strings.Contains(string(s.ChatGPT), "retirement does not apply to the OpenAI API") {
			return d, fmt.Errorf("ChatGPT retirement API scope unclear")
		}
		dt, e := date(match[1])
		if e != nil {
			return d, e
		}
		id := strings.ToLower(match[2])
		d.Retirements[id] = Retirement{Status: "deprecated", Date: dt, Scope: "subscription", Source: ChatGPTURL}
		d.Notes = append(d.Notes, id+" retirement applies only to ChatGPT/Codex subscriptions; OpenAI API retirement is explicitly excluded. Replacement depends on account/plan and requires review.")
	}
	if strings.Contains(strings.ToLower(string(s.ChatGPT)), "retirement") && len(chatRetirement.FindAllString(string(s.ChatGPT), -1)) == 0 {
		return d, fmt.Errorf("chatgpt: unrecognized retirement notice; manual source review required")
	}
	sort.Slice(d.Candidates, func(i, j int) bool { return d.Candidates[i].Name < d.Candidates[j].Name })
	sort.Strings(d.Notes)
	return d, nil
}
