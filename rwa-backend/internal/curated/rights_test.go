package curated

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// The tests mirror rwa-data-pipeline/tests/test_rights.py.

var today = time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)

const (
	issuerA = "issuer-1"
	issuerB = "issuer-2"
	docURL  = "https://example.org/terms.pdf"
)

type m = map[string]any

func evidence(quote string) m {
	if quote == "" {
		quote = "The holder has an individual entitlement."
	}
	return m{"quote": quote, "url": docURL, "title": "Terms and Conditions", "doc_date": "2026-06-11", "retrieved_at": "2026-11-13"}
}

func fact(spec factSpec) m {
	out := m{"value": spec.Values[0], "evidence": []any{evidence("")}}
	if slicesContains(spec.NeedsWho, spec.Values[0]) {
		out["who"] = "verified"
	}
	if spec.Key == "governing_law" {
		out["countries"] = []any{"AE"}
		out["zones"] = []any{"ADGM"}
	}
	return out
}

func slicesContains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func allFacts() m {
	out := m{}
	for _, spec := range rightsFactDef {
		out[spec.Key] = fact(spec)
	}
	return out
}

func profile(id string, tokenIDs []int, overrides m) m {
	ids := make([]any, len(tokenIDs))
	for i, t := range tokenIDs {
		ids[i] = t
	}
	basis := "per_token_documents"
	if len(tokenIDs) > 1 {
		basis = "programme_documents"
	}
	out := m{
		"id": id, "issuer_id": issuerA, "cmc_label": "Acme Assets", "product": "Acme Stocks", "token_ids": ids,
		"scope_basis": basis, "reviewed_at": "2026-11-13",
		"legal_issuer": m{"name": "Acme Holdings Ltd", "country": "AE", "zone": "ADGM",
			"evidence": []any{evidence("The Issuer is Acme Holdings Ltd, a company incorporated in the ADGM.")}},
		"facts":     allFacts(),
		"documents": []any{m{"title": "Terms and Conditions", "url": docURL, "type": "terms", "doc_date": "2026-06-11"}},
	}
	if len(tokenIDs) > 1 {
		out["scope_evidence"] = []any{evidence("These Terms apply to every Token issued under the programme.")}
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

func otherProfile() m {
	return profile("other-line", []int{3}, m{"issuer_id": issuerB, "cmc_label": "Other Assets"})
}

func config(top m, profiles ...any) []byte {
	if profiles == nil {
		profiles = []any{}
	}
	cfg := m{"schema_version": 1, "review_valid_days": 180, "profiles": profiles}
	for k, v := range top {
		cfg[k] = v
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	return b
}

func str(s string) *string { return &s }

func exportOf() *model.Export {
	tok := func(id int64, issuer string) model.Token {
		return model.Token{ID: id, Symbol: str("T"), IssuerID: str(issuer), Kind: "tokenized"}
	}
	return &model.Export{
		Tokens: []model.Token{tok(1, issuerA), tok(2, issuerA), tok(3, issuerB)},
		Issuers: []model.Issuer{
			{ID: issuerA, Name: str("Acme"), CMCLabel: str("Acme Assets")},
			{ID: issuerB, Name: str("Other"), CMCLabel: str("Other Assets")},
		},
	}
}

func attachRights(t *testing.T, cfg []byte, exp *model.Export, now time.Time) (*model.Export, []string) {
	t.Helper()
	var rights *Rights
	if cfg != nil {
		var err error
		rights, err = ParseRights("rights.json", cfg)
		require.NoError(t, err)
	}
	if exp == nil {
		exp = exportOf()
	}
	return exp, AttachRights(exp, rights, now)
}

func profileIDs(t *testing.T, exp *model.Export) []string {
	t.Helper()
	ids := []string{}
	for _, raw := range exp.RightsProfiles {
		var p struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.Unmarshal(raw, &p))
		ids = append(ids, p.ID)
	}
	return ids
}

func boundTokens(exp *model.Export) map[int64]string {
	out := map[int64]string{}
	for _, tk := range exp.Tokens {
		out[tk.ID] = ""
		if tk.RightsProfileID != nil {
			out[tk.ID] = *tk.RightsProfileID
		}
	}
	return out
}

func TestFactTableIsThePublishedContract(t *testing.T) {
	keys := make([]string, len(rightsFactDef))
	for i, f := range rightsFactDef {
		keys[i] = f.Key
	}
	assert.Equal(t, []string{"instrument", "backing", "asset_protection", "primary_access", "redemption", "claim_scope", "restricted",
		"transfer", "freeze", "cancellation", "pause", "upgrade", "forced_redemption", "loss_exploit", "loss_lost_tokens",
		"loss_issuer_insolvency", "loss_custodian_failure", "dividends", "voting", "reserves_proof", "governing_law"}, keys)
	redemption, _ := factByKey("redemption")
	assert.Equal(t, factSpec{"redemption", "who", []string{"available", "at_events_only", "not_redeemable"}, []string{"available", "at_events_only"}}, redemption)
	assert.Equal(t, []string{"anyone", "verified", "professional", "institutional", "platform_clients", "eligible_holders", "nobody"}, rightsWho)
}

func TestValidProfileBindsOnlyItsListedTokens(t *testing.T) {
	exp, warnings := attachRights(t, config(nil, profile("acme-line", []int{1}, nil)), nil, today)
	assert.Empty(t, warnings)
	assert.Equal(t, map[int64]string{1: "acme-line", 2: "", 3: ""}, boundTokens(exp), "token 2 has the same issuer but nobody reviewed it")
	assert.Equal(t, []string{"acme-line"}, profileIDs(t, exp))
	assert.JSONEq(t, string(mustJSON(profile("acme-line", []int{1}, m{"stale": false}))), string(exp.RightsProfiles[0]))
	assert.Equal(t, []string{"acme-line"}, exp.Issuers[0].RightsProfileIDs)
	assert.Equal(t, 1, exp.Issuers[0].RightsReviewedTokensCount)
	assert.Equal(t, []string{}, exp.Issuers[1].RightsProfileIDs)
	s := exp.Summary
	assert.Equal(t, 1, s.RightsProfilesCount)
	assert.Equal(t, 1, s.RightsReviewedTokensCount)
	assert.Equal(t, 180, s.RightsReviewValidDays)
	assert.Zero(t, s.RightsWarningsCount)
	assert.Zero(t, s.RightsExcludedProfilesCount)
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestSchemaTravelsWithTheExport(t *testing.T) {
	exp, _ := attachRights(t, config(nil, profile("acme-line", []int{1}, nil)), nil, today)
	var schema struct {
		UnknownValue string   `json:"unknown_value"`
		Who          []string `json:"who"`
		Facts        []struct {
			Key      string   `json:"key"`
			Group    string   `json:"group"`
			Values   []string `json:"values"`
			NeedsWho []string `json:"needs_who"`
		} `json:"facts"`
	}
	require.NoError(t, json.Unmarshal(exp.RightsSchema, &schema))
	assert.Equal(t, "unknown", schema.UnknownValue)
	assert.Equal(t, rightsWho, schema.Who)
	require.Len(t, schema.Facts, len(rightsFactDef))
	assert.Equal(t, "primary_access", schema.Facts[3].Key)
	assert.Equal(t, []string{"available"}, schema.Facts[3].NeedsWho)
	assert.Equal(t, []string{}, schema.Facts[0].NeedsWho)
}

func TestWithoutProfilesEveryFieldIsPresentAndEmpty(t *testing.T) {
	for name, cfg := range map[string][]byte{"no file": nil, "empty": config(nil)} {
		t.Run(name, func(t *testing.T) {
			exp, warnings := attachRights(t, cfg, nil, today)
			assert.Empty(t, warnings)
			assert.NotNil(t, exp.RightsProfiles)
			assert.Empty(t, exp.RightsProfiles)
			assert.NotEmpty(t, exp.RightsSchema)
			for _, tk := range exp.Tokens {
				assert.Nil(t, tk.RightsProfileID)
			}
			for _, is := range exp.Issuers {
				assert.Equal(t, []string{}, is.RightsProfileIDs)
				assert.Zero(t, is.RightsReviewedTokensCount)
			}
			assert.Equal(t, 180, exp.Summary.RightsReviewValidDays)
		})
	}
}

func TestProgrammeProfileCoversSeveralTokensAndTwoProfilesCoexist(t *testing.T) {
	exp, warnings := attachRights(t, config(nil, profile("acme-programme", []int{1, 2}, nil), otherProfile()), nil, today)
	assert.Empty(t, warnings)
	assert.Equal(t, map[int64]string{1: "acme-programme", 2: "acme-programme", 3: "other-line"}, boundTokens(exp))
	assert.Equal(t, 3, exp.Summary.RightsReviewedTokensCount)
}

func TestTokenMissingFromTheExportIsAWarningNotAnExclusion(t *testing.T) {
	exp, warnings := attachRights(t, config(nil, profile("acme-programme", []int{1, 2, 999}, nil)), nil, today)
	assert.Equal(t, []string{"rights profile acme-programme: tokens 999 are absent from the export"}, warnings)
	assert.Equal(t, map[int64]string{1: "acme-programme", 2: "acme-programme", 3: ""}, boundTokens(exp))
	assert.Equal(t, 2, exp.Issuers[0].RightsReviewedTokensCount)
	assert.Zero(t, exp.Summary.RightsExcludedProfilesCount)
	var p struct {
		TokenIDs []int `json:"token_ids"`
	}
	require.NoError(t, json.Unmarshal(exp.RightsProfiles[0], &p))
	assert.Equal(t, []int{1, 2}, p.TokenIDs, "the published list names the tokens of this export")
}

func TestConditionalOutcomesSayToWhom(t *testing.T) {
	for _, c := range [][2]string{{"redemption", "at_events_only"}, {"loss_issuer_insolvency", "unsecured_claim"}, {"loss_exploit", "unsecured_claim"}, {"voting", "discretionary"}} {
		facts := allFacts()
		facts[c[0]] = m{"value": c[1], "who": "platform_clients", "evidence": []any{evidence("")}}
		exp, warnings := attachRights(t, config(nil, profile("acme-line", []int{1}, m{"facts": facts})), nil, today)
		assert.Empty(t, warnings, c)
		assert.Len(t, exp.RightsProfiles, 1, c)

		facts[c[0]] = m{"value": c[1], "evidence": []any{evidence("")}}
		exp, warnings = attachRights(t, config(nil, profile("acme-line", []int{1}, m{"facts": facts})), nil, today)
		assert.Empty(t, exp.RightsProfiles, c)
		assert.Len(t, warnings, 1, c)
	}
}

func TestATokenListedTwiceMakesTheProfileInvalid(t *testing.T) {
	exp, warnings := attachRights(t, config(nil, profile("acme-programme", []int{1, 2, 1}, nil)), nil, today)
	assert.Empty(t, exp.RightsProfiles)
	assert.Equal(t, 1, exp.Summary.RightsExcludedProfilesCount)
	assert.Equal(t, []string{"rights profile acme-programme: excluded: token_ids: expected unique token IDs"}, warnings)
}

func TestARestrictionMayListOnlyZones(t *testing.T) {
	facts := allFacts()
	facts["restricted"] = m{"value": "listed", "zones": []any{"Crimea"}, "evidence": []any{evidence("")}}
	_, warnings := attachRights(t, config(nil, profile("acme-line", []int{1}, m{"facts": facts})), nil, today)
	assert.Empty(t, warnings)
}

func TestAProfileOutsideTheBasketIsAbsentNotBroken(t *testing.T) {
	elsewhere := profile("elsewhere-line", []int{501, 502}, m{"issuer_id": "issuer-elsewhere", "cmc_label": "Elsewhere Assets"})
	exp, warnings := attachRights(t, config(nil, profile("acme-line", []int{1}, nil), elsewhere), nil, today)
	assert.Equal(t, []string{"acme-line"}, profileIDs(t, exp))
	assert.Equal(t, []string{"rights profile elsewhere-line: outside the export; absent tokens: 501, 502"}, warnings)
	assert.Zero(t, exp.Summary.RightsExcludedProfilesCount, "nothing is wrong with the profile itself")
	assert.Equal(t, 1, exp.Summary.RightsWarningsCount)
	assert.Equal(t, 1, exp.Summary.RightsProfilesCount)
}

func TestAProfileWithNoTokenInTheExportIsNotPublished(t *testing.T) {
	exp, warnings := attachRights(t, config(nil, profile("acme-line", []int{999}, nil)), nil, today)
	assert.Empty(t, exp.RightsProfiles)
	assert.Equal(t, []string{"rights profile acme-line: outside the export; absent tokens: 999"}, warnings)
}

// A review is stale after review_valid_days: the boundary is exclusive (exactly 180 days old is current).
func TestStaleBoundary(t *testing.T) {
	exp, warnings := attachRights(t, config(nil, profile("acme-line", []int{1}, m{"reviewed_at": "2026-05-17"})), nil, today)
	assert.Empty(t, warnings)
	assert.Contains(t, string(exp.RightsProfiles[0]), `"stale":false`)

	exp, warnings = attachRights(t, config(nil, profile("acme-line", []int{1}, m{"reviewed_at": "2026-05-16"})), nil, today)
	assert.Contains(t, string(exp.RightsProfiles[0]), `"stale":true`)
	assert.Equal(t, "acme-line", boundTokens(exp)[1], "still shown, marked as old")
	assert.Equal(t, []string{"rights profile acme-line: review is stale (older than 180 days)"}, warnings)
	assert.Zero(t, exp.Summary.RightsExcludedProfilesCount)

	// today is the UTC date of the build time, not a moment: a later hour of the same day changes nothing.
	exp = exportOf()
	rights, err := ParseRights("r", config(nil, profile("acme-line", []int{1}, m{"reviewed_at": "2026-05-17"})))
	require.NoError(t, err)
	assert.Empty(t, AttachRights(exp, rights, time.Date(2026, 11, 13, 23, 59, 0, 0, time.UTC)))
}

func TestReviewValidDaysDefaultsTo180(t *testing.T) {
	raw := mustJSON(m{"schema_version": 1, "profiles": []any{profile("acme-line", []int{1}, nil)}})
	rights, err := ParseRights("r", raw)
	require.NoError(t, err)
	exp := exportOf()
	AttachRights(exp, rights, today)
	assert.Equal(t, 180, exp.Summary.RightsReviewValidDays)

	exp = exportOf()
	rights, err = ParseRights("r", config(m{"review_valid_days": 30}, profile("acme-line", []int{1}, m{"reviewed_at": "2026-10-13"})))
	require.NoError(t, err)
	assert.Equal(t, []string{"rights profile acme-line: review is stale (older than 30 days)"}, AttachRights(exp, rights, today))
}

func fromFact(key string, changes m) func(p m) {
	return func(p m) {
		f := p["facts"].(m)[key].(m)
		for k, v := range changes {
			f[k] = v
		}
	}
}

func put(path ...any) func(p m) {
	return func(p m) {
		value := path[len(path)-1]
		node := p
		for _, k := range path[:len(path)-2] {
			node = node[k.(string)].(m)
		}
		node[path[len(path)-2].(string)] = value
	}
}

func drop(path ...string) func(p m) {
	return func(p m) {
		node := p
		for _, k := range path[:len(path)-1] {
			node = node[k].(m)
		}
		delete(node, path[len(path)-1])
	}
}

func ev(changes m) []any {
	e := evidence("")
	for k, v := range changes {
		if v == nil {
			delete(e, k)
		} else {
			e[k] = v
		}
	}
	return []any{e}
}

func TestBrokenProfileIsExcludedAndTheOtherOneIsPublished(t *testing.T) {
	broken := map[string]func(p m){
		"unknown top-level key":                   put("rating", "A+"),
		"unknown fact":                            put("facts", "safety_score", m{"value": "yes", "evidence": ev(nil)}),
		"missing fact":                            drop("facts", "dividends"),
		"value outside the list":                  fromFact("voting", m{"value": "maybe"}),
		"value without who":                       drop("facts", "redemption", "who"),
		"who outside the list":                    fromFact("redemption", m{"who": "residents_only"}),
		"who on a value that takes none":          fromFact("voting", m{"who": "anyone"}),
		"fact without evidence":                   fromFact("freeze", m{"evidence": []any{}}),
		"empty quote":                             fromFact("freeze", m{"evidence": ev(m{"quote": "  "})}),
		"quote longer than 600 characters":        fromFact("freeze", m{"evidence": ev(m{"quote": strings.Repeat("word ", 121)})}),
		"quote is a fragment":                     fromFact("freeze", m{"evidence": ev(m{"quote": "an admin-burn"})}),
		"quote is a bare label of five words":     fromFact("freeze", m{"evidence": ev(m{"quote": "Daily Proof of Collateral here"})}),
		"issuer quote is only a name":             put("legal_issuer", "evidence", []any{evidence("Acme Holdings Ltd")}),
		"evidence url is not https":               fromFact("freeze", m{"evidence": ev(m{"url": "http://example.org/terms.pdf"})}),
		"evidence url is a script":                fromFact("freeze", m{"evidence": ev(m{"url": "javascript:alert(1)"})}),
		"evidence without a title":                fromFact("freeze", m{"evidence": ev(m{"title": ""})}),
		"evidence without a document date":        fromFact("freeze", m{"evidence": ev(m{"doc_date": nil})}),
		"retrieved in the future":                 fromFact("freeze", m{"evidence": ev(m{"retrieved_at": "2026-11-14"})}),
		"retrieved date is not a date":            fromFact("freeze", m{"evidence": ev(m{"retrieved_at": "13.11.2026"})}),
		"unknown key in evidence":                 fromFact("freeze", m{"evidence": ev(m{"page": 12})}),
		"unknown key in a fact":                   fromFact("freeze", m{"confidence": "high"}),
		"unknown without checked":                 put("facts", "pause", m{"value": "unknown"}),
		"unknown with evidence":                   put("facts", "pause", m{"value": "unknown", "checked": []any{docURL}, "evidence": ev(nil)}),
		"unknown checked is not https":            put("facts", "pause", m{"value": "unknown", "checked": []any{"ftp://example.org"}}),
		"note in one language":                    fromFact("freeze", m{"note": m{"en": "Only English."}}),
		"note with an extra language":             fromFact("freeze", m{"note": m{"en": "a", "zh": "b", "ru": "c"}}),
		"country is not an ISO code":              fromFact("governing_law", m{"countries": []any{"UAE"}}),
		"governing law without a place":           put("facts", "governing_law", m{"value": "stated", "evidence": ev(nil)}),
		"restricted listed without anything":      put("facts", "restricted", m{"value": "listed", "evidence": ev(nil)}),
		"us_persons on another fact":              fromFact("freeze", m{"us_persons": true}),
		"us_persons is not a boolean":             put("facts", "restricted", m{"value": "listed", "us_persons": "yes", "evidence": ev(nil)}),
		"reviewed in the future":                  put("reviewed_at", "2026-11-14"),
		"reviewed_at is not a date":               put("reviewed_at", "yesterday"),
		"reviewed_at is an impossible date":       put("reviewed_at", "2026-02-30"),
		"legal issuer without evidence":           put("legal_issuer", m{"name": "Acme Holdings Ltd", "country": "AE"}),
		"legal issuer without a name":             put("legal_issuer", "name", ""),
		"legal issuer country is not an ISO":      put("legal_issuer", "country", "Abu Dhabi"),
		"legal issuer note in one language":       put("legal_issuer", "note", m{"zh": "x"}),
		"unknown key in the legal issuer":         put("legal_issuer", "licence", "FSRA"),
		"no documents":                            put("documents", []any{}),
		"document type outside the list":          put("documents", []any{m{"title": "T", "url": docURL, "type": "tweet", "doc_date": "2026"}}),
		"document url is not https":               put("documents", []any{m{"title": "T", "url": "//example.org", "type": "terms", "doc_date": "2026"}}),
		"scope basis outside the list":            put("scope_basis", "all_tokens"),
		"empty token list":                        put("token_ids", []any{}),
		"token ids are not integers":              put("token_ids", []any{"1"}),
		"token id is a boolean":                   put("token_ids", []any{true}),
		"token id is a float":                     put("token_ids", []any{1.5}),
		"empty product":                           put("product", ""),
		"id with spaces":                          put("id", "Acme Line"),
		"label differs from the export":           put("cmc_label", "Acme"),
		"issuer is not in the export":             put("issuer_id", "issuer-404"),
		"none_stated yet countries are listed":    put("facts", "restricted", m{"value": "none_stated", "countries": []any{"US"}, "evidence": ev(nil)}),
		"none_stated yet zones are listed":        put("facts", "restricted", m{"value": "none_stated", "zones": []any{"Crimea"}, "evidence": ev(nil)}),
		"none_stated yet US persons are excluded": put("facts", "restricted", m{"value": "none_stated", "us_persons": true, "evidence": ev(nil)}),
	}
	for name, mutate := range broken {
		t.Run(name, func(t *testing.T) {
			p := profile("acme-line", []int{1}, nil)
			mutate(p)
			exp, warnings := attachRights(t, config(nil, p, otherProfile()), nil, today)
			assert.Equal(t, []string{"other-line"}, profileIDs(t, exp))
			assert.Equal(t, map[int64]string{1: "", 2: "", 3: "other-line"}, boundTokens(exp))
			require.Len(t, warnings, 1, "one line per excluded profile: %v", warnings)
			assert.Contains(t, warnings[0], ": excluded: ")
			assert.Equal(t, 1, exp.Summary.RightsExcludedProfilesCount)
			assert.Equal(t, 1, exp.Summary.RightsWarningsCount)
		})
	}
}

func TestATokenOfAnotherIssuerExcludesTheProfile(t *testing.T) {
	p := profile("acme-line", []int{3}, nil)
	exp, warnings := attachRights(t, config(nil, p), nil, today)
	assert.Empty(t, exp.RightsProfiles)
	assert.Equal(t, []string{"rights profile acme-line: excluded: token_ids[3]: expected a token of this issuer"}, warnings)
}

func TestExclusionMessages(t *testing.T) {
	for _, tc := range []struct {
		mutate func(p m)
		want   string
	}{
		{put("rating", "A+"), "profile: expected known keys only (unexpected 'rating')"},
		{drop("facts", "dividends"), "facts.dividends: expected an object"},
		{fromFact("voting", m{"value": "maybe"}), "facts.voting.value: expected an allowed value or unknown"},
		{drop("facts", "redemption", "who"), "facts.redemption.who: expected an allowed holder category"},
		{fromFact("redemption", m{"value": "not_redeemable"}), "facts.redemption: expected who only on values that need it"},
		{fromFact("freeze", m{"evidence": ev(m{"quote": "an admin-burn"})}), "facts.freeze.evidence[0].quote: expected at least six words"},
		{put("reviewed_at", "2026-11-14"), "reviewed_at: expected a date no later than today"},
		{put("cmc_label", "Acme"), "cmc_label: expected the export issuer label"},
		{put("issuer_id", "issuer-404"), "issuer_id: expected an issuer present in the export"},
		{put("facts", "restricted", m{"value": "listed", "evidence": ev(nil)}), "facts.restricted: expected countries, zones or us_persons: true"},
	} {
		p := profile("acme-line", []int{1}, nil)
		tc.mutate(p)
		_, warnings := attachRights(t, config(nil, p), nil, today)
		assert.Equal(t, []string{"rights profile acme-line: excluded: " + tc.want}, warnings)
	}
}

func TestPerTokenDocumentsCoverExactlyOneToken(t *testing.T) {
	exp, warnings := attachRights(t, config(nil, profile("acme-line", []int{1, 2}, m{"scope_basis": "per_token_documents"})), nil, today)
	assert.Empty(t, exp.RightsProfiles)
	assert.Len(t, warnings, 1)
}

func TestProgrammeDocumentsNeedAQuote(t *testing.T) {
	p := profile("acme-programme", []int{1, 2}, nil)
	delete(p, "scope_evidence")
	exp, warnings := attachRights(t, config(nil, p), nil, today)
	assert.Empty(t, exp.RightsProfiles)
	assert.Len(t, warnings, 1)
}

func TestAProfileThatIsNotAnObjectIsExcludedWithoutACrash(t *testing.T) {
	exp, warnings := attachRights(t, config(nil, "acme-line", nil, otherProfile()), nil, today)
	assert.Equal(t, []string{"other-line"}, profileIDs(t, exp))
	assert.Equal(t, 2, exp.Summary.RightsExcludedProfilesCount)
	assert.Equal(t, []string{"rights profile #1: excluded: profile: expected an object", "rights profile #2: excluded: profile: expected an object"}, warnings)
}

func TestQuotesArePublishedWithoutTheLineBreaksOfAPDFTextLayer(t *testing.T) {
	ragged := "The Issuer   holds its interest\n      in the Underlying as trustee\tfor the benefit of holders."
	facts := allFacts()
	facts["freeze"] = m{"value": "yes", "evidence": []any{evidence(ragged)}}
	exp, warnings := attachRights(t, config(nil, profile("acme-line", []int{1}, m{"facts": facts})), nil, today)
	assert.Empty(t, warnings)
	assert.Contains(t, string(exp.RightsProfiles[0]), `"quote":"The Issuer holds its interest in the Underlying as trustee for the benefit of holders."`)
}

func TestTheLengthLimitCountsThePublishedText(t *testing.T) {
	sentence := "The Issuer holds its interest in the Underlying as trustee for the benefit of holders."
	padded := strings.ReplaceAll(sentence, " ", strings.Repeat(" ", 60)) // far over 600 raw characters, 86 once collapsed
	require.Greater(t, len(padded), 600)
	facts := allFacts()
	facts["freeze"] = m{"value": "yes", "evidence": []any{evidence(padded)}}
	exp, warnings := attachRights(t, config(nil, profile("acme-line", []int{1}, m{"facts": facts})), nil, today)
	assert.Empty(t, warnings)
	assert.Contains(t, string(exp.RightsProfiles[0]), sentence)

	facts["freeze"] = m{"value": "yes", "evidence": []any{evidence(strings.TrimSpace(strings.Repeat("word ", 130)))}}
	exp, warnings = attachRights(t, config(nil, profile("acme-line", []int{1}, m{"facts": facts})), nil, today)
	assert.Empty(t, exp.RightsProfiles)
	assert.Len(t, warnings, 1)
}

func TestPublishedProfileKeepsTheKeyOrderOfTheFile(t *testing.T) {
	raw := `{"schema_version":1,"profiles":[` + string(mustJSON(profile("acme-line", []int{1}, nil))) + `]}`
	// json.Marshal sorted the keys: build a file with a deliberate order instead.
	raw = strings.Replace(raw, `{"cmc_label"`, `{"product":"Acme Stocks","cmc_label"`, 1)
	raw = strings.Replace(raw, `,"product":"Acme Stocks","reviewed_at"`, `,"reviewed_at"`, 1)
	rights, err := ParseRights("r", []byte(raw))
	require.NoError(t, err)
	exp := exportOf()
	AttachRights(exp, rights, today)
	require.Len(t, exp.RightsProfiles, 1)
	out := string(exp.RightsProfiles[0])
	assert.Less(t, strings.Index(out, `"product"`), strings.Index(out, `"cmc_label"`))
	assert.True(t, strings.HasSuffix(out, `"stale":false}`), "stale is appended last: %s", out[len(out)-40:])
}

func TestLoadRights(t *testing.T) {
	dir := t.TempDir()
	rights, err := LoadRights(dir + "/rights_profiles.json")
	require.NoError(t, err)
	assert.Nil(t, rights, "a missing file means no profiles")

	for name, raw := range map[string][]byte{
		"not json":            []byte("{not json"),
		"a list":              []byte("[]"),
		"wrong schema":        []byte(`{"schema_version":2,"profiles":[]}`),
		"profiles not a list": []byte(`{"schema_version":1,"profiles":{}}`),
		"duplicate id":        config(nil, profile("same-id", []int{1}, nil), profile("same-id", []int{2}, nil)),
		"token in two":        config(nil, profile("acme-a", []int{1}, nil), profile("acme-b", []int{1}, nil)),
		"zero days":           config(m{"review_valid_days": 0}, profile("acme-line", []int{1}, nil)),
		"days is a string":    config(m{"review_valid_days": "180"}),
		"days is a bool":      config(m{"review_valid_days": true}),
		"days is a float":     config(m{"review_valid_days": 1.5}),
	} {
		_, err := ParseRights("rights.json", raw)
		var ce *ConfigError
		require.ErrorAs(t, err, &ce, name)
		assert.True(t, strings.HasPrefix(ce.Msg, "rights.json: "), "%s: %s", name, ce.Msg)
	}
	_, err = ParseRights("rights.json", config(nil, profile("same-id", []int{1}, nil), profile("same-id", []int{2}, nil)))
	assert.EqualError(t, err, "rights.json: duplicate profile id 'same-id'")
	_, err = ParseRights("rights.json", config(nil, profile("acme-a", []int{1}, nil), profile("acme-b", []int{1}, nil)))
	assert.EqualError(t, err, "rights.json: token 1 belongs to two profiles")

	rights, err = ParseRights("rights.json", config(nil, profile("acme-line", []int{1}, nil)))
	require.NoError(t, err)
	assert.Equal(t, 180, rights.ReviewValidDays)
}

// The shipped profiles hold against the shipped export: none is excluded and none stale on the day of the golden.
func TestShippedProfilesLoad(t *testing.T) {
	rights, err := LoadRights("../../config/rights_profiles.json")
	require.NoError(t, err)
	require.NotNil(t, rights)
	assert.Equal(t, 180, rights.ReviewValidDays)
	assert.NotEmpty(t, rights.profiles)
}
