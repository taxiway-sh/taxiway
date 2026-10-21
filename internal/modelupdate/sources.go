// Package modelupdate prepares reviewable catalog changes from public documents.
package modelupdate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
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

// Reference URLs are recorded in the catalog; download URLs remain provenance.
var ReferenceURLs = map[string]string{
	"codex":        "https://github.com/openai/codex/blob/main/codex-rs/models-manager/models.json",
	"chatgpt":      "https://learn.chatgpt.com/docs/models",
	"anthropic":    "https://platform.claude.com/docs/en/models/overview",
	"deprecations": "https://platform.claude.com/docs/en/about-claude/model-deprecations",
}

// chatRetirementForm is one known wording of a ChatGPT subscription retirement
// notice. A section must state exactly one notice in exactly one known form,
// with that form's own OpenAI API exclusion; anything else needs manual review.
// A notice must open a paragraph, so no qualifier ("Proposed:", "Tentative.")
// can precede it. Patterns run on one paragraph with its whitespace collapsed;
// scoped runs on that paragraph when scopeInParagraph is set, otherwise on the
// whole section body.
type chatRetirementForm struct {
	notice, scoped   *regexp.Regexp
	scopeInParagraph bool
}

// The effective form's API exclusion must be the sentence right after it, so
// a qualified or unrelated exclusion never lends it a scope.
const chatEffectiveNotice = `^Effective (?P<date>[A-Z][a-z]+ [0-9]{1,2}, [0-9]{4})(?:, at (?P<clock>noon|midnight|(?:1[0-2]|[1-9])(?::[0-5][0-9])? ?[AaPp]M) (?P<zone>P[DS]?T|E[DS]?T|C[DS]?T|M[DS]?T|UTC|GMT)(?: \((?P<utc>(?:[01]?[0-9]|2[0-3]):[0-5][0-9]) UTC\))?)?, (?P<model>(?i:GPT)-[A-Za-z0-9. -]+?) retires from Codex for users signed in with ChatGPT\.`

// Both forms retire the model from Codex signed in with ChatGPT, which is the
// `chatgpt` route, and explicitly leave the OpenAI API out of scope.
var chatRetirementForms = []chatRetirementForm{
	{
		notice: regexp.MustCompile(`^On (?P<date>[A-Z][a-z]+ [0-9]{1,2}, [0-9]{4}), (?P<model>(?i:GPT)-[A-Za-z0-9. -]+?) will retire from ChatGPT, ChatGPT Work, and Codex`),
		scoped: regexp.MustCompile(`retirement does not apply to the OpenAI API`),
	},
	{
		notice:           regexp.MustCompile(chatEffectiveNotice),
		scoped:           regexp.MustCompile(chatEffectiveNotice + ` The OpenAI API and Codex sessions authenticated with an API key (?:aren't|aren’t|are not) affected\.`),
		scopeInParagraph: true,
	},
}

// Hours behind UTC for each zone a notice may name.
var chatZoneOffsets = map[string][]int{
	"UTC": {0}, "GMT": {0},
	"EDT": {4}, "EST": {5}, "ET": {4, 5},
	"CDT": {5}, "CST": {6}, "CT": {5, 6},
	"MDT": {6}, "MST": {7}, "MT": {6, 7},
	"PDT": {7}, "PST": {8}, "PT": {7, 8},
}

// chatSameUTCDay fails when a notice's time of day is not the recorded date in
// UTC, or when its UTC conversion disagrees with its zone.
func chatSameUTCDay(clock, zone, utc string) error {
	hour, minute := 0, 0
	switch c := strings.ToUpper(strings.ReplaceAll(clock, " ", "")); c {
	case "NOON":
		hour = 12
	case "MIDNIGHT":
	default:
		if _, err := fmt.Sscanf(strings.TrimRight(c, "APM"), "%d:%d", &hour, &minute); err != nil {
			minute = 0
			if _, err = fmt.Sscanf(strings.TrimRight(c, "APM"), "%d", &hour); err != nil {
				return fmt.Errorf("chatgpt: ambiguous retirement time %q", clock)
			}
		}
		hour %= 12
		if strings.HasSuffix(c, "PM") {
			hour += 12
		}
	}
	offsets := chatZoneOffsets[zone]
	if utc != "" {
		var utcHour, utcMinute int
		if _, err := fmt.Sscanf(utc, "%d:%d", &utcHour, &utcMinute); err != nil {
			return fmt.Errorf("chatgpt: ambiguous retirement time %q", utc)
		}
		offset := (utcHour - hour + 24) % 24
		if utcMinute != minute || !slices.Contains(offsets, offset) {
			return fmt.Errorf("chatgpt: retirement time %s %s does not match %s UTC", clock, zone, utc)
		}
		offsets = []int{offset}
	}
	for _, offset := range offsets {
		if hour+offset >= 24 {
			return fmt.Errorf("chatgpt: retirement time %s %s falls on another UTC date", clock, zone)
		}
	}
	return nil
}

var chatRetirementAnnouncement = regexp.MustCompile(`(?i)\b(?:will retire|will be retired|retire[sd]? (?:from|on)|retiring|retirement of|deprecat\w*|sunset\w*|will be removed|no longer available)\b`)

// chatEffectiveTrigger finds near-miss effective notices that no known form
// reads, under any heading, so they fail closed instead of being skipped.
var chatEffectiveTrigger = regexp.MustCompile(`(?i)\beffective\b.{0,200}?\bretire`)
var chatParagraphBreak = regexp.MustCompile(`\n[ \t]*\n`)
var chatTypography = strings.NewReplacer(" ", " ", "‑", "-")
var whitespace = regexp.MustCompile(`\s+`)
var chatVersionID = regexp.MustCompile(`^gpt-[0-9.]+$`)

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
			add(m.Slug, "chatgpt", ReferenceURLs["chatgpt"])
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
				add(id, "anthropic", ReferenceURLs["anthropic"])
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
			r := Retirement{Status: status, Scope: "api", Source: ReferenceURLs["deprecations"]}
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
	// Check each Markdown section independently: one understood announcement
	// must never hide another or lend it an API-scope disclaimer.
	sections := regexp.MustCompile(`(?m)^#{1,6} `).Split(string(s.ChatGPT), -1)
	for _, section := range sections {
		// Historical summaries and links are not primary announcements.
		// A retirement heading, a future announcement or a notice in a known
		// form always requires parsing.
		heading, body, _ := strings.Cut(chatTypography.Replace(section), "\n")
		// Markdown wraps sentences at arbitrary points.
		collapse := func(s string) string { return strings.TrimSpace(whitespace.ReplaceAllString(s, " ")) }
		text := collapse(body)
		var match []string
		var form chatRetirementForm
		var paragraph string
		notices := 0
		for _, p := range chatParagraphBreak.Split(body, -1) {
			p = collapse(p)
			for _, f := range chatRetirementForms {
				if m := f.notice.FindStringSubmatch(p); m != nil {
					notices++
					match, form, paragraph = m, f, p
				}
			}
		}
		all := heading + " " + text
		lower := strings.ToLower(all)
		if notices == 0 && !strings.Contains(strings.ToLower(heading), "retirement") && !strings.Contains(lower, "will retire") && !strings.Contains(lower, "will be retired") && !chatEffectiveTrigger.MatchString(all) {
			continue
		}
		if notices != 1 || len(chatRetirementAnnouncement.FindAllString(all, -1)) != 1 {
			return d, fmt.Errorf("chatgpt: unrecognized retirement notice; manual source review required")
		}
		scope := text
		if form.scopeInParagraph {
			scope = paragraph
		}
		if !form.scoped.MatchString(scope) {
			return d, fmt.Errorf("ChatGPT retirement API scope unclear")
		}
		group := func(name string) string {
			if i := form.notice.SubexpIndex(name); i >= 0 {
				return match[i]
			}
			return ""
		}
		dt, e := date(group("date"))
		if e != nil {
			return d, e
		}
		if clock := group("clock"); clock != "" {
			if e = chatSameUTCDay(clock, group("zone"), group("utc")); e != nil {
				return d, e
			}
		}
		model := group("model")
		id := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(model)), " ", "-")
		// Plain historical version IDs are unambiguous. Suffix names must resolve
		// to an exact public Codex slug; no fuzzy family matching.
		if !chatVersionID.MatchString(id) && !seen[id] {
			return d, fmt.Errorf("chatgpt: ambiguous retirement model %q", model)
		}
		if old, ok := d.Retirements[id]; ok && old.Date != dt {
			return d, fmt.Errorf("conflicting retirement dates for %s", id)
		}
		d.Retirements[id] = Retirement{Status: "deprecated", Date: dt, Scope: "subscription", Source: ReferenceURLs["chatgpt"]}
		d.Notes = append(d.Notes, id+" retirement applies only to ChatGPT/Codex subscriptions; OpenAI API retirement is explicitly excluded. Replacement depends on account/plan and requires review.")
	}
	sort.Slice(d.Candidates, func(i, j int) bool { return d.Candidates[i].Name < d.Candidates[j].Name })
	sort.Strings(d.Notes)
	return d, nil
}
