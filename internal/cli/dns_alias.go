package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const taxiwayDNSAliasServiceMaxLength = 42

func taxiwayServiceDNSAlias(scope, service string, parts ...string) string {
	slug := dnsAliasServiceSlug(service)
	if len(slug) > taxiwayDNSAliasServiceMaxLength {
		slug = slug[:taxiwayDNSAliasServiceMaxLength]
		slug = strings.Trim(slug, "-")
	}
	h := sha256.New()
	for _, part := range append([]string{scope, service}, parts...) {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	sum := hex.EncodeToString(h.Sum(nil))[:12]
	return "taxiway-" + sum + "-" + slug
}

func dnsAliasServiceSlug(service string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(service) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "service"
	}
	return slug
}

func proxyDNSAlias(runtime proxyRuntime) string {
	return taxiwayServiceDNSAlias("proxy", "proxy", runtime.Context, runtime.ContextID, runtime.ComposeProject, runtime.Container)
}

func labGatewayDNSAlias(runtime proxyRuntime, lab, service string) string {
	return taxiwayServiceDNSAlias("gateway", service, runtime.Context, runtime.ContextID, lab)
}

func observabilityDNSAlias(runtime observabilityRuntime, service string) string {
	return taxiwayServiceDNSAlias("observability", service, runtime.Context, runtime.ContextID, runtime.ComposeProject)
}

func observabilityDNSAliasEnvKey(service string) string {
	return "TAXIWAY_OBSERVABILITY_" + strings.ToUpper(strings.ReplaceAll(service, "-", "_")) + "_DNS"
}
