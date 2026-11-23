-- +goose Up

-- cp0x deals cache (the 1.x data/cp0x_deals.json): one row per successful fetch, payload = {fetched_at, deals}.
-- Publish reads the newest row; a failed fetch leaves the previous row in place.
CREATE TABLE cp0x_deals (
    id         BIGSERIAL PRIMARY KEY,
    fetched_at TIMESTAMPTZ NOT NULL,
    payload    JSONB NOT NULL
);
CREATE INDEX cp0x_deals_fetched_idx ON cp0x_deals (fetched_at DESC, id DESC);

-- +goose Down
DROP TABLE IF EXISTS cp0x_deals;
