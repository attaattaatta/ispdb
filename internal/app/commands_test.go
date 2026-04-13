package app

import (
	"strings"
	"testing"
)

func TestBuildCommandsUsesDefaultIPAndNS(t *testing.T) {
	t.Parallel()

	data := SourceData{
		WebDomains: []WebDomain{
			{
				ID:    "1",
				Name:  "example.com",
				Owner: "alice",
			},
		},
		EmailDomains: []EmailDomain{
			{
				ID:    "1",
				Name:  "mail.example.com",
				Owner: "alice",
			},
		},
		DNSDomains: []DNSDomain{
			{
				ID:    "1",
				Name:  "example.com",
				Owner: "alice",
			},
		},
	}

	groups, warnings := buildCommands(data, "all", CommandBuildOptions{
		DefaultIP: "203.0.113.10",
		DefaultNS: "ns1.example.com. ns2.example.com.",
	})
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	joined := strings.Join(flattenCommandGroups(groups), "\n")
	for _, want := range []string{
		"site_ipaddrs=203.0.113.10",
		"ip=203.0.113.10",
		"ipsrc=203.0.113.10",
		"'ns=ns1.example.com. ns2.example.com.'",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("generated commands do not contain %q\n%s", want, joined)
		}
	}
}
