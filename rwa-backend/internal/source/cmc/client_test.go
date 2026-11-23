package cmc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/source/httpx"
)

// fixture returns the recorded response body of testdata/raw/cmc/<name>.json.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../../testdata/raw/cmc/" + name + ".json")
	require.NoError(t, err)
	var rec struct {
		Body json.RawMessage `json:"body"`
	}
	require.NoError(t, json.Unmarshal(b, &rec))
	return rec.Body
}

type recorded struct {
	mu   sync.Mutex
	reqs []*http.Request
}

func (r *recorded) add(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, req)
}

func newClient(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) (*Client, *httpx.Client, *recorded) {
	t.Helper()
	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	api := httpx.New(httpx.Config{Provider: "cmc", BaseURL: srv.URL, Headers: map[string]string{"X-CMC_PRO_API_KEY": "k"},
		Timeout: 5 * time.Second, RequestsPerMinute: 60000, MaxRetries: 1})
	c := New(api)
	c.now = func() time.Time { return time.Date(2026, 10, 18, 20, 0, 0, 0, time.UTC) }
	return c, api, rec
}

func TestFetchRWAQuotes(t *testing.T) {
	c, api, rec := newClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(fixture(t, "quotes_latest")) })
	got, err := c.FetchRWAQuotes(context.Background(), []string{"NVDA", "AAPL", "SPY"})
	require.NoError(t, err)
	assert.Equal(t, "NVDA,AAPL,SPY", rec.reqs[0].URL.Query().Get("symbol"))
	assert.Equal(t, "/v5/real-world-assets/quotes/latest", rec.reqs[0].URL.Path)
	require.Len(t, got["NVDA"], 1)
	nvda := got["NVDA"][0]
	require.NotNil(t, nvda.RwaID)
	assert.Equal(t, 2, *nvda.RwaID)
	assert.Equal(t, "stock", *nvda.AssetType)
	assert.Len(t, nvda.Tokens, 9)
	assert.Equal(t, "etf", *got["SPY"][0].AssetType)
	assert.True(t, nvda.TokenizedMarketCap.V.IsPositive())
	// Raw is the verbatim record: it still carries fields the typed struct drops.
	assert.Contains(t, string(nvda.Raw), `"rwa_rank"`)
	assert.True(t, json.Valid(nvda.Raw))
	assert.EqualValues(t, 1, api.Credits())
	assert.EqualValues(t, 1, api.Requests())
}

func rwa(id int, typ string) RWAAsset {
	return RWAAsset{CMCRwa: model.CMCRwa{RwaID: &id, AssetType: &typ}}
}

func TestPickRWAAsset(t *testing.T) {
	stock := model.Asset{Ticker: "X", AssetType: "stock"}
	known := func(i int) *int { return &i }

	got, reason := PickRWAAsset([]RWAAsset{rwa(1, "etf"), rwa(2, "stock")}, stock, nil)
	require.NotNil(t, got)
	assert.Empty(t, reason)
	assert.Equal(t, 2, *got.RwaID)

	got, reason = PickRWAAsset([]RWAAsset{rwa(2, "stock")}, stock, known(2))
	require.NotNil(t, got)
	assert.Empty(t, reason)

	got, reason = PickRWAAsset([]RWAAsset{rwa(5, "stock")}, stock, known(2))
	assert.Nil(t, got)
	assert.Equal(t, "rwa_id_changed", reason)

	got, reason = PickRWAAsset([]RWAAsset{rwa(1, "etf")}, stock, known(2))
	assert.Nil(t, got)
	assert.Equal(t, "not_in_cmc", reason)

	got, reason = PickRWAAsset(nil, stock, nil)
	assert.Nil(t, got)
	assert.Equal(t, "not_in_cmc", reason)

	got, reason = PickRWAAsset([]RWAAsset{rwa(1, "stock"), rwa(2, "stock")}, stock, nil)
	assert.Nil(t, got)
	assert.Equal(t, "ambiguous_cmc_match", reason)
}

func TestFetchCryptoInfoInvalidIDThenSuccess(t *testing.T) {
	c, api, rec := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(","+r.URL.Query().Get("id")+",", ",39002,") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(fixture(t, "crypto_info_invalid_id"))
			return
		}
		_, _ = w.Write(fixture(t, "crypto_info"))
	})
	info, invalid, err := c.FetchCryptoInfo(context.Background(), []int{39002, 36992, 37120, 37191, 28616, 36992})
	require.NoError(t, err)
	assert.Equal(t, []int{39002}, invalid)
	require.Len(t, info, 4)
	assert.Equal(t, "NVDAX", *info[36992].Symbol)
	assert.Contains(t, string(info[36992].Raw), `"contract_address"`)
	assert.EqualValues(t, 2, api.Requests())
	assert.Equal(t, "28616,36992,37120,37191", rec.reqs[1].URL.Query().Get("id"))
	assert.EqualValues(t, 1, api.Credits()) // the 400 reports no credits
}

func TestFetchCryptoInfoBatches(t *testing.T) {
	var sizes []int
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		sizes = append(sizes, len(strings.Split(r.URL.Query().Get("id"), ",")))
		_, _ = w.Write([]byte(`{"data":{},"status":{"error_code":0,"error_message":null,"credit_count":1}}`))
	})
	ids := make([]int, 250)
	for i := range ids {
		ids[i] = 1000 + i
	}
	_, _, err := c.FetchCryptoInfo(context.Background(), ids)
	require.NoError(t, err)
	assert.Equal(t, []int{100, 100, 50}, sizes)
}

func TestFetchCryptoInfoOtherErrorPropagates(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":{"error_code":400,"error_message":"Invalid value for 'id': '777'"}}`))
	})
	// 777 is not in the requested chunk: it must not loop forever.
	_, _, err := c.FetchCryptoInfo(context.Background(), []int{1, 2})
	var ae *httpx.APIError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, 400, ae.Status)
}

func TestFetchIssuer(t *testing.T) {
	c, api, rec := newClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(fixture(t, "issuer_small")) })
	is, err := c.FetchIssuer(context.Background(), "6a0be4e680ee5978ec4455ce")
	require.NoError(t, err)
	assert.Equal(t, "6a0be4e680ee5978ec4455ce", rec.reqs[0].URL.Query().Get("issuer_id"))
	assert.Equal(t, "Tessera Assets", *is.Name)
	assert.Equal(t, 1, is.RosterSize)
	require.Len(t, is.Roster, 1)
	assert.Equal(t, "tSpaceX", *is.Roster[0].Symbol)
	assert.EqualValues(t, 40018, *is.Roster[0].CryptoID)
	assert.Equal(t, 9, *is.Roster[0].RWAID)
	assert.Equal(t, "2026-10-18T20:00:00Z", is.FetchedAt)
	assert.EqualValues(t, 1, api.Credits())
}

func TestFetchIssuerRosterSizeFallback(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"name":"I","tokens":[{"crypto_id":1,"symbol":"A","rwa_id":2},{"crypto_id":3,"symbol":"B","rwa_id":4}]},"status":{"error_code":"0"}}`))
	})
	is, err := c.FetchIssuer(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, 2, is.RosterSize)
}

func TestFetchRWAInfo(t *testing.T) {
	c, _, rec := newClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(fixture(t, "rwa_info")) })
	got, err := c.FetchRWAInfo(context.Background(), []int{86, 2, 3, 2})
	require.NoError(t, err)
	assert.Equal(t, "2,3,86", rec.reqs[0].URL.Query().Get("rwa_id"))
	require.Contains(t, got, "2")
	assert.Equal(t, "https://www.nvidia.com", *got["2"].Website)
	assert.Equal(t, "Semiconductors & Related Devices", *got["2"].Industry)
	assert.Equal(t, 2, *got["2"].RWAID)
	assert.Nil(t, got["86"].Website)
	assert.Len(t, got, 3)
}

func TestFetchRWAInfoWebsiteFromAbout(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"rwa_assets":[{"rwa_id":9,"name":"N","website":"","industry":"I","about":{"website":"https://a.example","logo":"https://l.example/x.png"}}]},"status":{"error_code":"0"}}`))
	})
	got, err := c.FetchRWAInfo(context.Background(), []int{9})
	require.NoError(t, err)
	assert.Equal(t, "https://a.example", *got["9"].Website)
	assert.Equal(t, "https://l.example/x.png", *got["9"].Logo)
}

func TestErrorInsideHTTP200(t *testing.T) {
	for _, code := range []string{`"1010"`, `1010`} {
		c, api, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintf(w, `{"status":{"error_code":%s,"error_message":"plan limit","credit_count":0}}`, code)
		})
		_, err := c.FetchRWAQuotes(context.Background(), []string{"NVDA"})
		var ae *httpx.APIError
		require.ErrorAs(t, err, &ae, code)
		assert.Equal(t, 1010, ae.Status)
		assert.Equal(t, "plan limit", ae.Message)
		assert.Equal(t, "cmc", ae.Provider)
		assert.EqualValues(t, 1, api.Requests())
	}
}

func TestAuthError(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":{"error_code":1002,"error_message":"API key missing."}}`))
	})
	_, err := c.FetchRWAQuotes(context.Background(), []string{"NVDA"})
	require.ErrorIs(t, err, httpx.ErrAuth)
}

func TestStatusBlockLenient(t *testing.T) {
	cases := []struct {
		name, status string
		credits      int64
		wantErr      string // "" = success
		wantStatus   int
	}{
		{"float credit", `{"error_code":"0","credit_count":1.0}`, 1, "", 0},
		{"string credit", `{"error_code":0,"credit_count":"1"}`, 1, "", 0},
		{"null credit", `{"error_code":0,"credit_count":null}`, 0, "", 0},
		{"missing error_code", `{"credit_count":2}`, 2, "", 0},
		{"null error_code", `{"error_code":null,"error_message":"odd"}`, 0, "odd", 0},
		{"empty error_code", `{"error_code":"","error_message":"odd"}`, 0, "odd", 0},
		{"numeric message", `{"error_code":5,"error_message":123}`, 0, "123", 5},
		{"object message", `{"error_code":"7","error_message":{"a":1}}`, 0, `{"a":1}`, 7},
		{"null message", `{"error_code":"7","error_message":null}`, 0, "None", 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, api, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprintf(w, `{"data":{"rwa_assets":[]},"status":%s}`, tc.status)
			})
			got, err := c.FetchRWAQuotes(context.Background(), []string{"NVDA"})
			assert.EqualValues(t, tc.credits, api.Credits())
			if tc.wantErr == "" {
				require.NoError(t, err)
				assert.NotNil(t, got)
				return
			}
			var ae *httpx.APIError
			require.ErrorAs(t, err, &ae)
			assert.Equal(t, tc.wantErr, ae.Message)
			assert.Equal(t, tc.wantStatus, ae.Status)
		})
	}
}

// The envelope is strict like the pipeline's _checked: status must be an object and data must be present.
func TestEnvelopeStrict(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"status not an object", `{"data":{},"status":"weird"}`, "invalid response: expected an object with status object"},
		{"status null", `{"data":{},"status":null}`, "invalid response: expected an object with status object"},
		{"no status", `{"data":{}}`, "invalid response: expected an object with status object"},
		{"body is a list", `[1]`, "invalid response: expected an object with status object"},
		{"no data key", `{"status":{"error_code":"0"}}`, "invalid response: expected a data key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) })
			_, err := c.FetchRWAQuotes(context.Background(), []string{"NVDA"})
			var ae *httpx.APIError
			require.ErrorAs(t, err, &ae)
			assert.Equal(t, tc.want, ae.Message)
			assert.Zero(t, ae.Status)
		})
	}
	// An error code wins over a missing data key; a null data value is present.
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":{"error_code":1010,"error_message":"plan limit"}}`))
	})
	_, err := c.FetchRWAQuotes(context.Background(), []string{"NVDA"})
	var ae *httpx.APIError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, "plan limit", ae.Message)
	c, _, _ = newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":null,"status":{"error_code":"0"}}`))
	})
	_, err = c.FetchRWAQuotes(context.Background(), []string{"NVDA"})
	require.NoError(t, err)
}

func TestRWAInfoEmptyWebsiteIsNil(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"rwa_assets":[{"rwa_id":9,"website":"","about":{"website":""}}]},"status":{"error_code":"0"}}`))
	})
	got, err := c.FetchRWAInfo(context.Background(), []int{9})
	require.NoError(t, err)
	assert.Nil(t, got["9"].Website)
	assert.Equal(t, "2026-10-18T20:00:00Z", got["9"].FetchedAt)
}

func TestEmptyResultsAreNotNil(t *testing.T) {
	c, _, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{},"status":{"error_code":"0"}}`))
	})
	info, invalid, err := c.FetchCryptoInfo(context.Background(), nil)
	require.NoError(t, err)
	assert.NotNil(t, info)
	assert.NotNil(t, invalid)
	is, err := c.FetchIssuer(context.Background(), "x")
	require.NoError(t, err)
	assert.NotNil(t, is.Roster)
	q, err := c.FetchRWAQuotes(context.Background(), []string{"A"})
	require.NoError(t, err)
	assert.NotNil(t, q)
	ri, err := c.FetchRWAInfo(context.Background(), []int{1})
	require.NoError(t, err)
	assert.NotNil(t, ri)
}
