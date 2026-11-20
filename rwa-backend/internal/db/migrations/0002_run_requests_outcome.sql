-- +goose Up

-- Outcome of a manual run request: a taken request must end with a visible result or go back to the queue.
--   done          the request was served (run_id says by which run; error holds a post-run problem such as a failed publish)
--   nothing_to_do mode add and every selected underlying already has a snapshot
--   rejected      the request itself is invalid (unknown symbols, unsupported mode); never retried
--   released      the collector gave the request back (another run in progress, shutdown, transient error); it is pending again
ALTER TABLE run_requests
    ADD COLUMN finished_at TIMESTAMPTZ,
    ADD COLUMN outcome     TEXT CHECK (outcome IN ('done', 'nothing_to_do', 'rejected', 'released')),
    ADD COLUMN error       TEXT;

-- +goose Down
ALTER TABLE run_requests DROP COLUMN IF EXISTS error, DROP COLUMN IF EXISTS outcome, DROP COLUMN IF EXISTS finished_at;
