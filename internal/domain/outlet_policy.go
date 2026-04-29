package domain

import (
	"errors"
	"fmt"
	"strings"
)

const (
	OutletPolicyAllow = "allow"
	OutletPolicyBlock = "block"
	OutletPolicyWatch = "watch"
)

type OutletPolicy struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
	Policy  string   `json:"policy"`
	Note    string   `json:"note,omitempty"`
	Enabled bool     `json:"enabled"`
}

func NormalizeOutletPolicies(policies []OutletPolicy) ([]OutletPolicy, error) {
	seen := map[string]struct{}{}
	normalized := make([]OutletPolicy, 0, len(policies))
	for _, policy := range policies {
		policy.Name = strings.TrimSpace(policy.Name)
		policy.Policy = strings.TrimSpace(strings.ToLower(policy.Policy))
		policy.Note = strings.TrimSpace(policy.Note)
		if policy.Name == "" {
			return nil, errors.New("outlet policy name is required")
		}
		if policy.Policy == "" {
			policy.Policy = OutletPolicyAllow
		}
		switch policy.Policy {
		case OutletPolicyAllow, OutletPolicyBlock, OutletPolicyWatch:
		default:
			return nil, fmt.Errorf("outlet policy %q policy must be allow, block, or watch", policy.Name)
		}
		policy.Aliases = compactStrings(policy.Aliases)
		key := strings.ToLower(policy.Name)
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate outlet policy %q", policy.Name)
		}
		seen[key] = struct{}{}
		normalized = append(normalized, policy)
	}
	return normalized, nil
}

func compactStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
