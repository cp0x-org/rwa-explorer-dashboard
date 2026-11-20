package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// SyncUnderlyings makes the underlyings table reflect assets.json: ticker, name, asset_type, isin and enabled
// are upserted by id, and rows no longer listed end up disabled. Everything runs in one implicit transaction
// (a single batch). All rows are disabled first, and the upserts re-enable the listed ones: ticker is unique
// among enabled rows only, so an ISIN added to a ticker or two tickers swapped between ISINs cannot collide
// with a stale row. It returns the number of listed assets.
func (s *Store) SyncUnderlyings(ctx context.Context, assets []model.Asset) (int, error) {
	b := &pgx.Batch{}
	b.Queue(`UPDATE underlyings SET enabled = FALSE WHERE enabled`)
	for _, a := range assets {
		b.Queue(`INSERT INTO underlyings (id, ticker, name, isin, asset_type, enabled) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (id) DO UPDATE SET ticker = EXCLUDED.ticker, name = EXCLUDED.name, isin = EXCLUDED.isin,
				asset_type = EXCLUDED.asset_type, enabled = EXCLUDED.enabled, updated_at = now()`,
			a.ID(), a.Ticker, a.Name, a.ISIN, a.AssetType, a.Enabled)
	}
	if err := s.execBatch(ctx, b, "sync underlyings"); err != nil {
		return 0, err
	}
	return len(assets), nil
}

// UnderlyingFactsRow carries the facts learned from a run for an existing underlying.
type UnderlyingFactsRow struct {
	ID         string
	CMCRwaID   *int
	CMCName    *string
	Website    *string
	CMCLogoURL *string
	Industry   *string
	Status     *string
	LastRunID  int64
}

// UpsertUnderlyingFacts updates CMC facts, status and last_run_id of underlyings created by SyncUnderlyings.
func (s *Store) UpsertUnderlyingFacts(ctx context.Context, rows []UnderlyingFactsRow) error {
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(`UPDATE underlyings SET cmc_rwa_id = $2, cmc_name = $3, website = $4, cmc_logo_url = $5, industry = $6,
			status = $7, last_run_id = $8, updated_at = now() WHERE id = $1`,
			r.ID, r.CMCRwaID, r.CMCName, r.Website, r.CMCLogoURL, r.Industry, r.Status, r.LastRunID)
	}
	br := s.q.SendBatch(ctx, b)
	defer func() { _ = br.Close() }()
	for _, r := range rows {
		tag, err := br.Exec()
		if err != nil {
			return fmt.Errorf("store: upsert underlying facts %s: %w", r.ID, err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("store: upsert underlying facts %s: %w", r.ID, ErrNotFound)
		}
	}
	return nil
}

// IssuerRow is an issuers row (current state; issuers have no active flag).
type IssuerRow struct {
	ID             string
	Name           *string
	CMCLabel       *string
	Website        *string
	CMCLogoURL     *string
	RosterSize     *int
	Roster         []byte // JSON [{crypto_id, symbol, rwa_id}]
	MetadataStatus string // cmc | unknown
	FetchedAt      *time.Time
}

// UpsertIssuers inserts or updates issuers.
func (s *Store) UpsertIssuers(ctx context.Context, rows []IssuerRow) error {
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(`INSERT INTO issuers (id, name, cmc_label, website, cmc_logo_url, roster_size, roster, metadata_status, fetched_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, cmc_label = EXCLUDED.cmc_label, website = EXCLUDED.website,
				cmc_logo_url = EXCLUDED.cmc_logo_url, roster_size = EXCLUDED.roster_size, roster = EXCLUDED.roster,
				metadata_status = EXCLUDED.metadata_status, fetched_at = EXCLUDED.fetched_at, updated_at = now()`,
			r.ID, r.Name, r.CMCLabel, r.Website, r.CMCLogoURL, r.RosterSize, jsonOr(r.Roster, "[]"), r.MetadataStatus, r.FetchedAt)
	}
	return s.execBatch(ctx, b, "upsert issuers")
}

// TokenRow is a tokens row (current state).
type TokenRow struct {
	ID             int64 // CMC crypto_id
	UnderlyingID   string
	IssuerID       *string
	Symbol         *string
	Name           *string
	CMCSlug        *string
	Kind           string // tokenized | wrapper | derivative | unclassified
	ExcludedReason *string
	CMCLogoURL     *string
	DateAdded      *time.Time
	Coverage       string
}

// UpsertTokens inserts or updates tokens, marking them seen in runID and active.
func (s *Store) UpsertTokens(ctx context.Context, runID int64, rows []TokenRow) error {
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(`INSERT INTO tokens (id, underlying_id, issuer_id, symbol, name, cmc_slug, kind, excluded_reason, cmc_logo_url,
				date_added, coverage, first_seen_run_id, last_seen_run_id, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12, TRUE)
			ON CONFLICT (id) DO UPDATE SET underlying_id = EXCLUDED.underlying_id, issuer_id = EXCLUDED.issuer_id,
				symbol = EXCLUDED.symbol, name = EXCLUDED.name, cmc_slug = EXCLUDED.cmc_slug, kind = EXCLUDED.kind,
				excluded_reason = EXCLUDED.excluded_reason, cmc_logo_url = EXCLUDED.cmc_logo_url, date_added = EXCLUDED.date_added,
				coverage = EXCLUDED.coverage, last_seen_run_id = EXCLUDED.last_seen_run_id, active = TRUE, updated_at = now()`,
			r.ID, r.UnderlyingID, r.IssuerID, r.Symbol, r.Name, r.CMCSlug, r.Kind, r.ExcludedReason, r.CMCLogoURL,
			r.DateAdded, r.Coverage, runID)
	}
	return s.execBatch(ctx, b, "upsert tokens")
}

// DeactivateMissingTokens marks tokens not seen in runID inactive and returns how many changed.
func (s *Store) DeactivateMissingTokens(ctx context.Context, runID int64) (int64, error) {
	return s.deactivate(ctx, "tokens", runID)
}

// DeploymentRow is a deployments row.
type DeploymentRow struct {
	TokenID         int64
	CMCNetwork      string
	Address         string // normalized (EVM lower-case)
	AddressRaw      string
	Network         *string
	PositionKey     *string
	Status          string
	PortalsSymbol   *string
	Decimals        *int
	PortalsPlatform *string
}

// UpsertDeployments inserts or updates deployments, marking them seen in runID and active.
func (s *Store) UpsertDeployments(ctx context.Context, runID int64, rows []DeploymentRow) error {
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(`INSERT INTO deployments (token_id, cmc_network, address, address_raw, network, position_key, status,
				portals_symbol, decimals, portals_platform, first_seen_run_id, last_seen_run_id, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11, TRUE)
			ON CONFLICT (token_id, cmc_network, address) DO UPDATE SET address_raw = EXCLUDED.address_raw,
				network = EXCLUDED.network, position_key = EXCLUDED.position_key, status = EXCLUDED.status,
				portals_symbol = EXCLUDED.portals_symbol, decimals = EXCLUDED.decimals,
				portals_platform = EXCLUDED.portals_platform, last_seen_run_id = EXCLUDED.last_seen_run_id, active = TRUE`,
			r.TokenID, r.CMCNetwork, r.Address, r.AddressRaw, r.Network, r.PositionKey, r.Status,
			r.PortalsSymbol, r.Decimals, r.PortalsPlatform, runID)
	}
	return s.execBatch(ctx, b, "upsert deployments")
}

// DeactivateMissingDeployments marks deployments not seen in runID inactive and returns how many changed.
func (s *Store) DeactivateMissingDeployments(ctx context.Context, runID int64) (int64, error) {
	return s.deactivate(ctx, "deployments", runID)
}

// PlatformRow is a platforms row (Portals /v2/platforms).
type PlatformRow struct {
	Slug string
	Name string
}

// UpsertPlatforms inserts or updates Portals platforms. The table has no run tracking.
func (s *Store) UpsertPlatforms(ctx context.Context, rows []PlatformRow) error {
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(`INSERT INTO platforms (slug, name) VALUES ($1, $2)
			ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, updated_at = now()`, r.Slug, r.Name)
	}
	return s.execBatch(ctx, b, "upsert platforms")
}

// NetworkRow is a networks row.
type NetworkRow struct {
	Slug     string
	CMCNames []string
}

// UpsertNetworks inserts or updates Portals networks, marking them seen in runID and active.
func (s *Store) UpsertNetworks(ctx context.Context, runID int64, rows []NetworkRow) error {
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(`INSERT INTO networks (slug, cmc_names, last_seen_run_id, active) VALUES ($1, $2, $3, TRUE)
			ON CONFLICT (slug) DO UPDATE SET cmc_names = EXCLUDED.cmc_names, last_seen_run_id = EXCLUDED.last_seen_run_id, active = TRUE`,
			r.Slug, strs(r.CMCNames), runID)
	}
	return s.execBatch(ctx, b, "upsert networks")
}

// DeactivateMissingNetworks marks networks not seen in runID inactive and returns how many changed.
func (s *Store) DeactivateMissingNetworks(ctx context.Context, runID int64) (int64, error) {
	return s.deactivate(ctx, "networks", runID)
}

// PositionRow is a positions row: a Portals position that made it into the export, with its latest metrics.
type PositionRow struct {
	Key                string
	Network            string
	Platform           string
	Symbol             *string
	Name               *string
	Address            string
	PositionKind       string // pool_id | contract
	ReceiptTokenKey    *string
	Category           string
	Tags               []string
	Components         []byte // JSON [{key, symbol}]
	LiquiditySemantics string
	IsPortable         *bool
	PortalsCreatedAt   *time.Time
	LiquidityUSD       model.Decimal
	APYPercent         model.Decimal
	BaseAPYPercent     model.Decimal
	RewardAPYPercent   model.Decimal
	BorrowAPYPercent   model.Decimal
	VolumeUSD1d        model.Decimal
	VolumeUSD7d        model.Decimal
	SourceUpdatedAt    *time.Time
	Flags              []string
	Eligibility        string
}

// UpsertPositions inserts or updates positions, marking them seen in runID and active.
func (s *Store) UpsertPositions(ctx context.Context, runID int64, rows []PositionRow) error {
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(`INSERT INTO positions (key, network, platform, symbol, name, address, position_kind, receipt_token_key, category,
				tags, components, liquidity_semantics, is_portable, portals_created_at, liquidity_usd, apy_percent,
				base_apy_percent, reward_apy_percent, borrow_apy_percent, volume_usd_1d, volume_usd_7d, source_updated_at,
				flags, eligibility, first_seen_run_id, last_seen_run_id, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22,
				$23, $24, $25, $25, TRUE)
			ON CONFLICT (key) DO UPDATE SET network = EXCLUDED.network, platform = EXCLUDED.platform, symbol = EXCLUDED.symbol,
				name = EXCLUDED.name, address = EXCLUDED.address, position_kind = EXCLUDED.position_kind,
				receipt_token_key = EXCLUDED.receipt_token_key, category = EXCLUDED.category, tags = EXCLUDED.tags,
				components = EXCLUDED.components, liquidity_semantics = EXCLUDED.liquidity_semantics,
				is_portable = EXCLUDED.is_portable, portals_created_at = EXCLUDED.portals_created_at,
				liquidity_usd = EXCLUDED.liquidity_usd, apy_percent = EXCLUDED.apy_percent,
				base_apy_percent = EXCLUDED.base_apy_percent, reward_apy_percent = EXCLUDED.reward_apy_percent,
				borrow_apy_percent = EXCLUDED.borrow_apy_percent, volume_usd_1d = EXCLUDED.volume_usd_1d,
				volume_usd_7d = EXCLUDED.volume_usd_7d, source_updated_at = EXCLUDED.source_updated_at,
				flags = EXCLUDED.flags, eligibility = EXCLUDED.eligibility, last_seen_run_id = EXCLUDED.last_seen_run_id,
				active = TRUE, updated_at = now()`,
			r.Key, r.Network, r.Platform, r.Symbol, r.Name, r.Address, r.PositionKind, r.ReceiptTokenKey, r.Category,
			strs(r.Tags), jsonOr(r.Components, "[]"), r.LiquiditySemantics, r.IsPortable, r.PortalsCreatedAt,
			numeric(r.LiquidityUSD), numeric(r.APYPercent), numeric(r.BaseAPYPercent), numeric(r.RewardAPYPercent),
			numeric(r.BorrowAPYPercent), numeric(r.VolumeUSD1d), numeric(r.VolumeUSD7d), r.SourceUpdatedAt,
			strs(r.Flags), r.Eligibility, runID)
	}
	return s.execBatch(ctx, b, "upsert positions")
}

// DeactivateMissingPositions marks positions not seen in runID inactive and returns how many changed.
func (s *Store) DeactivateMissingPositions(ctx context.Context, runID int64) (int64, error) {
	return s.deactivate(ctx, "positions", runID)
}

// TokenPositionRow is a token_positions row (relations 1.x plus unconfirmed candidates).
type TokenPositionRow struct {
	TokenID     int64
	PositionKey string
	Link        string // direct | indirect | unconfirmed
	Depth       *int   // 1 direct, >1 indirect, nil unconfirmed
	Path        []string
}

// UpsertTokenPositions inserts or updates token-position links, marking them seen in runID and active.
// Tokens and positions must already exist.
func (s *Store) UpsertTokenPositions(ctx context.Context, runID int64, rows []TokenPositionRow) error {
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(`INSERT INTO token_positions (token_id, position_key, link, depth, path, first_seen_run_id, last_seen_run_id, active)
			VALUES ($1, $2, $3, $4, $5, $6, $6, TRUE)
			ON CONFLICT (token_id, position_key) DO UPDATE SET link = EXCLUDED.link, depth = EXCLUDED.depth, path = EXCLUDED.path,
				last_seen_run_id = EXCLUDED.last_seen_run_id, active = TRUE`,
			r.TokenID, r.PositionKey, r.Link, r.Depth, r.Path, runID)
	}
	return s.execBatch(ctx, b, "upsert token positions")
}

// DeactivateMissingTokenPositions marks links not seen in runID inactive and returns how many changed.
func (s *Store) DeactivateMissingTokenPositions(ctx context.Context, runID int64) (int64, error) {
	return s.deactivate(ctx, "token_positions", runID)
}

// deactivate flips active for rows of a current-state table that the run did not see. The table name is
// one of a fixed set of constants, never user input.
func (s *Store) deactivate(ctx context.Context, table string, runID int64) (int64, error) {
	tag, err := s.q.Exec(ctx, `UPDATE `+table+` SET active = FALSE WHERE active AND last_seen_run_id IS DISTINCT FROM $1`, runID)
	if err != nil {
		return 0, fmt.Errorf("store: deactivate missing %s: %w", table, err)
	}
	return tag.RowsAffected(), nil
}
