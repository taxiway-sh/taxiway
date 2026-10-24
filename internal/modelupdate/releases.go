package modelupdate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var finalVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var gatewayImage = regexp.MustCompile(`(?m)^\s*image:\s+litellm/litellm:([^\s]+)\s*$`)

func pinnedVersion(data []byte) (string, error) {
	matches := gatewayImage.FindAllSubmatch(data, -1)
	if len(matches) != 1 || !finalVersion.Match(matches[0][1]) {
		return "", fmt.Errorf("gateway: expected exactly one final LiteLLM image tag")
	}
	return string(matches[0][1]), nil
}

func versionLess(a, b string) bool {
	x := strings.Split(strings.TrimPrefix(a, "v"), ".")
	y := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := range x {
		// Valid final versions have exactly three numeric components.
		if len(x[i]) != len(y[i]) {
			return len(x[i]) < len(y[i])
		}
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return false
}

func sortedFinalReleases(metadata map[string][]byte) []string {
	tags := []string{}
	for tag := range metadata {
		if strings.HasPrefix(tag, "v") && finalVersion.MatchString(strings.TrimPrefix(tag, "v")) {
			tags = append(tags, tag)
		}
	}
	sort.Slice(tags, func(i, j int) bool { return versionLess(tags[i], tags[j]) })
	return tags
}

type modelMetadata struct {
	Mode     string `json:"mode"`
	Provider string `json:"litellm_provider"`
}

func matchingMetadata(data []byte, id, provider string) bool {
	var prices map[string]modelMetadata
	if json.Unmarshal(data, &prices) != nil {
		return false
	}
	p, ok := prices[id]
	if !ok {
		p, ok = prices[map[string]string{"chatgpt": "openai/", "anthropic": "anthropic/"}[provider]+id]
	}
	if provider == "chatgpt" {
		provider = "openai"
	}
	return ok && p.Mode == "chat" && p.Provider == provider
}

type release struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}
