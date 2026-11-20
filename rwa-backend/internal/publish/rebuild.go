package publish

import (
	"context"
	"fmt"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
)

// RebuildParams describes the run row a rebuild creates.
type RebuildParams struct {
	Trigger          string // collection_runs.trigger: cli | manual
	KeySuffix        string // appended to the run_key, e.g. "-of-12"; may be empty
	CollectorVersion string
	SettingsJSON     []byte // settings.json recorded on the run
}

// Rebuild recomputes the export from the latest stored snapshots: it creates a run (mode=rebuild), publishes a build for
// it without history rows (a rebuild is not a new observation) and finishes the run. A failed publish leaves the run
// as failed and the previous build current. It returns the run id even on error once the run exists.
func (p *Publisher) Rebuild(ctx context.Context, rp RebuildParams) (int64, *Result, error) {
	st := store.New(p.pool)
	now := p.now().UTC()
	// Milliseconds keep two rebuilds started within one second apart (run_key is unique).
	runKey := "rebuild-" + now.Format("20060102T150405.000Z") + rp.KeySuffix
	runID, err := st.CreateRun(ctx, store.RunParams{RunKey: runKey, Mode: ModeRebuild, Trigger: rp.Trigger, StartedAt: now,
		CollectorVersion: rp.CollectorVersion, Settings: rp.SettingsJSON})
	if err != nil {
		return 0, nil, fmt.Errorf("publish: rebuild: %w", err)
	}
	res, err := p.Publish(ctx, runID, ModeRebuild)
	if err != nil {
		// Record the failure even when ctx was cancelled.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = st.FinishRun(fctx, runID, "failed", store.RunCounters{}, nil, err.Error())
		return runID, nil, err
	}
	if p.afterPublish != nil {
		p.afterPublish()
	}
	// The publish committed: record it even when ctx was cancelled meanwhile, or the run would stay 'running'.
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := st.FinishRun(fctx, runID, "succeeded", store.RunCounters{}, res.Warnings, ""); err != nil {
		return runID, res, fmt.Errorf("publish: rebuild: %w", err)
	}
	return runID, res, nil
}
