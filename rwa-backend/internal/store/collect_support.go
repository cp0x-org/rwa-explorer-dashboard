package store

import (
	"context"
	"fmt"
)

// UnderlyingsWithSnapshot returns the ids of underlyings that have at least one snapshot (any status).
func (s *Store) UnderlyingsWithSnapshot(ctx context.Context) (map[string]bool, error) {
	return s.idSet(ctx, "underlyings with snapshot", `SELECT DISTINCT underlying_id FROM snapshots`)
}

// KnownIssuerIDs returns the ids of issuers whose metadata came from CMC (metadata_status 'cmc').
func (s *Store) KnownIssuerIDs(ctx context.Context) (map[string]bool, error) {
	return s.idSet(ctx, "known issuers", `SELECT id FROM issuers WHERE metadata_status = 'cmc'`)
}

func (s *Store) idSet(ctx context.Context, what, query string) (map[string]bool, error) {
	rows, err := s.q.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("store: %s: %w", what, err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: %s: scan: %w", what, err)
		}
		out[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: %s: %w", what, err)
	}
	return out, nil
}

// LatestKnownRWAIDs returns, per underlying, the CMC rwa_id of its latest snapshot (any status). Underlyings
// whose latest snapshot has no CMC record are absent (1.x: previous snapshot without cmc.rwa.rwa_id).
func (s *Store) LatestKnownRWAIDs(ctx context.Context) (map[string]int, error) {
	rows, err := s.q.Query(ctx, `SELECT underlying_id, rwa_id FROM (
			SELECT DISTINCT ON (underlying_id) underlying_id, (payload #>> '{cmc,rwa,rwa_id}')::int AS rwa_id
			FROM snapshots ORDER BY underlying_id, fetched_at DESC, id DESC
		) latest WHERE rwa_id IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("store: latest rwa ids: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var rwaID int
		if err := rows.Scan(&id, &rwaID); err != nil {
			return nil, fmt.Errorf("store: latest rwa ids: scan: %w", err)
		}
		out[id] = rwaID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: latest rwa ids: %w", err)
	}
	return out, nil
}

// PlatformCount returns the number of rows in platforms.
func (s *Store) PlatformCount(ctx context.Context) (int, error) {
	var n int
	if err := s.q.QueryRow(ctx, `SELECT count(*) FROM platforms`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: platform count: %w", err)
	}
	return n, nil
}

// LogoJob is an entity that may need a logo: its CMC logo URL and official website.
type LogoJob struct {
	Kind     string // tokens | issuers | underlyings
	EntityID string // tokens: crypto_id; issuers: issuer_id; underlyings: storage id (model.StorageID)
	LogoURL  *string
	Website  *string
}

// LogoJobsMissing lists tokens, issuers and underlyings of the current state that have no logos row.
// Disabled underlyings, underlyings without a CMC record and inactive tokens are skipped.
func (s *Store) LogoJobsMissing(ctx context.Context) ([]LogoJob, error) {
	rows, err := s.q.Query(ctx, `
		SELECT 'tokens' AS kind, t.id::text AS entity_id, t.cmc_logo_url AS logo, NULL::text AS website FROM tokens t
			WHERE t.active AND NOT EXISTS (SELECT 1 FROM logos l WHERE l.kind = 'tokens' AND l.entity_id = t.id::text)
		UNION ALL
		SELECT 'issuers', i.id, i.cmc_logo_url, i.website FROM issuers i
			WHERE NOT EXISTS (SELECT 1 FROM logos l WHERE l.kind = 'issuers' AND l.entity_id = i.id)
		UNION ALL
		SELECT 'underlyings', sid, logo, website FROM (
			SELECT regexp_replace(u.id, '[^A-Za-z0-9_.-]', '_', 'g') AS sid, u.cmc_logo_url AS logo, u.website AS website
			FROM underlyings u WHERE u.enabled AND u.cmc_rwa_id IS NOT NULL
		) us WHERE NOT EXISTS (SELECT 1 FROM logos l WHERE l.kind = 'underlyings' AND l.entity_id = us.sid)
		ORDER BY 1, 2`)
	if err != nil {
		return nil, fmt.Errorf("store: logo jobs: %w", err)
	}
	defer rows.Close()
	var out []LogoJob
	for rows.Next() {
		var j LogoJob
		if err := rows.Scan(&j.Kind, &j.EntityID, &j.LogoURL, &j.Website); err != nil {
			return nil, fmt.Errorf("store: logo jobs: scan: %w", err)
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: logo jobs: %w", err)
	}
	return out, nil
}
