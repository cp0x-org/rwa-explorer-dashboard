package store

import (
	"context"
	"fmt"
)

// RunBuildChanges is the newest build of a run with its diff against the previous build. The change fields are
// zero and HasChanges is false when the build has no build_changes row (the first build, a 1.x import).
type RunBuildChanges struct {
	BuildID    int64
	HasChanges bool
	BuildChangesRow
}

// BuildChangesForRuns returns, for every run in runIDs that has a build, its newest build and that build's changes.
func (s *Store) BuildChangesForRuns(ctx context.Context, runIDs []int64) (map[int64]RunBuildChanges, error) {
	out := map[int64]RunBuildChanges{}
	if len(runIDs) == 0 {
		return out, nil
	}
	rows, err := s.q.Query(ctx, `SELECT DISTINCT ON (b.run_id) b.run_id, b.id, c.build_id IS NOT NULL,
			c.previous_build_id, COALESCE(c.new_tokens, '{}'), COALESCE(c.removed_tokens, '{}'), COALESCE(c.new_issuers, '{}'),
			COALESCE(c.new_positions_count, 0), COALESCE(c.gone_positions_count, 0), COALESCE(c.eligible_delta, 0),
			COALESCE(c.details, '{}'::jsonb)
		FROM build_exports b LEFT JOIN build_changes c ON c.build_id = b.id
		WHERE b.run_id = ANY($1) ORDER BY b.run_id, b.id DESC`, runIDs)
	if err != nil {
		return nil, fmt.Errorf("store: build changes for runs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var runID int64
		var r RunBuildChanges
		c := &r.BuildChangesRow
		if err := rows.Scan(&runID, &r.BuildID, &r.HasChanges, &c.PreviousBuildID, &c.NewTokens, &c.RemovedTokens, &c.NewIssuers,
			&c.NewPositionsCount, &c.GonePositionsCount, &c.EligibleDelta, &c.Details); err != nil {
			return nil, fmt.Errorf("store: build changes for runs: scan: %w", err)
		}
		c.BuildID = r.BuildID
		out[runID] = r
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: build changes for runs: %w", err)
	}
	return out, nil
}

// GetRunRequest returns a run request or ErrNotFound.
func (s *Store) GetRunRequest(ctx context.Context, id int64) (*RunRequest, error) {
	var r RunRequest
	err := s.q.QueryRow(ctx, `SELECT id, requested_at, requested_by, mode, symbols, taken_at, run_id, finished_at, outcome, error
		FROM run_requests WHERE id = $1`, id).
		Scan(&r.ID, &r.RequestedAt, &r.RequestedBy, &r.Mode, &r.Symbols, &r.TakenAt, &r.RunID, &r.FinishedAt, &r.Outcome, &r.Error)
	if err != nil {
		return nil, fmt.Errorf("store: get run request %d: %w", id, notFound(err))
	}
	return &r, nil
}
