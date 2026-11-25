-- +goose Up

-- Table groups (the prefix says what a table is for):
--   runs, run_*     the collection process: runs, per-underlying results, manual requests, the run journal
--   raw_*           source responses as fetched: snapshots (CMC + Portals per underlying), cp0x deals
--   catalog_*       every entity ever seen with its latest facts (active flag); history rows reference them
--   history_*       one row per entity and run: the charts
--   build_*         the published export (is_current = what the API and the site serve) and its changes

-- ---------- запуски сбора: runs, run_* ----------
CREATE TABLE runs (
    id                BIGSERIAL PRIMARY KEY,
    run_key           TEXT NOT NULL UNIQUE,                 -- 20261018T201453Z, как run_id в 1.x
    mode              TEXT NOT NULL CHECK (mode IN ('refresh', 'add', 'import', 'rebuild')),
    trigger           TEXT NOT NULL CHECK (trigger IN ('schedule', 'startup', 'manual', 'import', 'cli')),
    status            TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'partial', 'failed')),
    started_at        TIMESTAMPTZ NOT NULL,
    finished_at       TIMESTAMPTZ,
    selected_count    INT NOT NULL DEFAULT 0,
    completed_count   INT NOT NULL DEFAULT 0,
    failed_count      INT NOT NULL DEFAULT 0,
    cmc_requests      INT NOT NULL DEFAULT 0,
    portals_requests  INT NOT NULL DEFAULT 0,
    cmc_credits       INT NOT NULL DEFAULT 0,
    warnings          JSONB NOT NULL DEFAULT '[]'::jsonb,
    error             TEXT,                                 -- фатальная ошибка run
    collector_version TEXT NOT NULL,
    settings          JSONB NOT NULL                        -- settings.json, с которым шёл run
);
CREATE INDEX runs_started_at_idx ON runs (started_at DESC);

CREATE TABLE run_underlyings (
    run_id            BIGINT NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    underlying_id     TEXT NOT NULL,
    status            TEXT NOT NULL,   -- ok | no_tracked_tokens | not_in_cmc | rwa_id_changed | ambiguous_cmc_match | failed | skipped
    error             TEXT,
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ,
    portals_requests  INT NOT NULL DEFAULT 0,
    PRIMARY KEY (run_id, underlying_id)
);

-- ручной запуск: api пишет заявку, collector забирает
CREATE TABLE run_requests (
    id            BIGSERIAL PRIMARY KEY,
    requested_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    requested_by  TEXT NOT NULL,                            -- 'admin-api' | 'cli'
    mode          TEXT NOT NULL DEFAULT 'refresh' CHECK (mode IN ('refresh', 'add', 'rebuild')),
    symbols       TEXT[],                                   -- NULL = все enabled
    taken_at      TIMESTAMPTZ,
    run_id        BIGINT REFERENCES runs (id),
    finished_at   TIMESTAMPTZ,
    -- done | nothing_to_do (mode add, every selected underlying has a snapshot) | rejected (invalid, never retried)
    -- | released (given back: another run in progress, shutdown, transient error; pending again)
    outcome       TEXT CHECK (outcome IN ('done', 'nothing_to_do', 'rejected', 'released')),
    error         TEXT                                      -- post-run problem of a done request, e.g. a failed publish
);
CREATE INDEX run_requests_pending_idx ON run_requests (requested_at) WHERE taken_at IS NULL;

-- Per-run journal: the steps, per-underlying results, provider requests (debug), publish, logos and retention of a run.
CREATE TABLE run_events (
    id        BIGSERIAL PRIMARY KEY,
    run_id    BIGINT NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    ts        TIMESTAMPTZ NOT NULL,
    level     TEXT NOT NULL CHECK (level IN ('debug', 'info', 'warn', 'error')),
    step      TEXT NOT NULL,            -- collect.cmc | collect.portals | collect.underlying | publish | logos | retention | scheduler | run
    message   TEXT NOT NULL,
    attrs     JSONB NOT NULL DEFAULT '{}'::jsonb   -- underlying, provider, path, status, duration_ms, count, ...
);
CREATE INDEX run_events_run_idx ON run_events (run_id, id);
CREATE INDEX run_events_ts_idx ON run_events (ts);

-- ---------- справочник сущностей (последние известные факты): catalog_* ----------
CREATE TABLE catalog_underlyings (
    id            TEXT PRIMARY KEY,                         -- ISIN или stock:ticker:XXX
    ticker        TEXT NOT NULL,                            -- unique among enabled rows only (see underlyings_ticker_enabled_idx)
    name          TEXT NOT NULL,
    isin          TEXT,
    asset_type    TEXT NOT NULL CHECK (asset_type IN ('stock', 'etf')),
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,            -- из config/assets.json, синхронизируется на старте run
    cmc_rwa_id    INT,
    cmc_name      TEXT,
    website       TEXT,                                     -- rwa_info.website (CMC)
    cmc_logo_url  TEXT,                                     -- rwa_info.logo (источник для catalog_logos)
    industry      TEXT,
    status        TEXT,                                     -- статус последнего снапшота
    last_run_id   BIGINT REFERENCES runs (id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A ticker may move between ids (ISIN added or corrected, tickers swapped); only enabled rows must be unique.
CREATE UNIQUE INDEX catalog_underlyings_ticker_enabled_idx ON catalog_underlyings (ticker) WHERE enabled;

CREATE TABLE catalog_issuers (
    id              TEXT PRIMARY KEY,                       -- CMC issuer_id
    name            TEXT,
    cmc_label       TEXT,                                   -- issuer_name из quotes
    website         TEXT,
    cmc_logo_url    TEXT,
    roster_size     INT,
    roster          JSONB NOT NULL DEFAULT '[]'::jsonb,     -- [{crypto_id, symbol, rwa_id}]
    metadata_status TEXT NOT NULL DEFAULT 'unknown' CHECK (metadata_status IN ('cmc', 'unknown')),
    fetched_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE catalog_tokens (
    id                 INT PRIMARY KEY,                     -- CMC crypto_id
    underlying_id      TEXT NOT NULL REFERENCES catalog_underlyings (id),
    issuer_id          TEXT REFERENCES catalog_issuers (id),
    symbol             TEXT,
    name               TEXT,
    cmc_slug           TEXT,
    kind               TEXT NOT NULL CHECK (kind IN ('tokenized', 'wrapper', 'derivative', 'unclassified')),
    excluded_reason    TEXT,
    cmc_logo_url       TEXT,
    date_added         TIMESTAMPTZ,
    coverage           TEXT NOT NULL,
    first_seen_run_id  BIGINT NOT NULL REFERENCES runs (id),
    last_seen_run_id   BIGINT NOT NULL REFERENCES runs (id),
    active             BOOLEAN NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX catalog_tokens_underlying_idx ON catalog_tokens (underlying_id);
CREATE INDEX catalog_tokens_issuer_idx ON catalog_tokens (issuer_id);

CREATE TABLE catalog_deployments (
    token_id           INT NOT NULL REFERENCES catalog_tokens (id) ON DELETE CASCADE,
    cmc_network        TEXT NOT NULL,
    address            TEXT NOT NULL,                       -- нормализованный (EVM в нижнем регистре)
    address_raw        TEXT NOT NULL,
    network            TEXT,                                -- slug Portals или NULL
    position_key       TEXT,                                -- network:address или NULL
    status             TEXT NOT NULL,                       -- indexed | not_indexed | unsupported_network | unmapped_network | not_checked | data_unavailable
    portals_symbol     TEXT,
    decimals           INT,
    portals_platform   TEXT,
    first_seen_run_id  BIGINT NOT NULL REFERENCES runs (id),
    last_seen_run_id   BIGINT NOT NULL REFERENCES runs (id),
    active             BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (token_id, cmc_network, address)
);
CREATE INDEX catalog_deployments_position_key_idx ON catalog_deployments (position_key) WHERE position_key IS NOT NULL;

CREATE TABLE catalog_platforms (                            -- Portals /v2/platforms
    slug        TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE catalog_networks (                             -- Portals /v1/networks + chain_mapping
    slug              TEXT PRIMARY KEY,
    cmc_names         TEXT[] NOT NULL DEFAULT '{}',
    last_seen_run_id  BIGINT REFERENCES runs (id),
    active            BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE catalog_positions (                            -- позиции Portals, попавшие в экспорт (confirmed + unconfirmed)
    key                  TEXT PRIMARY KEY,                  -- network:address, нормализованный
    network              TEXT NOT NULL,
    platform             TEXT NOT NULL,
    symbol               TEXT,
    name                 TEXT,
    address              TEXT NOT NULL,
    position_kind        TEXT NOT NULL CHECK (position_kind IN ('pool_id', 'contract')),
    receipt_token_key    TEXT,
    category             TEXT NOT NULL,
    tags                 TEXT[] NOT NULL DEFAULT '{}',
    components           JSONB NOT NULL DEFAULT '[]'::jsonb, -- [{key, symbol}]
    liquidity_semantics  TEXT NOT NULL,
    is_portable          BOOLEAN,
    portals_created_at   TIMESTAMPTZ,
    -- последние метрики (денормализация для списков; история — в history_positions)
    liquidity_usd        NUMERIC,
    apy_percent          NUMERIC,
    base_apy_percent     NUMERIC,
    reward_apy_percent   NUMERIC,
    borrow_apy_percent   NUMERIC,
    volume_usd_1d        NUMERIC,
    volume_usd_7d        NUMERIC,
    source_updated_at    TIMESTAMPTZ,
    flags                TEXT[] NOT NULL DEFAULT '{}',
    eligibility          TEXT NOT NULL,
    first_seen_run_id    BIGINT NOT NULL REFERENCES runs (id),
    last_seen_run_id     BIGINT NOT NULL REFERENCES runs (id),
    active               BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()   -- row modification time; export updated_at = source_updated_at
);
CREATE INDEX catalog_positions_network_idx  ON catalog_positions (network);
CREATE INDEX catalog_positions_platform_idx ON catalog_positions (platform);
CREATE INDEX catalog_positions_category_idx ON catalog_positions (category);

CREATE TABLE catalog_token_positions (                      -- relations 1.x (direct/indirect) + unconfirmed_keys
    token_id           INT NOT NULL REFERENCES catalog_tokens (id) ON DELETE CASCADE,
    position_key       TEXT NOT NULL REFERENCES catalog_positions (key) ON DELETE CASCADE,
    link               TEXT NOT NULL CHECK (link IN ('direct', 'indirect', 'unconfirmed')),
    depth              INT,                                 -- 1 direct, >1 indirect, NULL unconfirmed
    path               TEXT[],                              -- root → LP → vault
    first_seen_run_id  BIGINT NOT NULL REFERENCES runs (id),
    last_seen_run_id   BIGINT NOT NULL REFERENCES runs (id),
    active             BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (token_id, position_key)
);
CREATE INDEX catalog_token_positions_position_idx ON catalog_token_positions (position_key);

CREATE TABLE catalog_logos (
    kind          TEXT NOT NULL CHECK (kind IN ('tokens', 'issuers', 'underlyings')),
    entity_id     TEXT NOT NULL,                            -- tokens: crypto_id; issuers: issuer_id; underlyings: storage_id(ISIN)
    ext           TEXT NOT NULL,                            -- png | jpg | svg | webp | ico | gif
    content_type  TEXT NOT NULL,
    bytes         BYTEA NOT NULL,
    sha256        TEXT NOT NULL,
    source        TEXT NOT NULL,                            -- URL источника или 'manual'
    fetched_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, entity_id)
);

-- ---------- сырые ответы источников: raw_* ----------
CREATE TABLE raw_snapshots (
    id             BIGSERIAL PRIMARY KEY,
    run_id         BIGINT NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    underlying_id  TEXT NOT NULL REFERENCES catalog_underlyings (id),
    fetched_at     TIMESTAMPTZ NOT NULL,
    status         TEXT NOT NULL,
    schema_version INT NOT NULL DEFAULT 1,
    payload        JSONB NOT NULL,                          -- документ снапшота 1.x без изменений
    payload_bytes  INT NOT NULL,
    UNIQUE (run_id, underlying_id)
);
ALTER TABLE raw_snapshots ALTER COLUMN payload SET COMPRESSION lz4;
CREATE INDEX raw_snapshots_underlying_fetched_idx ON raw_snapshots (underlying_id, fetched_at DESC);

-- cp0x deals cache (the 1.x data/cp0x_deals.json): one row per successful fetch, payload = {fetched_at, deals}.
-- Publish reads the newest row; a failed fetch leaves the previous row in place.
CREATE TABLE raw_cp0x_deals (
    id         BIGSERIAL PRIMARY KEY,
    fetched_at TIMESTAMPTZ NOT NULL,
    payload    JSONB NOT NULL
);
CREATE INDEX raw_cp0x_deals_fetched_idx ON raw_cp0x_deals (fetched_at DESC, id DESC);

-- ---------- история для графиков (append-only): history_* ----------
CREATE TABLE history_summary (
    run_id                              BIGINT PRIMARY KEY REFERENCES runs (id) ON DELETE CASCADE,
    observed_at                         TIMESTAMPTZ NOT NULL,   -- newest_fetched_at
    underlyings_count                   INT NOT NULL,
    tokens_count                        INT NOT NULL,
    excluded_tokens_count               INT NOT NULL,
    issuers_count                       INT NOT NULL,
    deployments_count                   INT NOT NULL,
    indexed_deployments_count           INT NOT NULL,
    tokens_in_defi_count                INT NOT NULL,
    raw_confirmed_opportunities_count   INT NOT NULL,
    eligible_opportunities_count        INT NOT NULL,
    unknown_liquidity_count             INT NOT NULL,
    direct_eligible_count               INT NOT NULL,
    indirect_eligible_count             INT NOT NULL,
    eligible_protocols_count            INT NOT NULL,
    eligible_networks_count             INT NOT NULL,
    yield_opportunities_count           INT NOT NULL,
    max_reported_apy_percent            NUMERIC,
    average_reported_apy_percent        NUMERIC,
    median_reported_apy_percent         NUMERIC,
    tokenized_market_cap_usd            NUMERIC,            -- новое в 2.0: сумма по корзине
    tokenized_volume_24h_usd            NUMERIC,            -- новое в 2.0
    eligible_liquidity_usd              NUMERIC             -- новое в 2.0: сумма liquidity eligible-позиций
);
CREATE INDEX history_summary_observed_idx ON history_summary (observed_at);

CREATE TABLE history_underlyings (
    run_id                              BIGINT NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    underlying_id                       TEXT NOT NULL REFERENCES catalog_underlyings (id),
    observed_at                         TIMESTAMPTZ NOT NULL,
    status                              TEXT NOT NULL,
    tokenized_market_cap_usd            NUMERIC,
    tokenized_volume_24h_usd            NUMERIC,
    average_tokenized_price_usd         NUMERIC,
    versions_count                      INT NOT NULL,
    issuers_count                       INT NOT NULL,
    tokens_in_defi_count                INT NOT NULL,
    raw_confirmed_opportunities_count   INT NOT NULL,
    eligible_opportunities_count        INT NOT NULL,
    unknown_liquidity_count             INT NOT NULL,
    direct_eligible_count               INT NOT NULL,
    indirect_eligible_count             INT NOT NULL,
    eligible_protocols_count            INT NOT NULL,
    eligible_networks_count             INT NOT NULL,
    yield_opportunities_count           INT NOT NULL,
    max_reported_apy_percent            NUMERIC,
    average_reported_apy_percent        NUMERIC,
    median_reported_apy_percent         NUMERIC,
    eligible_liquidity_usd              NUMERIC,
    PRIMARY KEY (underlying_id, run_id)
);
CREATE INDEX history_underlyings_observed_idx ON history_underlyings (underlying_id, observed_at);

CREATE TABLE history_tokens (
    run_id                              BIGINT NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    token_id                            INT NOT NULL REFERENCES catalog_tokens (id),
    observed_at                         TIMESTAMPTZ NOT NULL,
    price_usd                           NUMERIC,
    market_cap_usd                      NUMERIC,
    volume_24h_usd                      NUMERIC,
    usefulness_score                    INT NOT NULL,
    rank_in_underlying                  INT,
    raw_confirmed_opportunities_count   INT NOT NULL,
    eligible_opportunities_count        INT NOT NULL,
    unknown_liquidity_count             INT NOT NULL,
    direct_eligible_count               INT NOT NULL,
    indirect_eligible_count             INT NOT NULL,
    eligible_protocols_count            INT NOT NULL,
    eligible_networks_count             INT NOT NULL,
    unconfirmed_candidates_count        INT NOT NULL,
    yield_opportunities_count           INT NOT NULL,
    max_reported_apy_percent            NUMERIC,
    average_reported_apy_percent        NUMERIC,
    median_reported_apy_percent         NUMERIC,
    deployments_count                   INT NOT NULL,
    indexed_deployments_count           INT NOT NULL,
    eligible_liquidity_usd              NUMERIC,
    breakdowns                          JSONB NOT NULL,     -- {by_network, by_category, by_platform}
    PRIMARY KEY (token_id, run_id)
);
CREATE INDEX history_tokens_observed_idx ON history_tokens (token_id, observed_at);

CREATE TABLE history_issuers (
    run_id                              BIGINT NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    issuer_id                           TEXT NOT NULL REFERENCES catalog_issuers (id),
    observed_at                         TIMESTAMPTZ NOT NULL,
    issuer_usefulness_score             INT NOT NULL,
    basket_relative_score               NUMERIC,
    compared_underlyings_count          INT NOT NULL,
    basket_underlyings_count            INT NOT NULL,
    basket_tokens_count                 INT NOT NULL,
    tokens_in_defi_count                INT NOT NULL,
    deployments_count                   INT NOT NULL,
    supported_deployments_count         INT NOT NULL,
    unsupported_deployments_count       INT NOT NULL,
    raw_confirmed_opportunities_count   INT NOT NULL,
    eligible_opportunities_count        INT NOT NULL,
    unknown_liquidity_count             INT NOT NULL,
    direct_eligible_count               INT NOT NULL,
    indirect_eligible_count             INT NOT NULL,
    eligible_protocols_count            INT NOT NULL,
    eligible_networks_count             INT NOT NULL,
    yield_opportunities_count           INT NOT NULL,
    max_reported_apy_percent            NUMERIC,
    average_reported_apy_percent        NUMERIC,
    median_reported_apy_percent         NUMERIC,
    direct_pools_liquidity_usd          NUMERIC,
    basket_market_cap_usd               NUMERIC,
    roster_size                         INT,
    breakdowns                          JSONB NOT NULL,     -- {by_network, by_category, by_underlying}
    PRIMARY KEY (issuer_id, run_id)
);
CREATE INDEX history_issuers_observed_idx ON history_issuers (issuer_id, observed_at);

CREATE TABLE history_positions (
    run_id              BIGINT NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    position_key        TEXT NOT NULL REFERENCES catalog_positions (key),
    observed_at         TIMESTAMPTZ NOT NULL,
    liquidity_usd       NUMERIC,
    apy_percent         NUMERIC,
    base_apy_percent    NUMERIC,
    reward_apy_percent  NUMERIC,
    borrow_apy_percent  NUMERIC,
    volume_usd_1d       NUMERIC,
    volume_usd_7d       NUMERIC,
    source_updated_at   TIMESTAMPTZ,
    eligibility         TEXT NOT NULL,
    flags               TEXT[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (position_key, run_id)
);
CREATE INDEX history_positions_observed_idx ON history_positions (position_key, observed_at);

-- ---------- результат расчёта, его читают API и сайт: build_* ----------
CREATE TABLE build_exports (
    id              BIGSERIAL PRIMARY KEY,
    run_id          BIGINT NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    generated_at    TIMESTAMPTZ NOT NULL,
    schema_version  INT NOT NULL,
    score_version   TEXT NOT NULL,
    settings        JSONB NOT NULL,                         -- подмножество настроек, как в export.settings
    summary         JSONB NOT NULL,
    -- BYTEA, not JSONB: JSONB reorders object keys (shorter first, then alphabetical), but the 1.x export keeps
    -- breakdown keys sorted by (-eligible, -raw, key) and the API serves these exact bytes (export_sha256 = sha256 of them).
    export          BYTEA NOT NULL,                         -- полный документ формата rwa.json 1.x, байты как отдаёт API
    export_sha256   TEXT NOT NULL,                          -- ETag
    is_current      BOOLEAN NOT NULL DEFAULT FALSE
);
ALTER TABLE build_exports ALTER COLUMN export SET COMPRESSION lz4;
CREATE UNIQUE INDEX build_exports_current_idx ON build_exports (is_current) WHERE is_current;
CREATE INDEX build_exports_run_idx ON build_exports (run_id);

CREATE TABLE build_changes (                                -- что изменилось относительно предыдущего build (страница «Обновления»)
    build_id            BIGINT PRIMARY KEY REFERENCES build_exports (id) ON DELETE CASCADE,
    previous_build_id   BIGINT REFERENCES build_exports (id),
    new_tokens          INT[] NOT NULL DEFAULT '{}',
    removed_tokens      INT[] NOT NULL DEFAULT '{}',
    new_issuers         TEXT[] NOT NULL DEFAULT '{}',
    new_positions_count INT NOT NULL DEFAULT 0,
    gone_positions_count INT NOT NULL DEFAULT 0,
    eligible_delta      INT NOT NULL DEFAULT 0,
    details             JSONB NOT NULL DEFAULT '{}'::jsonb  -- произвольные уточнения (по сетям, протоколам)
);

-- +goose Down
DROP TABLE IF EXISTS raw_cp0x_deals, run_events, build_changes, build_exports, history_positions, history_issuers, history_tokens, history_underlyings,
    history_summary, raw_snapshots, catalog_logos, catalog_token_positions, catalog_positions, catalog_networks, catalog_platforms, catalog_deployments, catalog_tokens, catalog_issuers,
    catalog_underlyings, run_requests, run_underlyings, runs;
