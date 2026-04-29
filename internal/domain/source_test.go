package domain

import (
	"strings"
	"testing"
)

func TestNormalizeSourceAppliesDefaultsAndTrimming(t *testing.T) {
	source, err := NormalizeSource(Source{
		Key:     " Example ",
		Label:   " Example Feed ",
		Kind:    " RSS ",
		URL:     " https://example.com/feed.xml ",
		Section: " Technology ",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("NormalizeSource: %v", err)
	}
	if source.Key != "example" ||
		source.Kind != SourceKindRSS ||
		source.Section != "technology" ||
		source.Threshold != ThresholdMedium ||
		source.URLCanonicalization != URLCanonicalizationNone ||
		source.OutletExtraction != OutletExtractionNone {
		t.Fatalf("source = %+v", source)
	}
}

func TestNormalizeSourceRejectsInvalidKeyWithStableMessage(t *testing.T) {
	_, err := NormalizeSource(Source{Key: "Bad/Key"})
	if err == nil || !strings.Contains(err.Error(), "source key must be lowercase letters, numbers, dot, underscore, or hyphen") {
		t.Fatalf("err = %v", err)
	}
}

func TestNormalizeSourcesRejectsDuplicateNormalizedKey(t *testing.T) {
	_, err := NormalizeSources([]Source{
		{Key: "example", Label: "One", Kind: SourceKindRSS, URL: "https://example.com/one.xml", Section: "technology"},
		{Key: "Example", Label: "Two", Kind: SourceKindRSS, URL: "https://example.com/two.xml", Section: "technology"},
	})
	if err == nil || !strings.Contains(err.Error(), `duplicate source key "example"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestNormalizeOutletPoliciesDefaultsAndDedupesAliases(t *testing.T) {
	policies, err := NormalizeOutletPolicies([]OutletPolicy{{
		Name:    " Example Outlet ",
		Aliases: []string{"Example", " example ", ""},
		Enabled: true,
	}})
	if err != nil {
		t.Fatalf("NormalizeOutletPolicies: %v", err)
	}
	if len(policies) != 1 || policies[0].Name != "Example Outlet" || policies[0].Policy != OutletPolicyAllow || len(policies[0].Aliases) != 1 {
		t.Fatalf("policies = %+v", policies)
	}
}
