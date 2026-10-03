package modelupdate

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/taxiway-sh/taxiway/internal/modelcatalog"
	"gopkg.in/yaml.v3"
)

func get(n *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
func value(n *yaml.Node, key string) string {
	v := get(n, key)
	if v == nil {
		return ""
	}
	return v.Value
}
func set(n *yaml.Node, key, val string) bool {
	v := get(n, key)
	if v != nil {
		if v.Value == val {
			return false
		}
		v.Value = val
		v.Tag = "!!str"
		return true
	}
	n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: val})
	return true
}
func clone(n *yaml.Node) *yaml.Node {
	c := *n
	c.Content = nil
	for _, v := range n.Content {
		c.Content = append(c.Content, clone(v))
	}
	return &c
}

// Prepare never writes files. A source error returns no proposed catalog.
// Existing order, comments, routes and defaults are retained, including old models.
func Prepare(original []byte, s Sources, now time.Time) ([]byte, string, bool, error) {
	d, err := Discover(s)
	if err != nil {
		return nil, "", false, err
	}
	var doc yaml.Node
	if err = yaml.Unmarshal(original, &doc); err != nil {
		return nil, "", false, err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, "", false, fmt.Errorf("catalog must be a mapping")
	}
	var catalog modelcatalog.Catalog
	if err = doc.Decode(&catalog); err != nil {
		return nil, "", false, err
	}
	// Defaults may need an explicit migration in the review PR. Validate model
	// records with the runtime schema, but preserve and report those defaults.
	catalog.Defaults = nil
	validation, err := yaml.Marshal(catalog)
	if err != nil {
		return nil, "", false, err
	}
	if _, err = modelcatalog.Parse(validation); err != nil {
		return nil, "", false, err
	}
	models := get(doc.Content[0], "models")
	if models == nil || models.Kind != yaml.SequenceNode || len(models.Content) == 0 {
		return nil, "", false, fmt.Errorf("catalog must contain models")
	}
	beforeModels := clone(models)
	byName := map[string]*yaml.Node{}
	templates := map[string]*yaml.Node{}
	for _, m := range models.Content {
		if m.Kind != yaml.MappingNode || value(m, "name") == "" || value(m, "provider") == "" || value(m, "upstream") == "" {
			return nil, "", false, fmt.Errorf("invalid model entry")
		}
		name := value(m, "name")
		if byName[name] != nil {
			return nil, "", false, fmt.Errorf("duplicate model %s", name)
		}
		byName[name] = m
		if templates[value(m, "provider")] == nil && value(m, "status") != "retired" {
			templates[value(m, "provider")] = m
		}
	}
	changes := []string{}
	changed := false
	for _, c := range d.Candidates {
		if byName[c.Name] != nil {
			continue
		}
		if r, ok := d.Retirements[c.Name]; ok && r.Status != "active" {
			continue
		}
		template := templates[c.Provider]
		if template == nil {
			d.Notes = append(d.Notes, "Skipped "+c.Name+": no existing "+c.Provider+" route to preserve.")
			continue
		}
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for i := 0; i+1 < len(template.Content); i += 2 {
			key := template.Content[i].Value
			switch key {
			case "provider", "api_base", "api_key", "api", "forward_client_headers":
				m.Content = append(m.Content, clone(template.Content[i]), clone(template.Content[i+1]))
			}
		}
		set(m, "name", c.Name)
		set(m, "upstream", c.Name)
		set(m, "status", "active")
		set(m, "source", c.Source)
		models.Content = append(models.Content, m)
		byName[c.Name] = m
		changed = true
		changes = append(changes, "Added `"+c.Name+"` ("+c.Provider+").")
	}
	for _, m := range models.Content {
		r, ok := d.Retirements[value(m, "upstream")]
		if !ok {
			continue
		}
		provider := value(m, "provider")
		if (r.Scope == "subscription" && provider != "chatgpt") || (r.Scope == "api" && provider != "anthropic") {
			continue
		}
		status := r.Status
		if r.Date != "" {
			dt, _ := time.Parse("2006-01-02", r.Date)
			if !now.Before(dt) {
				status = "retired"
			}
		}
		local := set(m, "status", status)
		if r.Date != "" {
			local = set(m, "retirement_date", r.Date) || local
		}
		if r.Replacement != "" {
			local = set(m, "replacement", r.Replacement) || local
		}
		if local {
			set(m, "source", r.Source)
		}
		if local {
			changed = true
			changes = append(changes, "Marked `"+value(m, "name")+"` "+status+" ("+r.Scope+", retirement "+r.Date+").")
		}
		if status == "retired" {
			d.Notes = append(d.Notes, "Review orchestrator manifest defaults selecting `"+value(m, "name")+"`; automatic default replacement is intentionally deferred.")
		}
	}
	defaults := get(doc.Content[0], "defaults")
	if defaults != nil {
		for i := 1; i < len(defaults.Content); i += 2 {
			family := defaults.Content[i]
			for j := 1; j < len(family.Content); j += 2 {
				id := family.Content[j].Value
				if m := byName[id]; m != nil && (value(m, "status") == "retired" || value(m, "status") == "deprecated") {
					d.Notes = append(d.Notes, "DEFAULT REQUIRES REVIEW: `"+id+"` remains selected. Choose and validate a replacement explicitly.")
				}
			}
		}
	}
	out := original
	if changed {
		out, err = patchModels(original, beforeModels, models)
		if err != nil {
			return nil, "", false, err
		}
	}
	sort.Strings(changes)
	sort.Strings(d.Notes)
	var report strings.Builder
	report.WriteString("## Model catalog review\n\nPublic-source preparation only; no authenticated inference or account availability checks. Defaults require explicit review. No automerge.\n\n")
	if len(changes) == 0 {
		report.WriteString("No catalog changes.\n\n")
	} else {
		for _, c := range changes {
			fmt.Fprintf(&report, "- %s\n", c)
		}
		report.WriteString("\n")
	}
	report.WriteString("### Provenance\n\n")
	for _, name := range []string{"codex", "chatgpt", "anthropic", "deprecations", "litellm", "releases"} {
		var data []byte
		switch name {
		case "codex":
			data = s.Codex
		case "chatgpt":
			data = s.ChatGPT
		case "anthropic":
			data = s.Anthropic
		case "deprecations":
			data = s.Deprecations
		case "litellm":
			data = s.LiteLLM
		case "releases":
			data = s.Releases
		}
		fmt.Fprintf(&report, "- [%s](%s), SHA-256 `%x`\n", name, SourceURLs[name], sha256.Sum256(data))
	}
	report.WriteString("\n### Uncertainty and review\n\n- Account access, rollout, public snapshots, and catalog absence do not prove general availability or retirement.\n- Existing supported models and defaults are preserved. Validate routing and protocol behavior before merge.\n- OpenAI API lifecycle is separate from ChatGPT subscription lifecycle; this updater creates no API-key routes.\n")
	for _, n := range d.Notes {
		fmt.Fprintf(&report, "- %s\n", n)
	}
	return out, report.String(), changed, nil
}
