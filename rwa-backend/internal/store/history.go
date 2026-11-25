package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// HistoryCounts is the opportunity counters block shared by every history row.
type HistoryCounts struct {
	RawConfirmedOpportunitiesCount int
	EligibleOpportunitiesCount     int
	UnknownLiquidityCount          int
	DirectEligibleCount            int
	IndirectEligibleCount          int
	EligibleProtocolsCount         int
	EligibleNetworksCount          int
}

// HistoryYield is the yield block shared by every history row.
type HistoryYield struct {
	YieldOpportunitiesCount   int
	MaxReportedAPYPercent     model.Decimal
	AverageReportedAPYPercent model.Decimal
	MedianReportedAPYPercent  model.Decimal
}

// SummaryHistoryRow is a history_summary row.
type SummaryHistoryRow struct {
	RunID                   int64
	ObservedAt              time.Time
	UnderlyingsCount        int
	TokensCount             int
	ExcludedTokensCount     int
	IssuersCount            int
	DeploymentsCount        int
	IndexedDeploymentsCount int
	TokensInDefiCount       int
	HistoryCounts
	YieldOpportunitiesCount   int
	MaxReportedAPYPercent     model.Decimal
	AverageReportedAPYPercent model.Decimal
	MedianReportedAPYPercent  model.Decimal
	TokenizedMarketCapUSD     model.Decimal
	TokenizedVolume24hUSD     model.Decimal
	EligibleLiquidityUSD      model.Decimal
}

// UnderlyingHistoryRow is a history_underlyings row.
type UnderlyingHistoryRow struct {
	RunID                    int64
	UnderlyingID             string
	ObservedAt               time.Time
	Status                   string
	TokenizedMarketCapUSD    model.Decimal
	TokenizedVolume24hUSD    model.Decimal
	AverageTokenizedPriceUSD model.Decimal
	VersionsCount            int
	IssuersCount             int
	TokensInDefiCount        int
	HistoryCounts
	HistoryYield
	EligibleLiquidityUSD model.Decimal
}

// TokenHistoryRow is a history_tokens row.
type TokenHistoryRow struct {
	RunID            int64
	TokenID          int64
	ObservedAt       time.Time
	PriceUSD         model.Decimal
	MarketCapUSD     model.Decimal
	Volume24hUSD     model.Decimal
	UsefulnessScore  int
	RankInUnderlying *int
	HistoryCounts
	UnconfirmedCandidatesCount int
	HistoryYield
	DeploymentsCount        int
	IndexedDeploymentsCount int
	EligibleLiquidityUSD    model.Decimal
	Breakdowns              []byte // JSON {by_network, by_category, by_platform}
}

// IssuerHistoryRow is a history_issuers row.
type IssuerHistoryRow struct {
	RunID                       int64
	IssuerID                    string
	ObservedAt                  time.Time
	IssuerUsefulnessScore       int
	BasketRelativeScore         model.Decimal
	ComparedUnderlyingsCount    int
	BasketUnderlyingsCount      int
	BasketTokensCount           int
	TokensInDefiCount           int
	DeploymentsCount            int
	SupportedDeploymentsCount   int
	UnsupportedDeploymentsCount int
	HistoryCounts
	HistoryYield
	DirectPoolsLiquidityUSD model.Decimal
	BasketMarketCapUSD      model.Decimal
	RosterSize              *int
	Breakdowns              []byte // JSON {by_network, by_category, by_underlying}
}

// PositionHistoryRow is a history_positions row.
type PositionHistoryRow struct {
	RunID            int64
	PositionKey      string
	ObservedAt       time.Time
	LiquidityUSD     model.Decimal
	APYPercent       model.Decimal
	BaseAPYPercent   model.Decimal
	RewardAPYPercent model.Decimal
	BorrowAPYPercent model.Decimal
	VolumeUSD1d      model.Decimal
	VolumeUSD7d      model.Decimal
	SourceUpdatedAt  *time.Time
	Eligibility      string
	Flags            []string
}

func (c HistoryCounts) values() []any {
	return []any{c.RawConfirmedOpportunitiesCount, c.EligibleOpportunitiesCount, c.UnknownLiquidityCount,
		c.DirectEligibleCount, c.IndirectEligibleCount, c.EligibleProtocolsCount, c.EligibleNetworksCount}
}

func (y HistoryYield) values() []any {
	return []any{y.YieldOpportunitiesCount, numeric(y.MaxReportedAPYPercent), numeric(y.AverageReportedAPYPercent), numeric(y.MedianReportedAPYPercent)}
}

func concat(parts ...[]any) []any {
	var out []any
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

var countCols = []string{"raw_confirmed_opportunities_count", "eligible_opportunities_count", "unknown_liquidity_count",
	"direct_eligible_count", "indirect_eligible_count", "eligible_protocols_count", "eligible_networks_count"}

var yieldCols = []string{"yield_opportunities_count", "max_reported_apy_percent", "average_reported_apy_percent", "median_reported_apy_percent"}

func cols(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func (s *Store) copyRows(ctx context.Context, table string, columns []string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	n, err := s.q.CopyFrom(ctx, pgx.Identifier{table}, columns, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("store: insert %s: %w", table, err)
	}
	if int(n) != len(rows) {
		return fmt.Errorf("store: insert %s: copied %d of %d rows", table, n, len(rows))
	}
	return nil
}

// InsertSummaryHistory appends summary rows (one per run).
func (s *Store) InsertSummaryHistory(ctx context.Context, rows []SummaryHistoryRow) error {
	columns := cols([]string{"run_id", "observed_at", "underlyings_count", "tokens_count", "excluded_tokens_count", "issuers_count",
		"deployments_count", "indexed_deployments_count", "tokens_in_defi_count"}, countCols, yieldCols,
		[]string{"tokenized_market_cap_usd", "tokenized_volume_24h_usd", "eligible_liquidity_usd"})
	var data [][]any
	for _, r := range rows {
		data = append(data, concat(
			[]any{r.RunID, r.ObservedAt, r.UnderlyingsCount, r.TokensCount, r.ExcludedTokensCount, r.IssuersCount,
				r.DeploymentsCount, r.IndexedDeploymentsCount, r.TokensInDefiCount},
			r.HistoryCounts.values(), //nolint:staticcheck // QF1008: name the embedded part, as the other rows do
			HistoryYield{r.YieldOpportunitiesCount, r.MaxReportedAPYPercent, r.AverageReportedAPYPercent, r.MedianReportedAPYPercent}.values(),
			[]any{numeric(r.TokenizedMarketCapUSD), numeric(r.TokenizedVolume24hUSD), numeric(r.EligibleLiquidityUSD)}))
	}
	return s.copyRows(ctx, "history_summary", columns, data)
}

// InsertUnderlyingHistory appends underlying rows.
func (s *Store) InsertUnderlyingHistory(ctx context.Context, rows []UnderlyingHistoryRow) error {
	columns := cols([]string{"run_id", "underlying_id", "observed_at", "status", "tokenized_market_cap_usd", "tokenized_volume_24h_usd",
		"average_tokenized_price_usd", "versions_count", "issuers_count", "tokens_in_defi_count"}, countCols, yieldCols,
		[]string{"eligible_liquidity_usd"})
	var data [][]any
	for _, r := range rows {
		data = append(data, concat(
			[]any{r.RunID, r.UnderlyingID, r.ObservedAt, r.Status, numeric(r.TokenizedMarketCapUSD), numeric(r.TokenizedVolume24hUSD),
				numeric(r.AverageTokenizedPriceUSD), r.VersionsCount, r.IssuersCount, r.TokensInDefiCount},
			r.HistoryCounts.values(), r.HistoryYield.values(), []any{numeric(r.EligibleLiquidityUSD)}))
	}
	return s.copyRows(ctx, "history_underlyings", columns, data)
}

// InsertTokenHistory appends token rows.
func (s *Store) InsertTokenHistory(ctx context.Context, rows []TokenHistoryRow) error {
	columns := cols([]string{"run_id", "token_id", "observed_at", "price_usd", "market_cap_usd", "volume_24h_usd", "usefulness_score",
		"rank_in_underlying"}, countCols, []string{"unconfirmed_candidates_count"}, yieldCols,
		[]string{"deployments_count", "indexed_deployments_count", "eligible_liquidity_usd", "breakdowns"})
	var data [][]any
	for _, r := range rows {
		data = append(data, concat(
			[]any{r.RunID, r.TokenID, r.ObservedAt, numeric(r.PriceUSD), numeric(r.MarketCapUSD), numeric(r.Volume24hUSD),
				r.UsefulnessScore, r.RankInUnderlying},
			r.HistoryCounts.values(), []any{r.UnconfirmedCandidatesCount}, r.HistoryYield.values(),
			[]any{r.DeploymentsCount, r.IndexedDeploymentsCount, numeric(r.EligibleLiquidityUSD), jsonOr(r.Breakdowns, "{}")}))
	}
	return s.copyRows(ctx, "history_tokens", columns, data)
}

// InsertIssuerHistory appends issuer rows.
func (s *Store) InsertIssuerHistory(ctx context.Context, rows []IssuerHistoryRow) error {
	columns := cols([]string{"run_id", "issuer_id", "observed_at", "issuer_usefulness_score", "basket_relative_score",
		"compared_underlyings_count", "basket_underlyings_count", "basket_tokens_count", "tokens_in_defi_count", "deployments_count",
		"supported_deployments_count", "unsupported_deployments_count"}, countCols, yieldCols,
		[]string{"direct_pools_liquidity_usd", "basket_market_cap_usd", "roster_size", "breakdowns"})
	var data [][]any
	for _, r := range rows {
		data = append(data, concat(
			[]any{r.RunID, r.IssuerID, r.ObservedAt, r.IssuerUsefulnessScore, numeric(r.BasketRelativeScore),
				r.ComparedUnderlyingsCount, r.BasketUnderlyingsCount, r.BasketTokensCount, r.TokensInDefiCount, r.DeploymentsCount,
				r.SupportedDeploymentsCount, r.UnsupportedDeploymentsCount},
			r.HistoryCounts.values(), r.HistoryYield.values(),
			[]any{numeric(r.DirectPoolsLiquidityUSD), numeric(r.BasketMarketCapUSD), r.RosterSize, jsonOr(r.Breakdowns, "{}")}))
	}
	return s.copyRows(ctx, "history_issuers", columns, data)
}

// InsertPositionHistory appends position rows.
func (s *Store) InsertPositionHistory(ctx context.Context, rows []PositionHistoryRow) error {
	columns := []string{"run_id", "position_key", "observed_at", "liquidity_usd", "apy_percent", "base_apy_percent", "reward_apy_percent",
		"borrow_apy_percent", "volume_usd_1d", "volume_usd_7d", "source_updated_at", "eligibility", "flags"}
	var data [][]any
	for _, r := range rows {
		data = append(data, []any{r.RunID, r.PositionKey, r.ObservedAt, numeric(r.LiquidityUSD), numeric(r.APYPercent),
			numeric(r.BaseAPYPercent), numeric(r.RewardAPYPercent), numeric(r.BorrowAPYPercent), numeric(r.VolumeUSD1d),
			numeric(r.VolumeUSD7d), r.SourceUpdatedAt, r.Eligibility, strs(r.Flags)})
	}
	return s.copyRows(ctx, "history_positions", columns, data)
}

// ---------- queries ----------

// Entity names accepted by QueryHistory.
const (
	EntitySummary    = "summary"
	EntityUnderlying = "underlyings"
	EntityToken      = "tokens"
	EntityIssuer     = "issuers"
	EntityPosition   = "opportunities" // history_positions, addressed by opportunity key
)

// History steps accepted by QueryHistory.
const (
	StepRun  = "run"
	StepDay  = "day"
	StepWeek = "week"
)

var (
	// ErrUnknownMetric is matched (errors.Is) by every UnknownMetricError.
	ErrUnknownMetric = errors.New("store: unknown history metric")
	// ErrUnknownEntity is returned for an entity QueryHistory does not know.
	ErrUnknownEntity = errors.New("store: unknown history entity")
	// ErrBadID is returned when an entity id cannot be valid (for example a non-numeric token id).
	ErrBadID = errors.New("store: bad history entity id")
	// ErrUnknownStep is returned for a step other than run, day or week.
	ErrUnknownStep = errors.New("store: unknown history step")
)

// UnknownMetricError names the rejected metric.
type UnknownMetricError struct {
	Entity string
	Metric string
}

func (e *UnknownMetricError) Error() string {
	return fmt.Sprintf("store: unknown history metric %q for %s", e.Metric, e.Entity)
}

// Is makes errors.Is(err, ErrUnknownMetric) true.
func (e *UnknownMetricError) Is(target error) bool { return target == ErrUnknownMetric }

type metricKind int

const (
	kindInt metricKind = iota
	kindNumeric
	kindText
	kindTime
	kindTextArray
	kindJSON
)

type historyEntity struct {
	table   string
	idCol   string // "" for summary
	metrics map[string]metricKind
}

func metricSet(parts ...map[string]metricKind) map[string]metricKind {
	out := map[string]metricKind{}
	for _, p := range parts {
		for k, v := range p {
			out[k] = v
		}
	}
	return out
}

func kinds(kind metricKind, names ...string) map[string]metricKind {
	m := map[string]metricKind{}
	for _, n := range names {
		m[n] = kind
	}
	return m
}

var (
	countMetrics = kinds(kindInt, countCols...)
	yieldMetrics = metricSet(kinds(kindInt, "yield_opportunities_count"),
		kinds(kindNumeric, "max_reported_apy_percent", "average_reported_apy_percent", "median_reported_apy_percent"))
)

// historyEntities is the whitelist of queryable metrics, derived from the history table columns
// (keys and run_id/observed_at are returned separately). Metric names are interpolated into SQL only
// after matching this map.
var historyEntities = map[string]historyEntity{
	EntitySummary: {"history_summary", "", metricSet(countMetrics, yieldMetrics,
		kinds(kindInt, "underlyings_count", "tokens_count", "excluded_tokens_count", "issuers_count", "deployments_count",
			"indexed_deployments_count", "tokens_in_defi_count"),
		kinds(kindNumeric, "tokenized_market_cap_usd", "tokenized_volume_24h_usd", "eligible_liquidity_usd"))},
	EntityUnderlying: {"history_underlyings", "underlying_id", metricSet(countMetrics, yieldMetrics,
		kinds(kindText, "status"),
		kinds(kindNumeric, "tokenized_market_cap_usd", "tokenized_volume_24h_usd", "average_tokenized_price_usd", "eligible_liquidity_usd"),
		kinds(kindInt, "versions_count", "issuers_count", "tokens_in_defi_count"))},
	EntityToken: {"history_tokens", "token_id", metricSet(countMetrics, yieldMetrics,
		kinds(kindNumeric, "price_usd", "market_cap_usd", "volume_24h_usd", "eligible_liquidity_usd"),
		kinds(kindInt, "usefulness_score", "rank_in_underlying", "unconfirmed_candidates_count", "deployments_count", "indexed_deployments_count"),
		kinds(kindJSON, "breakdowns"))},
	EntityIssuer: {"history_issuers", "issuer_id", metricSet(countMetrics, yieldMetrics,
		kinds(kindInt, "issuer_usefulness_score", "compared_underlyings_count", "basket_underlyings_count", "basket_tokens_count",
			"tokens_in_defi_count", "deployments_count", "supported_deployments_count", "unsupported_deployments_count", "roster_size"),
		kinds(kindNumeric, "basket_relative_score", "direct_pools_liquidity_usd", "basket_market_cap_usd"),
		kinds(kindJSON, "breakdowns"))},
	EntityPosition: {"history_positions", "position_key", metricSet(
		kinds(kindNumeric, "liquidity_usd", "apy_percent", "base_apy_percent", "reward_apy_percent", "borrow_apy_percent",
			"volume_usd_1d", "volume_usd_7d"),
		kinds(kindTime, "source_updated_at"), kinds(kindText, "eligibility"), kinds(kindTextArray, "flags"))},
}

// HistoryMetrics returns the sorted metric names QueryHistory accepts for an entity.
func HistoryMetrics(entity string) []string {
	e, ok := historyEntities[entity]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(e.metrics))
	for n := range e.metrics {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// HistoryPoint is one observation. Values holds the requested metrics: NUMERIC as decimal strings (no
// exponent), integers as int32, text as string, timestamps as time.Time, arrays as []any, JSONB decoded to any;
// NULL is nil.
type HistoryPoint struct {
	RunID      int64
	ObservedAt time.Time
	Values     map[string]any
}

// Default and maximum number of points QueryHistory returns.
const (
	DefaultHistoryLimit = 1000
	MaxHistoryLimit     = 5000
)

// HistoryResult is the outcome of QueryHistory.
type HistoryResult struct {
	Points []HistoryPoint // ascending by time
	// Truncated is true when more points matched than limit: Points then holds the NEWEST limit points.
	Truncated bool
}

// QueryHistory returns the series of metrics for one entity, ascending by time. id is ignored for the summary
// entity. from/to bound observed_at (nil = open). step is run (every run, default), day or week (the last
// observation of each UTC day / ISO week). limit defaults to 1000 and is capped at 5000; when more points
// match, the newest ones are returned and Truncated is set. Unknown metrics yield an *UnknownMetricError
// (errors.Is(err, ErrUnknownMetric)); no metrics = all.
func (s *Store) QueryHistory(ctx context.Context, entity, id string, metrics []string, from, to *time.Time, step string, limit int) (*HistoryResult, error) {
	ent, ok := historyEntities[entity]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownEntity, entity)
	}
	if len(metrics) == 0 {
		metrics = HistoryMetrics(entity)
	}
	selects := make([]string, 0, len(metrics))
	seen := map[string]bool{}
	var names []string
	for _, m := range metrics {
		kind, ok := ent.metrics[m]
		if !ok {
			return nil, &UnknownMetricError{Entity: entity, Metric: m}
		}
		if seen[m] {
			continue
		}
		seen[m] = true
		names = append(names, m)
		switch kind {
		case kindNumeric:
			selects = append(selects, m+"::text")
		default:
			selects = append(selects, m)
		}
	}
	if limit <= 0 {
		limit = DefaultHistoryLimit
	}
	limit = min(limit, MaxHistoryLimit)

	var trunc string
	switch step {
	case "", StepRun:
	case StepDay:
		trunc = "day"
	case StepWeek:
		trunc = "week"
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownStep, step)
	}

	var sb strings.Builder
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	list := "run_id, observed_at, " + strings.Join(selects, ", ")
	bucket := "date_trunc('" + trunc + "', observed_at AT TIME ZONE 'UTC')"
	if trunc != "" {
		sb.WriteString("SELECT DISTINCT ON (" + bucket + ") " + list)
	} else {
		sb.WriteString("SELECT " + list)
	}
	sb.WriteString(" FROM " + ent.table + " WHERE TRUE")
	switch ent.idCol {
	case "":
	case "token_id": // INT column: the id arrives as a string
		n, err := strconv.ParseInt(id, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("%w: token id %q", ErrBadID, id)
		}
		sb.WriteString(" AND token_id = " + arg(n))
	default:
		sb.WriteString(" AND " + ent.idCol + " = " + arg(id))
	}
	if from != nil {
		sb.WriteString(" AND observed_at >= " + arg(*from))
	}
	if to != nil {
		sb.WriteString(" AND observed_at <= " + arg(*to))
	}
	if trunc != "" {
		sb.WriteString(" ORDER BY " + bucket + ", observed_at DESC, run_id DESC")
	} else {
		sb.WriteString(" ORDER BY observed_at, run_id")
	}
	inner := sb.String()
	// Newest limit+1 points (to detect truncation), reversed to ascending below.
	query := "SELECT * FROM (" + inner + ") h ORDER BY observed_at DESC, run_id DESC LIMIT " + arg(limit+1)

	rows, err := s.q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query %s history: %w", entity, err)
	}
	defer rows.Close()
	out := []HistoryPoint{}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("store: query %s history: scan: %w", entity, err)
		}
		p := HistoryPoint{Values: make(map[string]any, len(names))}
		p.RunID, _ = vals[0].(int64)
		p.ObservedAt, _ = vals[1].(time.Time)
		for i, n := range names {
			p.Values[n] = vals[2+i]
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: query %s history: %w", entity, err)
	}
	res := &HistoryResult{}
	if len(out) > limit {
		res.Truncated = true
		out = out[:limit]
	}
	slices.Reverse(out)
	res.Points = out
	return res, nil
}
