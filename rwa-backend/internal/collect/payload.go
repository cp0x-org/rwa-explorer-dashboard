package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/compute"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/cmc"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/portals"
)

// Snapshot statuses.
const (
	StatusOK              = "ok"
	StatusNoTrackedTokens = "no_tracked_tokens"
)

// SchemaVersion is the snapshot document version (the 1.x format).
const SchemaVersion = 1

// timeFormat is the 1.x timestamp format (now_iso).
const timeFormat = "2006-01-02T15:04:05Z"

// snapshotDoc is the 1.x snapshot document. Field order matches 1.x.
type snapshotDoc struct {
	SchemaVersion int          `json:"schema_version"`
	UnderlyingID  string       `json:"underlying_id"`
	Ticker        string       `json:"ticker"`
	Name          string       `json:"name"`
	ISIN          *string      `json:"isin"`
	AssetType     string       `json:"asset_type"`
	RunID         string       `json:"run_id"`
	FetchedAt     string       `json:"fetched_at"`
	Status        string       `json:"status"`
	CMC           *cmcPart     `json:"cmc"`
	Portals       *portalsPart `json:"portals"`
}

type cmcPart struct {
	RWA        json.RawMessage            `json:"rwa"`
	Info       map[string]json.RawMessage `json:"info"`
	InvalidIDs []int                      `json:"invalid_ids"`
}

type portalsPart struct {
	Networks []string                      `json:"networks"`
	LookedUp []string                      `json:"looked_up"`
	Exact    map[string]json.RawMessage    `json:"exact"`
	Searches map[string]portals.SearchMeta `json:"searches"`
	Records  map[string]json.RawMessage    `json:"records"`
}

// infoKeys is the subset of /v2/cryptocurrency/info a snapshot keeps (1.x _info_subset).
var infoKeys = []string{"id", "symbol", "name", "slug", "logo", "date_added", "platform", "contract_address"}

// infoSubset keeps only infoKeys of the raw entry; missing keys become null like Python's item.get(k).
func infoSubset(raw json.RawMessage) (json.RawMessage, error) {
	var full map[string]json.RawMessage
	if err := json.Unmarshal(raw, &full); err != nil {
		return nil, fmt.Errorf("decode info entry: %w", err)
	}
	sub := make(map[string]json.RawMessage, len(infoKeys))
	for _, k := range infoKeys {
		v, ok := full[k]
		if !ok {
			v = json.RawMessage("null")
		}
		sub[k] = v
	}
	b, err := json.Marshal(sub)
	if err != nil {
		return nil, fmt.Errorf("encode info subset: %w", err)
	}
	return b, nil
}

// shared is what the global steps [1/5]-[5/5] produced and every underlying needs.
type shared struct {
	info     map[int]cmc.Info
	invalid  []int
	networks []string
}

// portalsAPI is the part of the Portals client the per-underlying collection uses.
type portalsAPI interface {
	Lookup(ctx context.Context, keys []string, limit int) (map[string]json.RawMessage, error)
	Search(ctx context.Context, term string, limit int) (map[string]json.RawMessage, portals.SearchMeta, error)
}

// collectAsset is collect.py::collect_asset. It returns the snapshot payload and its status; an error means
// the snapshot must not be stored (previous one stays).
func collectAsset(ctx context.Context, log *slog.Logger, asset model.Asset, rwa *cmc.RWAAsset, reason string, sh shared,
	api portalsAPI, settings *model.Settings, runKey string, now func() time.Time) ([]byte, string, error) {
	doc := snapshotDoc{
		SchemaVersion: SchemaVersion,
		UnderlyingID:  asset.ID(),
		Ticker:        asset.Ticker,
		Name:          asset.Name,
		ISIN:          asset.ISIN,
		AssetType:     asset.AssetType,
		RunID:         runKey,
		FetchedAt:     now().UTC().Format(timeFormat),
	}
	finish := func(status string) ([]byte, string, error) {
		doc.Status = status
		b, err := json.Marshal(doc)
		if err != nil {
			return nil, "", fmt.Errorf("encode snapshot: %w", err)
		}
		return b, status, nil
	}
	if rwa == nil {
		log.Info("nothing to fetch from Portals", "reason", reason)
		return finish(reason)
	}

	var tokens []model.CMCToken
	for _, t := range rwa.Tokens {
		if t.CryptoID != nil && *t.CryptoID != 0 {
			tokens = append(tokens, t)
		}
	}
	tokenIDs := map[int]bool{}
	tokenInfo := map[string]json.RawMessage{}
	for _, t := range tokens {
		id := int(*t.CryptoID)
		tokenIDs[id] = true
		if in, ok := sh.info[id]; ok {
			sub, err := infoSubset(in.Raw)
			if err != nil {
				return nil, "", fmt.Errorf("token %d: %w", id, err)
			}
			tokenInfo[strconv.Itoa(id)] = sub
		}
	}
	invalid := []int{}
	for _, id := range sh.invalid {
		if tokenIDs[id] {
			invalid = append(invalid, id)
		}
	}
	doc.CMC = &cmcPart{RWA: rwa.Raw, Info: tokenInfo, InvalidIDs: invalid}
	if len(tokens) == 0 {
		log.Info("CMC tracks no tokenized assets")
		return finish(StatusNoTrackedTokens)
	}

	networkSet := map[string]bool{}
	for _, n := range sh.networks {
		networkSet[n] = true
	}
	var keys []string
	var included []model.CMCToken
	var includedSymbols []string
	for _, t := range tokens {
		var infoEntry *model.CMCInfo
		if in, ok := sh.info[int(*t.CryptoID)]; ok {
			infoEntry = &in.CMCInfo
		}
		for _, d := range compute.ParseDeployments(infoEntry, settings.ChainMapping) {
			if d.Key != nil && *d.Key != "" && d.Network != nil && networkSet[*d.Network] {
				keys = append(keys, *d.Key)
			}
		}
		if kind, _ := compute.ClassifyToken(t, asset.Ticker, asset.SymbolAliases...); !compute.ExcludedKinds[kind] {
			included = append(included, t)
			if t.Symbol != nil && *t.Symbol != "" {
				includedSymbols = append(includedSymbols, *t.Symbol)
			}
		}
	}
	lookedUp := slices.Clone(keys)
	slices.Sort(lookedUp)
	lookedUp = slices.Compact(lookedUp)
	if lookedUp == nil {
		lookedUp = []string{}
	}
	log.Info("tokens in CMC", "tokens", len(tokens), "tokenized", len(included),
		"symbols", includedSymbols, "contracts_on_portals_networks", len(lookedUp))

	limit := settings.PortalsPageLimit
	exact := map[string]json.RawMessage{}
	if len(keys) > 0 {
		var err error
		if exact, err = api.Lookup(ctx, keys, limit); err != nil {
			return nil, "", err
		}
		log.Info("lookup done", "indexed", len(exact), "of", len(lookedUp))
	}

	terms := portals.MinimalTerms(append([]string{asset.Ticker}, includedSymbols...))
	records := map[string]json.RawMessage{}
	searches := map[string]portals.SearchMeta{}
	for _, term := range terms {
		t0 := time.Now()
		found, meta, err := api.Search(ctx, term, limit)
		if err != nil {
			return nil, "", err
		}
		for k, v := range found {
			records[k] = v
		}
		searches[term] = meta
		log.Info("search done", "term", term, "positions", len(found), "pages", meta.Pages,
			"complete", meta.Complete, "took", time.Since(t0).Round(time.Millisecond))
	}

	doc.FetchedAt = now().UTC().Format(timeFormat)
	networks := sh.networks
	if networks == nil {
		networks = []string{}
	}
	doc.Portals = &portalsPart{Networks: networks, LookedUp: lookedUp, Exact: exact, Searches: searches, Records: records}
	return finish(StatusOK)
}
