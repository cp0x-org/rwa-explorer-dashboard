package compute

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// Opp is a normalized Portals position: the export record plus the raw composition (child keys; the export
// resolves them to {key, symbol} components at the end of Build).
type Opp struct {
	model.Opportunity
	ComponentKeys []string
}

// NormKey is analyze.py::norm_key: "<network>:<address>" with an EVM address lower-cased, non-EVM as is.
func NormKey(key string) string {
	network, address, _ := strings.Cut(key, ":")
	return network + ":" + NormalizeAddress(address)
}

// ChildKeys is analyze.py::child_keys: composition from tokens[]; bare addresses inherit the position's network.
func ChildKeys(record model.PortalsRecord) []string {
	var out []string
	for _, ref := range record.Tokens {
		if ref == "" {
			continue
		}
		if strings.Contains(ref, ":") {
			out = append(out, NormKey(ref))
		} else {
			network := "None" // Python formats a missing network as "None"
			if record.Network != nil {
				network = *record.Network
			}
			out = append(out, network+":"+NormalizeAddress(ref))
		}
	}
	return out
}

// ClassifyPosition is analyze.py::classify_position: platform first, then the first matching tag.
func ClassifyPosition(record model.PortalsRecord, settings *model.Settings) string {
	if record.Platform != nil {
		if c, ok := settings.PlatformCategories[*record.Platform]; ok {
			return c
		}
	}
	tags := map[string]bool{}
	if record.Metadata != nil {
		for _, t := range record.Metadata.Tags {
			tags[t] = true
		}
	}
	for _, tc := range settings.TagCategories {
		if tags[tc[0]] {
			return tc[1]
		}
	}
	return "unknown"
}

// NormalizeOpportunity is analyze.py::normalize_opportunity. Eligibility is filled in by the caller (Eligibility).
func NormalizeOpportunity(record model.PortalsRecord, settings *model.Settings, platformNames map[string]string) Opp {
	var metrics model.PortalsMetrics
	if record.Metrics != nil {
		metrics = *record.Metrics
	}
	var tags []string
	if record.Metadata != nil {
		tags = record.Metadata.Tags
	}
	category := ClassifyPosition(record, settings)
	apy := metrics.APY.Decimal
	liquidity := record.Liquidity.Decimal
	price := record.Price.Decimal
	address := record.Address
	isPortable := record.IsPortable

	var flags []string
	if !price.IsNull() && price.V.GreaterThanOrEqual(SentinelPrice) {
		flags = append(flags, "sentinel_price")
	}
	switch {
	case apy.IsNull():
		flags = append(flags, "apy_missing")
	case apy.V.GreaterThan(*settings.YieldAPYReviewThresholdPercent.V):
		flags = append(flags, "apy_anomaly")
	case apy.V.IsNegative():
		flags = append(flags, "apy_negative")
	}
	for _, v := range []model.Decimal{metrics.VolumeUSD1d.Decimal, metrics.VolumeUSD7d.Decimal} {
		if !v.IsNull() && v.V.IsNegative() { // `(dec(x) or 0) < 0`: None counts as 0
			flags = append(flags, "negative_volume_net_flow")
			break
		}
	}
	if category == "dex_lp" && contains(tags, "#clp") {
		flags = append(flags, "range_details_unknown")
	}
	if liquidity.IsNull() {
		flags = append(flags, "liquidity_missing")
	}

	// model.Opportunity carries platform/network as strings (the positions table is NOT NULL on both), so a null in
	// the source becomes ""; the 1.x data has none, and breakdown() buckets "" as "unknown" like `or "unknown"`.
	platform := deref(record.Platform)
	platformName := platform
	if n, ok := platformNames[platform]; ok {
		platformName = n
	}
	semantics, ok := LiquiditySemantics[category]
	if !ok {
		semantics = "unknown"
	}
	key := NormKey(record.Key)
	positionKind := "contract"
	if utf8.RuneCountInString(address) == 66 { // 32-byte V4/Infinity pool ids are not ERC-20 contracts
		positionKind = "pool_id"
	}
	var receipt *string
	if isPortable != nil && *isPortable && utf8.RuneCountInString(address) == 42 {
		receipt = &key
	}
	return Opp{
		Opportunity: model.Opportunity{
			Key:               key,
			Network:           deref(record.Network),
			Platform:          platform,
			PlatformName:      platformName,
			Symbol:            record.Symbol,
			Name:              model.NormalizeName(record.Name),
			Address:           address,
			PositionKind:      positionKind,
			ReceiptTokenKey:   receipt,
			Category:          category,
			Tags:              tags,
			LiquidityUSD:      liquidity,
			LiquiditySemantic: semantics,
			APYPercent:        apy,
			BaseAPYPercent:    metrics.BaseAPY.Decimal,
			RewardAPYPercent:  metrics.RewardAPY.Decimal,
			BorrowAPYPercent:  metrics.BorrowAPY.Decimal,
			VolumeUSD1d:       metrics.VolumeUSD1d.Decimal,
			VolumeUSD7d:       metrics.VolumeUSD7d.Decimal,
			IsPortable:        isPortable,
			UpdatedAt:         record.UpdatedAt,
			Flags:             flags,
		},
		ComponentKeys: ChildKeys(record),
	}
}

// Eligibility is analyze.py::eligibility. Freshness is the source age at collection time (fetchedAt), so a saved
// snapshot does not decay on its own.
func Eligibility(opp *Opp, fetchedAt time.Time, settings *model.Settings) string {
	if !UseCategories[opp.Category] {
		return "category_excluded"
	}
	var updated time.Time
	ok := false
	if opp.UpdatedAt != nil {
		updated, ok = ParseTime(*opp.UpdatedAt)
	}
	if !ok || fetchedAt.IsZero() {
		return "unknown_freshness"
	}
	if fetchedAt.Sub(updated) > time.Duration(settings.MaxPositionAgeHours)*time.Hour {
		return "stale"
	}
	if opp.LiquidityUSD.IsNull() || opp.LiquiditySemantic == "unknown" {
		return "unknown_liquidity"
	}
	if opp.LiquidityUSD.V.LessThan(*settings.MinEligibleLiquidityUSD.V) {
		return "low_liquidity"
	}
	return "eligible"
}
