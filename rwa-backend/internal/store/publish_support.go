package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// LogoExts returns the extension of every stored logo keyed "kind/entity_id" (the key format of
// compute.MapLogos), without loading the image bytes.
func (s *Store) LogoExts(ctx context.Context) (map[string]string, error) {
	rows, err := s.q.Query(ctx, `SELECT kind, entity_id, ext FROM catalog_logos`)
	if err != nil {
		return nil, fmt.Errorf("store: logo exts: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var kind, id, ext string
		if err := rows.Scan(&kind, &id, &ext); err != nil {
			return nil, fmt.Errorf("store: logo exts: scan: %w", err)
		}
		out[kind+"/"+id] = ext
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: logo exts: %w", err)
	}
	return out, nil
}

// ListIssuers returns every issuers row.
func (s *Store) ListIssuers(ctx context.Context) ([]IssuerRow, error) {
	rows, err := s.q.Query(ctx, `SELECT id, name, cmc_label, website, cmc_logo_url, roster_size, roster, metadata_status, fetched_at
		FROM catalog_issuers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list issuers: %w", err)
	}
	defer rows.Close()
	var out []IssuerRow
	for rows.Next() {
		var r IssuerRow
		if err := rows.Scan(&r.ID, &r.Name, &r.CMCLabel, &r.Website, &r.CMCLogoURL, &r.RosterSize, &r.Roster, &r.MetadataStatus, &r.FetchedAt); err != nil {
			return nil, fmt.Errorf("store: list issuers: scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list issuers: %w", err)
	}
	return out, nil
}

// ListPlatformNames returns the platforms table as slug -> name.
func (s *Store) ListPlatformNames(ctx context.Context) (map[string]string, error) {
	rows, err := s.q.Query(ctx, `SELECT slug, name FROM catalog_platforms`)
	if err != nil {
		return nil, fmt.Errorf("store: list platforms: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var slug, name string
		if err := rows.Scan(&slug, &name); err != nil {
			return nil, fmt.Errorf("store: list platforms: scan: %w", err)
		}
		out[slug] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list platforms: %w", err)
	}
	return out, nil
}

// UnderlyingInfo is the CMC RWA metadata kept on an underlying (the rwa_info.json entry of 1.x).
type UnderlyingInfo struct {
	CMCRwaID   int
	Website    *string
	CMCLogoURL *string
	Industry   *string
}

// ListUnderlyingInfo returns the RWA metadata of every underlying that has a cmc_rwa_id.
func (s *Store) ListUnderlyingInfo(ctx context.Context) ([]UnderlyingInfo, error) {
	rows, err := s.q.Query(ctx, `SELECT cmc_rwa_id, website, cmc_logo_url, industry FROM catalog_underlyings
		WHERE cmc_rwa_id IS NOT NULL ORDER BY cmc_rwa_id, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list underlying info: %w", err)
	}
	defer rows.Close()
	var out []UnderlyingInfo
	for rows.Next() {
		var u UnderlyingInfo
		if err := rows.Scan(&u.CMCRwaID, &u.Website, &u.CMCLogoURL, &u.Industry); err != nil {
			return nil, fmt.Errorf("store: list underlying info: scan: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list underlying info: %w", err)
	}
	return out, nil
}

// AppendRunWarnings appends warnings to runs.warnings (a JSON array of strings).
func (s *Store) AppendRunWarnings(ctx context.Context, runID int64, warnings []string) error {
	if len(warnings) == 0 {
		return nil
	}
	b, err := json.Marshal(warnings)
	if err != nil {
		return fmt.Errorf("store: append run warnings: %w", err)
	}
	tag, err := s.q.Exec(ctx, `UPDATE runs SET warnings = warnings || $2::jsonb WHERE id = $1`, runID, b)
	if err != nil {
		return fmt.Errorf("store: append run warnings %d: %w", runID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("store: append run warnings %d: %w", runID, ErrNotFound)
	}
	return nil
}

// RunIDByKey returns the id of the run with run_key, or ErrNotFound.
func (s *Store) RunIDByKey(ctx context.Context, runKey string) (int64, error) {
	var id int64
	if err := s.q.QueryRow(ctx, `SELECT id FROM runs WHERE run_key = $1`, runKey).Scan(&id); err != nil {
		return 0, fmt.Errorf("store: run by key %s: %w", runKey, notFound(err))
	}
	return id, nil
}

// LastSuccessfulRunStart returns started_at of the newest run that finished succeeded or partial in a full-data mode
// (refresh, import) and has a published build, or ErrNotFound. `add` runs touch only a few symbols and rebuilds do not
// collect, so neither makes the basket fresh; a run whose publish failed has no build. The scheduler uses it for
// COLLECT_ON_START.
func (s *Store) LastSuccessfulRunStart(ctx context.Context) (time.Time, error) {
	var t time.Time
	err := s.q.QueryRow(ctx, `SELECT r.started_at FROM runs r
		WHERE r.status IN ('succeeded', 'partial') AND r.mode IN ('refresh', 'import')
		  AND EXISTS (SELECT 1 FROM build_exports b WHERE b.run_id = r.id)
		ORDER BY r.started_at DESC, r.id DESC LIMIT 1`).Scan(&t)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: last successful run: %w", notFound(err))
	}
	return t, nil
}
