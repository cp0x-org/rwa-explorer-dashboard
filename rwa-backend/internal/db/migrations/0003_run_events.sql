-- +goose Up

-- Per-run journal: the steps, per-underlying results, provider requests (debug), publish, logos and retention of a run.
CREATE TABLE run_events (
    id        BIGSERIAL PRIMARY KEY,
    run_id    BIGINT NOT NULL REFERENCES collection_runs (id) ON DELETE CASCADE,
    ts        TIMESTAMPTZ NOT NULL,
    level     TEXT NOT NULL CHECK (level IN ('debug', 'info', 'warn', 'error')),
    step      TEXT NOT NULL,            -- collect.cmc | collect.portals | collect.underlying | publish | logos | retention | scheduler | run
    message   TEXT NOT NULL,
    attrs     JSONB NOT NULL DEFAULT '{}'::jsonb   -- underlying, provider, path, status, duration_ms, count, ...
);
CREATE INDEX run_events_run_idx ON run_events (run_id, id);
CREATE INDEX run_events_ts_idx ON run_events (ts);

-- +goose Down
DROP TABLE IF EXISTS run_events;
