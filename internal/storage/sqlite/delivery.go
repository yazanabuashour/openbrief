package sqlite

import (
	"context"
	"errors"
	"strings"
	"time"
)

type SentItem struct {
	Title  string    `json:"title"`
	URL    string    `json:"url"`
	SentAt time.Time `json:"sent_at"`
}

func (s *Store) RecentSentItems(ctx context.Context, since time.Time) ([]SentItem, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT title, url, sent_at FROM sent_item
WHERE sent_at >= ?
ORDER BY sent_at DESC, id DESC`, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	var items []SentItem
	for rows.Next() {
		var item SentItem
		var sentAt string
		if err := rows.Scan(&item.Title, &item.URL, &sentAt); err != nil {
			return nil, err
		}
		item.SentAt, _ = time.Parse(time.RFC3339Nano, sentAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) InsertDelivery(ctx context.Context, runID string, message string, items []SentItem) ([]SentItem, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, errors.New("run_id is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	deliveredAt := s.now().UTC()
	result, err := tx.ExecContext(ctx, `
INSERT INTO delivery (run_id, message, delivered_at)
VALUES (?, ?, ?)`, runID, message, deliveredAt.Format(time.RFC3339Nano))
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	deliveryID, err := result.LastInsertId()
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	for i := range items {
		items[i].SentAt = deliveredAt
		if _, err := tx.ExecContext(ctx, `
INSERT INTO sent_item (delivery_id, run_id, title, url, title_key, sent_at)
VALUES (?, ?, ?, ?, ?, ?)`,
			deliveryID, runID, items[i].Title, items[i].URL, NormalizeTitleKey(items[i].Title), deliveredAt.Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func NormalizeTitleKey(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	previousSpace := false
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			previousSpace = false
		default:
			if !previousSpace {
				b.WriteByte(' ')
				previousSpace = true
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
