package model

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Asset is one underlying from config/assets.json.
type Asset struct {
	Ticker    string
	Name      string
	AssetType string // stock | etf
	ISIN      *string
	Enabled   bool
}

// ID is the underlying id: the ISIN, or "<type>:ticker:<TICKER>" for assets without one.
func (a Asset) ID() string {
	if a.ISIN != nil && *a.ISIN != "" {
		return *a.ISIN
	}
	return a.AssetType + ":ticker:" + a.Ticker
}

// StorageID is the file-name-safe form of the id.
func (a Asset) StorageID() string { return StorageID(a.ID()) }

var storageUnsafe = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// StorageID replaces every character outside [A-Za-z0-9_.-] with "_" (an ISIN stays as is).
func StorageID(underlyingID string) string { return storageUnsafe.ReplaceAllString(underlyingID, "_") }

var isinFormat = regexp.MustCompile(`^[A-Z]{2}[A-Z0-9]{9}[0-9]$`)

// ISINIsValid checks the ISIN format and its Luhn check digit.
func ISINIsValid(isin string) bool {
	if !isinFormat.MatchString(isin) {
		return false
	}
	var digits []byte
	for _, ch := range isin[:len(isin)-1] {
		switch {
		case ch >= '0' && ch <= '9':
			digits = append(digits, byte(ch-'0'))
		default: // letters expand to two digits: A=10 ... Z=35
			v := int(ch-'A') + 10
			digits = append(digits, byte(v/10), byte(v%10))
		}
	}
	total := 0
	// Luhn over the expanded digit string, rightmost digit doubled first.
	for i := range digits {
		d := int(digits[len(digits)-1-i])
		if i%2 == 0 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		total += d
	}
	return (10-total%10)%10 == int(isin[len(isin)-1]-'0')
}

type rawAsset struct {
	Ticker    string  `json:"ticker"`
	Name      string  `json:"name"`
	AssetType string  `json:"asset_type"`
	ISIN      *string `json:"isin"`
	Enabled   *bool   `json:"enabled"`
}

// LoadAssets reads assets.json with the validation of the 1.x pipeline: ticker required (uppercased),
// asset_type stock|etf, valid ISIN, no duplicate ticker or id.
func LoadAssets(path string) ([]Asset, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path comes from operator configuration
	if err != nil {
		return nil, fmt.Errorf("model: read assets: %w", err)
	}
	var doc struct {
		Assets []rawAsset `json:"assets"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("model: parse assets %s: %w", path, err)
	}
	assets := make([]Asset, 0, len(doc.Assets))
	seenIDs, seenTickers := map[string]bool{}, map[string]bool{}
	for i, item := range doc.Assets {
		ticker := strings.ToUpper(strings.TrimSpace(item.Ticker))
		if ticker == "" {
			return nil, fmt.Errorf("model: assets[%d]: ticker is required", i)
		}
		if item.AssetType != "stock" && item.AssetType != "etf" {
			return nil, fmt.Errorf("model: %s: asset_type must be one of [etf stock]", ticker)
		}
		var isin *string
		if item.ISIN != nil && *item.ISIN != "" {
			if !ISINIsValid(*item.ISIN) {
				return nil, fmt.Errorf("model: %s: invalid ISIN %q (format or check digit)", ticker, *item.ISIN)
			}
			isin = item.ISIN
		}
		name := item.Name
		if name == "" {
			name = ticker
		}
		a := Asset{Ticker: ticker, Name: name, AssetType: item.AssetType, ISIN: isin, Enabled: item.Enabled == nil || *item.Enabled}
		if seenIDs[a.ID()] || seenTickers[ticker] {
			return nil, fmt.Errorf("model: %s: duplicate ticker or ISIN", ticker)
		}
		seenIDs[a.ID()] = true
		seenTickers[ticker] = true
		assets = append(assets, a)
	}
	return assets, nil
}
