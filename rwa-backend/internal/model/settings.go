package model

import (
	"encoding/json"
	"fmt"
	"os"
)

// RequestsPerMinute is the per-provider rate limit.
type RequestsPerMinute struct {
	CMC     int `json:"cmc"`
	Portals int `json:"portals"`
}

// Settings is config/settings.json.
type Settings struct {
	SchemaVersion         int               `json:"schema_version"`
	PortalsPageLimit      int               `json:"portals_page_limit"`
	RequestsPerMinute     RequestsPerMinute `json:"requests_per_minute"`
	RequestTimeoutSeconds int               `json:"request_timeout_seconds"`
	MaxRetries            int               `json:"max_retries"`

	MaxGraphDepth                  int     `json:"max_graph_depth"`
	MinEligibleLiquidityUSD        Decimal `json:"min_eligible_liquidity_usd"`
	MaxPositionAgeHours            int     `json:"max_position_age_hours"`
	YieldAPYReviewThresholdPercent Decimal `json:"yield_apy_review_threshold_percent"`

	// ChainMapping maps a CMC network name to a Portals network slug. A present key with a null
	// value means "known, unsupported by Portals"; a missing key means "unmapped".
	ChainMapping       map[string]*string `json:"chain_mapping"`
	PlatformCategories map[string]string  `json:"platform_categories"`
	// TagCategories is an ordered list of [tag, category]; the first matching tag wins.
	TagCategories [][2]string `json:"tag_categories"`
}

// LoadSettings reads and validates settings.json.
func LoadSettings(path string) (*Settings, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path comes from operator configuration
	if err != nil {
		return nil, fmt.Errorf("model: read settings: %w", err)
	}
	var s Settings
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("model: parse settings %s: %w", path, err)
	}
	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("model: settings %s: %w", path, err)
	}
	return &s, nil
}

func (s *Settings) validate() error {
	switch {
	case s.MinEligibleLiquidityUSD.IsNull():
		return fmt.Errorf("min_eligible_liquidity_usd is required")
	case s.YieldAPYReviewThresholdPercent.IsNull():
		return fmt.Errorf("yield_apy_review_threshold_percent is required")
	case s.MaxGraphDepth < 1:
		return fmt.Errorf("max_graph_depth must be >= 1")
	case s.MaxPositionAgeHours < 1:
		return fmt.Errorf("max_position_age_hours must be >= 1")
	case s.PortalsPageLimit < 1:
		return fmt.Errorf("portals_page_limit must be >= 1")
	case s.RequestsPerMinute.CMC < 1 || s.RequestsPerMinute.Portals < 1:
		return fmt.Errorf("requests_per_minute must be positive for cmc and portals")
	case s.ChainMapping == nil:
		return fmt.Errorf("chain_mapping is required")
	}
	return nil
}

// ExportSettings returns the subset written to export.settings.
func (s *Settings) ExportSettings() ExportSettings {
	return ExportSettings{
		MinEligibleLiquidityUSD:        s.MinEligibleLiquidityUSD,
		MaxPositionAgeHours:            s.MaxPositionAgeHours,
		MaxGraphDepth:                  s.MaxGraphDepth,
		YieldAPYReviewThresholdPercent: s.YieldAPYReviewThresholdPercent,
	}
}
