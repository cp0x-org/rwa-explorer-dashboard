package store

import (
	"context"
	"fmt"
	"time"
)

// InsertDeals stores a deals cache document ({fetched_at, deals}) fetched at fetchedAt and returns its id.
func (s *Store) InsertDeals(ctx context.Context, fetchedAt time.Time, payload []byte) (int64, error) {
	var id int64
	err := s.q.QueryRow(ctx, `INSERT INTO raw_cp0x_deals (fetched_at, payload) VALUES ($1, $2::jsonb) RETURNING id`,
		fetchedAt, payload).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: insert deals: %w", err)
	}
	return id, nil
}

// LatestDeals returns the payload of the newest deals row, or ErrNotFound when there is none.
func (s *Store) LatestDeals(ctx context.Context) ([]byte, error) {
	var payload []byte
	err := s.q.QueryRow(ctx, `SELECT payload FROM raw_cp0x_deals ORDER BY fetched_at DESC, id DESC LIMIT 1`).Scan(&payload)
	if err != nil {
		return nil, fmt.Errorf("store: latest deals: %w", notFound(err))
	}
	return payload, nil
}

// LatestSnapshotTokenCounts returns, per underlying, the number of tokens with a crypto_id in its latest snapshot
// (any status; collect.py::_token_count). Underlyings without snapshots are absent.
func (s *Store) LatestSnapshotTokenCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.q.Query(ctx, `SELECT underlying_id, (
			SELECT count(*) FROM jsonb_array_elements(CASE WHEN jsonb_typeof(payload #> '{cmc,rwa,tokens}') = 'array'
				THEN payload #> '{cmc,rwa,tokens}' ELSE '[]'::jsonb END) t
			WHERE jsonb_typeof(t) = 'object' AND coalesce(t ->> 'crypto_id', '') NOT IN ('', '0', 'false')
		)::int
		FROM (SELECT DISTINCT ON (underlying_id) underlying_id, payload FROM raw_snapshots
			ORDER BY underlying_id, fetched_at DESC, id DESC) latest`)
	if err != nil {
		return nil, fmt.Errorf("store: latest token counts: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("store: latest token counts: scan: %w", err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: latest token counts: %w", err)
	}
	return out, nil
}
