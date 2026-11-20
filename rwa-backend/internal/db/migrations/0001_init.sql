-- +goose Up

-- ---------- запуски ----------
CREATE TABLE collection_runs (
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
CREATE INDEX collection_runs_started_at_idx ON collection_runs (started_at DESC);

CREATE TABLE run_underlyings (
    run_id            BIGINT NOT NULL REFERENCES collection_runs (id) ON DELETE CASCADE,
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
    run_id        BIGINT REFERENCES collection_runs (id)
);
CREATE INDEX run_requests_pending_idx ON run_requests (requested_at) WHERE taken_at IS NULL;

-- ---------- текущее состояние ----------
CREATE TABLE underlyings (
    id            TEXT PRIMARY KEY,                         -- ISIN или stock:ticker:XXX
    ticker        TEXT NOT NULL,                            -- unique among enabled rows only (see underlyings_ticker_enabled_idx)
    name          TEXT NOT NULL,
    isin          TEXT,
    asset_type    TEXT NOT NULL CHECK (asset_type IN ('stock', 'etf')),
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,            -- из config/assets.json, синхронизируется на старте run
    cmc_rwa_id    INT,
    cmc_name      TEXT,
    website       TEXT,                                     -- rwa_info.website (CMC)
    cmc_logo_url  TEXT,                                     -- rwa_info.logo (источник для logos)
    industry      TEXT,
    status        TEXT,                                     -- статус последнего снапшота
    last_run_id   BIGINT REFERENCES collection_runs (id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A ticker may move between ids (ISIN added or corrected, tickers swapped); only enabled rows must be unique.
CREATE UNIQUE INDEX underlyings_ticker_enabled_idx ON underlyings (ticker) WHERE enabled;

CREATE TABLE issuers (
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

CREATE TABLE tokens (
    id                 INT PRIMARY KEY,                     -- CMC crypto_id
    underlying_id      TEXT NOT NULL REFERENCES underlyings (id),
    issuer_id          TEXT REFERENCES issuers (id),
    symbol             TEXT,
    name               TEXT,
    cmc_slug           TEXT,
    kind               TEXT NOT NULL CHECK (kind IN ('tokenized', 'wrapper', 'derivative', 'unclassified')),
    excluded_reason    TEXT,
    cmc_logo_url       TEXT,
    date_added         TIMESTAMPTZ,
    coverage           TEXT NOT NULL,
    first_seen_run_id  BIGINT NOT NULL REFERENCES collection_runs (id),
    last_seen_run_id   BIGINT NOT NULL REFERENCES collection_runs (id),
    active             BOOLEAN NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tokens_underlying_idx ON tokens (underlying_id);
CREATE INDEX tokens_issuer_idx ON tokens (issuer_id);

CREATE TABLE deployments (
    token_id           INT NOT NULL REFERENCES tokens (id) ON DELETE CASCADE,
    cmc_network        TEXT NOT NULL,
    address            TEXT NOT NULL,                       -- нормализованный (EVM в нижнем регистре)
    address_raw        TEXT NOT NULL,
    network            TEXT,                                -- slug Portals или NULL
    position_key       TEXT,                                -- network:address или NULL
    status             TEXT NOT NULL,                       -- indexed | not_indexed | unsupported_network | unmapped_network | not_checked | data_unavailable
    portals_symbol     TEXT,
    decimals           INT,
    portals_platform   TEXT,
    first_seen_run_id  BIGINT NOT NULL REFERENCES collection_runs (id),
    last_seen_run_id   BIGINT NOT NULL REFERENCES collection_runs (id),
    active             BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (token_id, cmc_network, address)
);
CREATE INDEX deployments_position_key_idx ON deployments (position_key) WHERE position_key IS NOT NULL;

CREATE TABLE platforms (                                    -- Portals /v2/platforms
    slug        TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE networks (                                     -- Portals /v1/networks + chain_mapping
    slug              TEXT PRIMARY KEY,
    cmc_names         TEXT[] NOT NULL DEFAULT '{}',
    last_seen_run_id  BIGINT REFERENCES collection_runs (id),
    active            BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE positions (                                    -- позиции Portals, попавшие в экспорт (confirmed + unconfirmed)
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
    -- последние метрики (денормализация для списков; история — в position_history)
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
    first_seen_run_id    BIGINT NOT NULL REFERENCES collection_runs (id),
    last_seen_run_id     BIGINT NOT NULL REFERENCES collection_runs (id),
    active               BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()   -- row modification time; export updated_at = source_updated_at
);
CREATE INDEX positions_network_idx  ON positions (network);
CREATE INDEX positions_platform_idx ON positions (platform);
CREATE INDEX positions_category_idx ON positions (category);

CREATE TABLE token_positions (                              -- relations 1.x (direct/indirect) + unconfirmed_keys
    token_id           INT NOT NULL REFERENCES tokens (id) ON DELETE CASCADE,
    position_key       TEXT NOT NULL REFERENCES positions (key) ON DELETE CASCADE,
    link               TEXT NOT NULL CHECK (link IN ('direct', 'indirect', 'unconfirmed')),
    depth              INT,                                 -- 1 direct, >1 indirect, NULL unconfirmed
    path               TEXT[],                              -- root → LP → vault
    first_seen_run_id  BIGINT NOT NULL REFERENCES collection_runs (id),
    last_seen_run_id   BIGINT NOT NULL REFERENCES collection_runs (id),
    active             BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (token_id, position_key)
);
CREATE INDEX token_positions_position_idx ON token_positions (position_key);

CREATE TABLE logos (
    kind          TEXT NOT NULL CHECK (kind IN ('tokens', 'issuers', 'underlyings')),
    entity_id     TEXT NOT NULL,                            -- tokens: crypto_id; issuers: issuer_id; underlyings: storage_id(ISIN)
    ext           TEXT NOT NULL,                            -- png | jpg | svg | webp | ico | gif
    content_type  TEXT NOT NULL,
    bytes         BYTEA NOT NULL,
    sha256        TEXT NOT NULL,
    source        TEXT NOT NULL,                            -- URL источника или 'manual' / 'import-v1'
    fetched_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, entity_id)
);

-- ---------- сырые факты ----------
CREATE TABLE snapshots (
    id             BIGSERIAL PRIMARY KEY,
    run_id         BIGINT NOT NULL REFERENCES collection_runs (id) ON DELETE CASCADE,
    underlying_id  TEXT NOT NULL REFERENCES underlyings (id),
    fetched_at     TIMESTAMPTZ NOT NULL,
    status         TEXT NOT NULL,
    schema_version INT NOT NULL DEFAULT 1,
    payload        JSONB NOT NULL,                          -- документ снапшота 1.x без изменений
    payload_bytes  INT NOT NULL,
    UNIQUE (run_id, underlying_id)
);
ALTER TABLE snapshots ALTER COLUMN payload SET COMPRESSION lz4;
CREATE INDEX snapshots_underlying_fetched_idx ON snapshots (underlying_id, fetched_at DESC);

-- ---------- история (append-only) ----------
CREATE TABLE summary_history (
    run_id                              BIGINT PRIMARY KEY REFERENCES collection_runs (id) ON DELETE CASCADE,
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
CREATE INDEX summary_history_observed_idx ON summary_history (observed_at);

CREATE TABLE underlying_history (
    run_id                              BIGINT NOT NULL REFERENCES collection_runs (id) ON DELETE CASCADE,
    underlying_id                       TEXT NOT NULL REFERENCES underlyings (id),
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
CREATE INDEX underlying_history_observed_idx ON underlying_history (underlying_id, observed_at);

CREATE TABLE token_history (
    run_id                              BIGINT NOT NULL REFERENCES collection_runs (id) ON DELETE CASCADE,
    token_id                            INT NOT NULL REFERENCES tokens (id),
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
CREATE INDEX token_history_observed_idx ON token_history (token_id, observed_at);

CREATE TABLE issuer_history (
    run_id                              BIGINT NOT NULL REFERENCES collection_runs (id) ON DELETE CASCADE,
    issuer_id                           TEXT NOT NULL REFERENCES issuers (id),
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
CREATE INDEX issuer_history_observed_idx ON issuer_history (issuer_id, observed_at);

CREATE TABLE position_history (
    run_id              BIGINT NOT NULL REFERENCES collection_runs (id) ON DELETE CASCADE,
    position_key        TEXT NOT NULL REFERENCES positions (key),
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
CREATE INDEX position_history_observed_idx ON position_history (position_key, observed_at);

-- ---------- результат расчёта ----------
CREATE TABLE builds (
    id              BIGSERIAL PRIMARY KEY,
    run_id          BIGINT NOT NULL REFERENCES collection_runs (id) ON DELETE CASCADE,
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
ALTER TABLE builds ALTER COLUMN export SET COMPRESSION lz4;
CREATE UNIQUE INDEX builds_current_idx ON builds (is_current) WHERE is_current;
CREATE INDEX builds_run_idx ON builds (run_id);

CREATE TABLE build_changes (                                -- что изменилось относительно предыдущего build (страница «Обновления»)
    build_id            BIGINT PRIMARY KEY REFERENCES builds (id) ON DELETE CASCADE,
    previous_build_id   BIGINT REFERENCES builds (id),
    new_tokens          INT[] NOT NULL DEFAULT '{}',
    removed_tokens      INT[] NOT NULL DEFAULT '{}',
    new_issuers         TEXT[] NOT NULL DEFAULT '{}',
    new_positions_count INT NOT NULL DEFAULT 0,
    gone_positions_count INT NOT NULL DEFAULT 0,
    eligible_delta      INT NOT NULL DEFAULT 0,
    details             JSONB NOT NULL DEFAULT '{}'::jsonb  -- произвольные уточнения (по сетям, протоколам)
);

-- +goose Down
DROP TABLE IF EXISTS build_changes, builds, position_history, issuer_history, token_history, underlying_history,
    summary_history, snapshots, logos, token_positions, positions, networks, platforms, deployments, tokens, issuers,
    underlyings, run_requests, run_underlyings, collection_runs;
