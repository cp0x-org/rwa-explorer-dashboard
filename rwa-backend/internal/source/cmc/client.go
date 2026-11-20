// Package cmc is the CoinMarketCap source: RWA roster (quotes), per-token info with contract
// addresses, issuers and RWA metadata.
package cmc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/httpx"
)

// BaseURL is the CMC Pro API host.
const BaseURL = "https://pro-api.coinmarketcap.com"

// InfoBatch is the number of ids per /v2/cryptocurrency/info request.
const InfoBatch = 100

const provider = "cmc"

// Pick reasons returned by PickRWAAsset when no asset is chosen.
const (
	ReasonRWAIDChanged = "rwa_id_changed"
	ReasonNotInCMC     = "not_in_cmc"
	ReasonAmbiguous    = "ambiguous_cmc_match"
)

// NewHTTPClient builds the httpx client for CMC from settings. rawDir may be empty.
func NewHTTPClient(apiKey string, s *model.Settings, rawDir string, onRequest func(string, string, url.Values, int, time.Duration)) *httpx.Client {
	return httpx.New(httpx.Config{
		Provider:          provider,
		BaseURL:           BaseURL,
		Headers:           map[string]string{"X-CMC_PRO_API_KEY": apiKey},
		Timeout:           time.Duration(s.RequestTimeoutSeconds) * time.Second,
		RequestsPerMinute: s.RequestsPerMinute.CMC,
		MaxRetries:        s.MaxRetries,
		RawDir:            rawDir,
		OnRequest:         onRequest,
	})
}

// Client talks to CMC through an httpx client.
type Client struct {
	api *httpx.Client
	now func() time.Time
}

// New wraps an httpx client configured for CMC.
func New(api *httpx.Client) *Client { return &Client{api: api, now: time.Now} }

// RWAAsset is one record of quotes/latest data.rwa_assets: the parsed fields plus the verbatim JSON,
// which the snapshot payload stores byte-faithfully.
type RWAAsset struct {
	model.CMCRwa
	Raw json.RawMessage `json:"-"`
}

// Info is one /v2/cryptocurrency/info entry: parsed subset plus the verbatim JSON.
type Info struct {
	model.CMCInfo
	Raw json.RawMessage `json:"-"`
}

// RosterToken is one token of an issuer roster.
type RosterToken struct {
	CryptoID *int64  `json:"crypto_id"`
	Symbol   *string `json:"symbol"`
	RWAID    *int    `json:"rwa_id"`
}

// Issuer is the issuers.json entry of 1.x.
type Issuer struct {
	IssuerID   string        `json:"issuer_id"`
	Name       *string       `json:"name"`
	Website    *string       `json:"website"`
	Logo       *string       `json:"logo"`
	RosterSize int           `json:"roster_size"`
	Roster     []RosterToken `json:"roster"`
	FetchedAt  string        `json:"fetched_at"`
}

// RWAInfo is the rwa_info.json entry of 1.x.
type RWAInfo struct {
	RWAID     *int    `json:"rwa_id"`
	Name      *string `json:"name"`
	Website   *string `json:"website"`
	Logo      *string `json:"logo"`
	Industry  *string `json:"industry"`
	FetchedAt string  `json:"fetched_at"`
}

// checked validates the CMC status block and counts credits. Like Python (`int(x or 0)`, `str(x)`) it
// never fails on the shape of the status block: credit_count may be 1, 1.0 or "1"; error_message any
// JSON value; error_code a string or a number. A missing error_code means success; null or "" does not
// (Python compares str(code) != "0"). It returns the raw "data" value.
func (c *Client) checked(body []byte) (json.RawMessage, error) {
	var env struct {
		Data   json.RawMessage `json:"data"`
		Status json.RawMessage `json:"status"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, &httpx.APIError{Provider: provider, Status: 200, Message: "invalid JSON body: " + httpx.ErrorMessage(body), Err: err}
	}
	var st map[string]json.RawMessage
	if json.Unmarshal(env.Status, &st) != nil || st == nil {
		return env.Data, nil
	}
	c.api.AddCredits(lenientInt(st["credit_count"]))
	rawCode, present := st["error_code"]
	if !present {
		return env.Data, nil
	}
	code := pyStr(rawCode)
	if code == "0" {
		return env.Data, nil
	}
	status, _ := strconv.Atoi(code)
	return nil, &httpx.APIError{Provider: provider, Status: status, Message: pyStr(st["error_message"])}
}

// pyStr renders a JSON value like Python str(): strings unquoted, null as "None", others as raw JSON text.
func pyStr(raw json.RawMessage) string {
	t := strings.TrimSpace(string(raw))
	switch t {
	case "", "null":
		return "None"
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return t
}

// lenientInt converts 1, 1.0, "1" and "1.0" to int64; anything else is 0.
func lenientInt(raw json.RawMessage) int64 {
	t := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if f, err := strconv.ParseFloat(t, 64); err == nil && f == f && f > -1e15 && f < 1e15 {
		return int64(f)
	}
	return 0
}

// stamp is the current UTC time as 1.x writes it (%Y-%m-%dT%H:%M:%SZ).
func (c *Client) stamp() string { return c.now().UTC().Format("2006-01-02T15:04:05Z") }

func (c *Client) get(ctx context.Context, path string, params url.Values) (json.RawMessage, error) {
	_, body, err := c.api.Get(ctx, path, params)
	if err != nil {
		return nil, err
	}
	return c.checked(body)
}

func rwaAssets(data json.RawMessage) ([]json.RawMessage, error) {
	var d struct {
		RWAAssets []json.RawMessage `json:"rwa_assets"`
	}
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("cmc: decode data: %w", err)
	}
	return d.RWAAssets, nil
}

// FetchRWAQuotes fetches all tracked RWA assets per upper-cased symbol in one request.
func (c *Client) FetchRWAQuotes(ctx context.Context, tickers []string) (map[string][]RWAAsset, error) {
	data, err := c.get(ctx, "/v5/real-world-assets/quotes/latest", url.Values{"symbol": {strings.Join(tickers, ",")}})
	if err != nil {
		return nil, fmt.Errorf("cmc quotes: %w", err)
	}
	items, err := rwaAssets(data)
	if err != nil {
		return nil, fmt.Errorf("cmc quotes: %w", err)
	}
	out := map[string][]RWAAsset{}
	for _, raw := range items {
		a := RWAAsset{Raw: slices.Clone(raw)}
		if err := json.Unmarshal(raw, &a.CMCRwa); err != nil {
			return nil, fmt.Errorf("cmc quotes: decode asset: %w", err)
		}
		sym := ""
		if a.Symbol != nil {
			sym = strings.ToUpper(*a.Symbol)
		}
		out[sym] = append(out[sym], a)
	}
	return out, nil
}

// PickRWAAsset chooses the RWA record for asset among the candidates for its ticker. It returns nil and
// a reason (rwa_id_changed, not_in_cmc, ambiguous_cmc_match) when no single record matches.
func PickRWAAsset(candidates []RWAAsset, asset model.Asset, knownRWAID *int) (*RWAAsset, string) {
	var matches []RWAAsset
	for _, c := range candidates {
		if c.AssetType != nil && *c.AssetType == asset.AssetType {
			matches = append(matches, c)
		}
	}
	if knownRWAID != nil {
		var same []RWAAsset
		for _, c := range matches {
			if c.RwaID != nil && *c.RwaID == *knownRWAID {
				same = append(same, c)
			}
		}
		if len(same) == 0 {
			if len(matches) > 0 {
				return nil, ReasonRWAIDChanged
			}
			return nil, ReasonNotInCMC
		}
		matches = same
	}
	switch {
	case len(matches) == 0:
		return nil, ReasonNotInCMC
	case len(matches) > 1:
		return nil, ReasonAmbiguous
	}
	return &matches[0], ""
}

var invalidID = regexp.MustCompile(`Invalid value for 'id': '([\d,]+)'`)

// FetchCryptoInfo fetches metadata (incl. contract_address[]) per CMC id in batches of 100. One unknown
// id fails the whole batch ("Invalid value for 'id': '39839'"): such ids are cut out, retried and
// returned in invalid.
func (c *Client) FetchCryptoInfo(ctx context.Context, ids []int) (info map[int]Info, invalid []int, err error) {
	info = map[int]Info{}
	invalid = []int{}
	unique := slices.Clone(ids)
	slices.Sort(unique)
	unique = slices.Compact(unique)
	for start := 0; start < len(unique); start += InfoBatch {
		chunk := unique[start:min(start+InfoBatch, len(unique))]
		for len(chunk) > 0 {
			strIDs := make([]string, len(chunk))
			for i, id := range chunk {
				strIDs[i] = strconv.Itoa(id)
			}
			data, gerr := c.get(ctx, "/v2/cryptocurrency/info", url.Values{"id": {strings.Join(strIDs, ",")}})
			if gerr != nil {
				bad := badIDs(gerr, chunk)
				if len(bad) == 0 {
					return nil, nil, fmt.Errorf("cmc crypto info: %w", gerr)
				}
				invalid = append(invalid, bad...)
				chunk = slices.DeleteFunc(slices.Clone(chunk), func(id int) bool { return slices.Contains(bad, id) })
				continue
			}
			var entries map[string]json.RawMessage
			if len(data) > 0 && string(data) != "null" {
				if err := json.Unmarshal(data, &entries); err != nil {
					return nil, nil, fmt.Errorf("cmc crypto info: decode data: %w", err)
				}
			}
			for key, raw := range entries {
				id, err := strconv.Atoi(key)
				if err != nil {
					return nil, nil, fmt.Errorf("cmc crypto info: bad id key %q", key)
				}
				e := Info{Raw: slices.Clone(raw)}
				if err := json.Unmarshal(raw, &e.CMCInfo); err != nil {
					return nil, nil, fmt.Errorf("cmc crypto info: decode %d: %w", id, err)
				}
				info[id] = e
			}
			break
		}
	}
	return info, invalid, nil
}

// badIDs extracts the rejected ids named in an "Invalid value for 'id'" error, limited to chunk.
func badIDs(err error, chunk []int) []int {
	var ae *httpx.APIError
	if !errors.As(err, &ae) {
		return nil
	}
	m := invalidID.FindStringSubmatch(ae.Message)
	if m == nil {
		return nil
	}
	var bad []int
	for _, s := range strings.Split(m[1], ",") {
		if id, err := strconv.Atoi(s); err == nil && slices.Contains(chunk, id) {
			bad = append(bad, id)
		}
	}
	slices.Sort(bad)
	return bad
}

// FetchIssuer fetches one issuer with its token roster.
func (c *Client) FetchIssuer(ctx context.Context, issuerID string) (Issuer, error) {
	data, err := c.get(ctx, "/v5/real-world-assets/issuers", url.Values{"issuer_id": {issuerID}})
	if err != nil {
		return Issuer{}, fmt.Errorf("cmc issuer %s: %w", issuerID, err)
	}
	var d struct {
		Name      *string       `json:"name"`
		Website   *string       `json:"website"`
		Logo      *string       `json:"logo"`
		Tokens    []RosterToken `json:"tokens"`
		NumTokens *int          `json:"num_tokens"`
	}
	if len(data) > 0 && string(data) != "null" {
		if err := json.Unmarshal(data, &d); err != nil {
			return Issuer{}, fmt.Errorf("cmc issuer %s: decode: %w", issuerID, err)
		}
	}
	size := len(d.Tokens)
	if d.NumTokens != nil {
		size = *d.NumTokens
	}
	roster := d.Tokens
	if roster == nil {
		roster = []RosterToken{}
	}
	return Issuer{IssuerID: issuerID, Name: d.Name, Website: d.Website, Logo: d.Logo,
		RosterSize: size, Roster: roster, FetchedAt: c.stamp()}, nil
}

// FetchRWAInfo fetches RWA metadata (official website, logo when CMC has one), keyed by rwa_id.
func (c *Client) FetchRWAInfo(ctx context.Context, rwaIDs []int) (map[string]RWAInfo, error) {
	unique := slices.Clone(rwaIDs)
	slices.Sort(unique)
	unique = slices.Compact(unique)
	strIDs := make([]string, len(unique))
	for i, id := range unique {
		strIDs[i] = strconv.Itoa(id)
	}
	data, err := c.get(ctx, "/v5/real-world-assets/info", url.Values{"rwa_id": {strings.Join(strIDs, ",")}})
	if err != nil {
		return nil, fmt.Errorf("cmc rwa info: %w", err)
	}
	items, err := rwaAssets(data)
	if err != nil {
		return nil, fmt.Errorf("cmc rwa info: %w", err)
	}
	now := c.stamp()
	out := map[string]RWAInfo{}
	for _, raw := range items {
		var it struct {
			RWAID    *int    `json:"rwa_id"`
			Name     *string `json:"name"`
			Website  *string `json:"website"`
			Industry *string `json:"industry"`
			About    *struct {
				Website *string `json:"website"`
				Logo    *string `json:"logo"`
			} `json:"about"`
		}
		if err := json.Unmarshal(raw, &it); err != nil {
			return nil, fmt.Errorf("cmc rwa info: decode: %w", err)
		}
		if it.RWAID == nil {
			continue
		}
		website := it.Website
		if empty(website) {
			website = nil
		}
		info := RWAInfo{RWAID: it.RWAID, Name: it.Name, Website: website, Industry: it.Industry, FetchedAt: now}
		if it.About != nil {
			info.Logo = it.About.Logo
			if info.Website == nil && !empty(it.About.Website) {
				info.Website = it.About.Website
			}
		}
		out[strconv.Itoa(*it.RWAID)] = info
	}
	return out, nil
}

func empty(s *string) bool { return s == nil || *s == "" }

// Deployments of a token are derived by compute.ParseDeployments (the parity-tested port of cmc.py::parse_deployments).
