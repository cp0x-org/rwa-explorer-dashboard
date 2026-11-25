package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// Snapshot is the 1.x per-underlying snapshot document (1.x data/snapshots/<id>.json; samples in testdata/snapshots/v1).
// Only the fields analyze.py reads are modelled; the rest of the document stays in the database payload.
// Numeric fields are LenientDecimal (lenient like 1.x dec()): CMC sends JSON numbers, Portals metrics are strings.
type Snapshot struct {
	SchemaVersion int       `json:"schema_version"`
	UnderlyingID  string    `json:"underlying_id"`
	Ticker        string    `json:"ticker"`
	Name          string    `json:"name"`
	ISIN          *string   `json:"isin"`
	AssetType     string    `json:"asset_type"`
	RunID         string    `json:"run_id"`
	FetchedAt     time.Time `json:"fetched_at"`
	// Status is ok | no_tracked_tokens | not_in_cmc | rwa_id_changed | ambiguous_cmc_match.
	Status  string           `json:"status"`
	CMC     *SnapshotCMC     `json:"cmc"`
	Portals *SnapshotPortals `json:"portals"`
}

// ParseSnapshot decodes a snapshot document.
func ParseSnapshot(b []byte) (*Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("model: parse snapshot: %w", err)
	}
	return &s, nil
}

// SnapshotCMC is the CMC part: the RWA record, per-token info and ids CMC rejected.
type SnapshotCMC struct {
	RWA        *CMCRwa            `json:"rwa"`
	Info       map[string]CMCInfo `json:"info"`
	InvalidIDs []json.RawMessage  `json:"invalid_ids"`
}

// CMCRwa is the CMC RWA asset (quotes/latest) record.
type CMCRwa struct {
	Name                  *string        `json:"name"`
	Symbol                *string        `json:"symbol"`
	Slug                  *string        `json:"slug"`
	RwaID                 *int           `json:"rwa_id"`
	AssetType             *string        `json:"asset_type"`
	AverageTokenizedPrice LenientDecimal `json:"average_tokenized_price"`
	TokenizedMarketCap    LenientDecimal `json:"tokenized_market_cap"`
	TokenizedVolume24h    LenientDecimal `json:"tokenized_volume_24h"`
	LastUpdated           *string        `json:"last_updated"`
	Tokens                []CMCToken     `json:"tokens"`
}

// CMCToken is one tokenized asset of an RWA record.
type CMCToken struct {
	CryptoID   *int64         `json:"crypto_id"`
	Symbol     *string        `json:"symbol"`
	Name       *string        `json:"name"`
	Price      LenientDecimal `json:"price"`
	MarketCap  LenientDecimal `json:"market_cap"`
	Volume24h  LenientDecimal `json:"volume_24h"`
	IssuerID   *string        `json:"issuer_id"`
	IssuerName *string        `json:"issuer_name"`
}

// CMCInfo is the subset of /v2/cryptocurrency/info stored in a snapshot.
type CMCInfo struct {
	ID              *int64               `json:"id"`
	Symbol          *string              `json:"symbol"`
	Name            *string              `json:"name"`
	Slug            *string              `json:"slug"`
	Logo            *string              `json:"logo"`
	DateAdded       *string              `json:"date_added"`
	Platform        *CMCInfoPlatform     `json:"platform"`
	ContractAddress []CMCContractAddress `json:"contract_address"`
}

// CMCInfoPlatform is the main platform of a token.
type CMCInfoPlatform struct {
	Name         *string `json:"name"`
	TokenAddress *string `json:"token_address"`
}

// CMCContractAddress is one deployment entry.
type CMCContractAddress struct {
	ContractAddress string `json:"contract_address"`
	Platform        *struct {
		Name *string `json:"name"`
	} `json:"platform"`
}

// SnapshotPortals is the Portals part.
type SnapshotPortals struct {
	Networks []string                 `json:"networks"`
	LookedUp []string                 `json:"looked_up"`
	Exact    map[string]PortalsRecord `json:"exact"`
	Searches map[string]PortalsSearch `json:"searches"`
	Records  map[string]PortalsRecord `json:"records"`
}

// PortalsSearch is the bookkeeping of one search term.
type PortalsSearch struct {
	TotalItems *int `json:"total_items"`
	Pages      *int `json:"pages"`
	Received   *int `json:"received"`
	Complete   bool `json:"complete"`
}

// PortalsRecord is one Portals token/position record, with the fields analyze.py reads.
type PortalsRecord struct {
	Key        string           `json:"key"`
	Name       *string          `json:"name"`
	Symbol     *string          `json:"symbol"`
	Address    string           `json:"address"`
	Platform   *string          `json:"platform"`
	Network    *string          `json:"network"`
	Decimals   *int             `json:"decimals"`
	Price      LenientDecimal   `json:"price"`
	Liquidity  LenientDecimal   `json:"liquidity"`
	IsPortable *bool            `json:"isPortable"`
	UpdatedAt  *string          `json:"updatedAt"`
	CreatedAt  *string          `json:"createdAt"`
	Tokens     []string         `json:"tokens"`
	Metrics    *PortalsMetrics  `json:"metrics"`
	Metadata   *PortalsMetadata `json:"metadata"`
}

// PortalsMetrics are the position metrics; absent or null means "not reported".
type PortalsMetrics struct {
	APY         LenientDecimal `json:"apy"`
	BaseAPY     LenientDecimal `json:"baseApy"`
	RewardAPY   LenientDecimal `json:"rewardApy"`
	BorrowAPY   LenientDecimal `json:"borrowApy"`
	VolumeUSD1d LenientDecimal `json:"volumeUsd1d"`
	VolumeUSD7d LenientDecimal `json:"volumeUsd7d"`
}

// PortalsMetadata holds position tags and the string-valued metadata fields (link templates read them as
// "{meta.<key>}"; other value types never resolve there).
type PortalsMetadata struct {
	Tags    []string          `json:"tags"`
	Strings map[string]string `json:"-"`
}

// UnmarshalJSON reads the tags and keeps every string-valued field.
func (m *PortalsMetadata) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*m = PortalsMetadata{}
	if t, ok := raw["tags"]; ok {
		if err := json.Unmarshal(t, &m.Tags); err != nil {
			return err
		}
	}
	for k, v := range raw {
		var s string
		if len(v) > 0 && v[0] == '"' && json.Unmarshal(v, &s) == nil {
			if m.Strings == nil {
				m.Strings = map[string]string{}
			}
			m.Strings[k] = s
		}
	}
	return nil
}
