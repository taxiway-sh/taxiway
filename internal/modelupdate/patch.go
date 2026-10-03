package modelupdate

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// patchModels edits scalar fields at their source coordinates rather than
// re-encoding the document, preserving comments, blank lines and routing text.
func patchModels(original []byte, before, after *yaml.Node) ([]byte, error) {
	lines := strings.SplitAfter(string(original), "\n")
	type edit struct {
		line   int
		text   string
		insert bool
	}
	var edits []edit
	for i, old := range before.Content {
		next := after.Content[i]
		end := old.Line
		for _, n := range old.Content {
			if n.Line > end {
				end = n.Line
			}
		}
		var added strings.Builder
		for k := 0; k < len(next.Content); k += 2 {
			key, v := next.Content[k], next.Content[k+1]
			previous := get(old, key.Value)
			if previous != nil && previous.Value == v.Value {
				continue
			}
			var buf bytes.Buffer
			enc := yaml.NewEncoder(&buf)
			enc.SetIndent(2)
			scalarNode := *v
			scalarNode.HeadComment = ""
			scalarNode.LineComment = ""
			scalarNode.FootComment = ""
			if old.Style&yaml.FlowStyle != 0 {
				return nil, fmt.Errorf("cannot patch flow-style model %s; use block-style entries", value(old, "name"))
			}
			if err := enc.Encode(&scalarNode); err != nil {
				return nil, err
			}
			_ = enc.Close()
			scalar := strings.TrimSuffix(buf.String(), "\n")
			if strings.Contains(scalar, "\n") {
				return nil, fmt.Errorf("cannot patch multiline model field %s", key.Value)
			}
			if previous == nil {
				fmt.Fprintf(&added, "%s%s: %s\n", strings.Repeat(" ", old.Content[0].Column-1), key.Value, scalar)
				continue
			}
			if previous.Kind != yaml.ScalarNode || previous.Style == yaml.LiteralStyle || previous.Style == yaml.FoldedStyle {
				return nil, fmt.Errorf("cannot patch non-scalar model field %s", key.Value)
			}
			line := lines[previous.Line-1]
			start := previous.Column - 1
			// The parser supplies the inline comment, so retain it verbatim.
			suffix := "\n"
			if !strings.HasSuffix(line, "\n") {
				suffix = ""
			}
			if previous.LineComment != "" {
				pos := strings.LastIndex(line, previous.LineComment)
				if pos < start {
					return nil, fmt.Errorf("invalid scalar comment location")
				}
				for pos > start && (line[pos-1] == ' ' || line[pos-1] == '\t') {
					pos--
				}
				suffix = strings.TrimSuffix(line[pos:], "\n") + suffix
			}
			edits = append(edits, edit{previous.Line - 1, line[:start] + scalar + suffix, false})
		}
		if added.Len() > 0 {
			edits = append(edits, edit{end - 1, added.String(), true})
		}
	}
	if len(after.Content) > len(before.Content) {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: after.Content[len(before.Content):]}
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(seq); err != nil {
			return nil, err
		}
		_ = enc.Close()
		var appended strings.Builder
		appended.WriteString("\n")
		for _, line := range strings.SplitAfter(buf.String(), "\n") {
			if line != "" {
				if strings.HasPrefix(line, "- ") && appended.Len() > 1 {
					appended.WriteString("\n")
				}
				appended.WriteString("  " + line)
			}
		}
		// Insert after the last existing model, leaving following document sections intact.
		last := before.Content[len(before.Content)-1]
		end := last.Line
		for _, n := range last.Content {
			if n.Line > end {
				end = n.Line
			}
		}
		edits = append(edits, edit{end - 1, appended.String(), true})
	}
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].line == edits[j].line {
			return !edits[i].insert && edits[j].insert
		}
		return edits[i].line > edits[j].line
	})
	for _, e := range edits {
		if e.insert {
			if !strings.HasSuffix(lines[e.line], "\n") {
				lines[e.line] += "\n"
			}
			lines[e.line] += e.text
		} else {
			lines[e.line] = e.text
		}
	}
	return []byte(strings.Join(lines, "")), nil
}
