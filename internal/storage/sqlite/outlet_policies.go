package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yazanabuashour/openbrief/internal/domain"
)

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
	normalized, err := domain.NormalizeOutletPolicies(policies)
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
