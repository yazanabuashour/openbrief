package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type OutletPolicy struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
	Policy  string   `json:"policy"`
	Note    string   `json:"note,omitempty"`
	Enabled bool     `json:"enabled"`
}

func (s *Store) ListOutletPolicies(ctx context.Context) ([]OutletPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, aliases_json, policy, note, enabled FROM outlet_policy ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	var policies []OutletPolicy
	for rows.Next() {
		var policy OutletPolicy
		var aliases string
		var enabled int
		if err := rows.Scan(&policy.Name, &aliases, &policy.Policy, &policy.Note, &enabled); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(aliases), &policy.Aliases)
		policy.Enabled = enabled == 1
		policies = append(policies, policy)
	}
	return policies, rows.Err()
}

func (s *Store) ReplaceOutletPolicies(ctx context.Context, policies []OutletPolicy) ([]OutletPolicy, error) {
	normalized, err := normalizeOutletPolicies(policies)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM outlet_policy`); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	now := s.now().Format(time.RFC3339Nano)
	for _, policy := range normalized {
		aliases, err := json.Marshal(policy.Aliases)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO outlet_policy (name, aliases_json, policy, note, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			policy.Name, string(aliases), policy.Policy, policy.Note, boolInt(policy.Enabled), now, now); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListOutletPolicies(ctx)
}

func normalizeOutletPolicies(policies []OutletPolicy) ([]OutletPolicy, error) {
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
			policy.Policy = "allow"
		}
		switch policy.Policy {
		case "allow", "block", "watch":
		default:
			return nil, fmt.Errorf("outlet policy %q policy must be allow, block, or watch", policy.Name)
		}
		for i := range policy.Aliases {
			policy.Aliases[i] = strings.TrimSpace(policy.Aliases[i])
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
