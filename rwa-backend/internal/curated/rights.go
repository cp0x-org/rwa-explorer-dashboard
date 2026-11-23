package curated

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// DefaultReviewValidDays is how long a review stays current when rights_profiles.json sets no review_valid_days.
const DefaultReviewValidDays = 180

type factSpec struct {
	Key      string
	Group    string
	Values   []string
	NeedsWho []string
}

var (
	lossValues    = []string{"holder_bears", "replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim", "not_addressed"}
	lossNeedsWho  = []string{"replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim"}
	yesNo         = []string{"yes", "no"}
	rightsFactDef = []factSpec{
		{"instrument", "what", []string{"share", "beneficial_interest", "secured_debt", "unsecured_debt", "derivative", "contractual_claim", "no_claim"}, nil},
		{"backing", "what", []string{"full_1to1", "full", "partial", "none"}, nil},
		{"asset_protection", "what", []string{"pledged_to_holders", "trust_or_segregated", "none"}, nil},
		{"primary_access", "who", []string{"available"}, []string{"available"}},
		{"redemption", "who", []string{"available", "at_events_only", "not_redeemable"}, []string{"available", "at_events_only"}},
		{"claim_scope", "who", []string{"has_claim", "no_claim"}, []string{"has_claim"}},
		{"restricted", "who", []string{"none_stated", "listed"}, nil},
		{"transfer", "who", []string{"free", "blocklist", "allowlist"}, nil},
		{"freeze", "issuer_powers", yesNo, nil},
		{"cancellation", "issuer_powers", yesNo, nil},
		{"pause", "issuer_powers", yesNo, nil},
		{"upgrade", "issuer_powers", yesNo, nil},
		{"forced_redemption", "issuer_powers", yesNo, nil},
		{"loss_exploit", "loss", lossValues, lossNeedsWho},
		{"loss_lost_tokens", "loss", lossValues, lossNeedsWho},
		{"loss_issuer_insolvency", "loss", lossValues, lossNeedsWho},
		{"loss_custodian_failure", "loss", lossValues, lossNeedsWho},
		{"dividends", "shareholder", []string{"paid_out", "reinvested", "none"}, nil},
		{"voting", "shareholder", []string{"none", "pass_through", "discretionary"}, []string{"discretionary"}},
		{"reserves_proof", "other", []string{"published", "none"}, nil},
		{"governing_law", "other", []string{"stated"}, nil},
	}
	rightsWho = []string{"anyone", "verified", "professional", "institutional", "platform_clients", "eligible_holders", "nobody"}
)

func factByKey(key string) (factSpec, bool) {
	for _, f := range rightsFactDef {
		if f.Key == key {
			return f, true
		}
	}
	return factSpec{}, false
}

// Rights is a validated rights_profiles.json: file-wide rules are checked on load, every profile when attached.
type Rights struct {
	// ReviewValidDays is review_valid_days (default 180).
	ReviewValidDays int
	profiles        []any
}

// LoadRights reads rights_profiles.json (rights.py::load_rights_profiles). A missing file means no profiles
// (nil, nil); a file-wide error is a *ConfigError.
func LoadRights(path string) (*Rights, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path comes from operator configuration
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, configErrorf("%s: invalid rights config: %v", path, err)
	}
	return ParseRights(path, b)
}

// ParseRights validates the content of a rights_profiles.json; path only labels the errors.
func ParseRights(path string, b []byte) (*Rights, error) {
	v, err := parseJSON(b)
	if err != nil {
		return nil, configErrorf("%s: invalid rights config: %v", path, err)
	}
	require := func(ok bool, message string) error {
		if !ok {
			return configErrorf("%s: %s", path, message)
		}
		return nil
	}
	config, ok := asObj(v)
	if err := require(ok, "expected an object"); err != nil {
		return nil, err
	}
	if n, ok := isInt(config.Get("schema_version")); !ok || n != 1 {
		return nil, require(false, "schema_version must be 1")
	}
	profiles, ok := asList(config.Get("profiles"))
	if err := require(ok, "profiles must be a list"); err != nil {
		return nil, err
	}
	rights := &Rights{ReviewValidDays: DefaultReviewValidDays, profiles: profiles}
	if config.Has("review_valid_days") {
		days, ok := isInt(config.Get("review_valid_days"))
		if err := require(ok && days > 0, "review_valid_days must be a positive integer"); err != nil {
			return nil, err
		}
		rights.ReviewValidDays = int(days)
	}
	ids, tokens := map[string]bool{}, map[int64]bool{}
	for _, p := range profiles {
		profile, ok := asObj(p)
		if !ok {
			continue
		}
		if id, ok := asStr(profile.Get("id")); ok {
			if err := require(!ids[id], "duplicate profile id "+pyRepr(id)); err != nil {
				return nil, err
			}
			ids[id] = true
		}
		if tokenIDs, ok := asList(profile.Get("token_ids")); ok {
			listed := map[int64]bool{}
			for _, t := range tokenIDs {
				if id, ok := isInt(t); ok {
					listed[id] = true
				}
			}
			for _, id := range sortedInts(listed) {
				if err := require(!tokens[id], fmt.Sprintf("token %d belongs to two profiles", id)); err != nil {
					return nil, err
				}
			}
			for id := range listed {
				tokens[id] = true
			}
		}
	}
	return rights, nil
}

func sortedInts(m map[int64]bool) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// profileError is a ValueError of the Python validators: its text is "<location>: expected <what>".
type profileError struct{ msg string }

func (e *profileError) Error() string { return e.msg }

func req(ok bool, location, expected string) error {
	if !ok {
		return &profileError{fmt.Sprintf("%s: expected %s", location, expected)}
	}
	return nil
}

// closedObject: every object has a closed set of keys.
func closedObject(v any, keys []string, location string) (*Obj, error) {
	o, ok := asObj(v)
	if err := req(ok, location, "an object"); err != nil {
		return nil, err
	}
	for _, k := range o.Keys() {
		if err := req(slices.Contains(keys, k), location, "known keys only (unexpected "+pyRepr(k)+")"); err != nil {
			return nil, err
		}
	}
	return o, nil
}

func textOK(v any) bool {
	s, ok := asStr(v)
	return ok && model.PyStrip(s) != ""
}

func checkText(v any, location string) error { return req(textOK(v), location, "a nonempty string") }

var profileID = regexp.MustCompile(`\A[a-z0-9-]+\z`)
var countryCode = regexp.MustCompile(`\A[A-Z]{2}\z`)

func checkCountry(v any, location string) error {
	s, ok := asStr(v)
	return req(ok && countryCode.MatchString(s), location, "a two-letter uppercase country code")
}

// checkDate: review and retrieval dates are ISO calendar dates no later than today.
func checkDate(v any, location string, today time.Time) (time.Time, error) {
	s, ok := asStr(v)
	if !ok || !isoDate.MatchString(s) {
		return time.Time{}, req(false, location, "an ISO date")
	}
	parsed, ok := parseISODate(s)
	if !ok {
		return time.Time{}, req(false, location, "an ISO date")
	}
	return parsed, req(!parsed.After(today), location, "a date no later than today")
}

func checkNote(v any, location string) error {
	o, err := closedObject(v, []string{"en", "zh"}, location)
	if err != nil {
		return err
	}
	for _, lang := range []string{"en", "zh"} {
		if err := checkText(o.Get(lang), location+"."+lang); err != nil {
			return err
		}
	}
	return nil
}

// checkEvidence: every cited fact has a short verbatim quote and a clean HTTPS source.
func checkEvidence(v any, location string, today time.Time) error {
	list, ok := asList(v)
	if err := req(ok && len(list) > 0, location, "a nonempty evidence list"); err != nil {
		return err
	}
	for i, c := range list {
		at := fmt.Sprintf("%s[%d]", location, i)
		citation, err := closedObject(c, []string{"quote", "url", "title", "doc_date", "retrieved_at"}, at)
		if err != nil {
			return err
		}
		if err := checkText(citation.Get("quote"), at+".quote"); err != nil {
			return err
		}
		quote := model.CollapseSpace(citation.Get("quote").(string))
		if err := req(utf8.RuneCountInString(quote) <= 600, at+".quote", "at most 600 characters"); err != nil {
			return err
		}
		if err := req(len(strings.Fields(quote)) >= 6, at+".quote", "at least six words"); err != nil {
			return err
		}
		if err := req(CleanHTTPSURL(citation.Get("url")), at+".url", "a clean HTTPS URL"); err != nil {
			return err
		}
		for _, k := range []string{"title", "doc_date"} {
			if err := checkText(citation.Get(k), at+"."+k); err != nil {
				return err
			}
		}
		if _, err := checkDate(citation.Get("retrieved_at"), at+".retrieved_at", today); err != nil {
			return err
		}
	}
	return nil
}

func checkFact(key string, v any, today time.Time) error {
	at := "facts." + key
	value, err := closedObject(v, []string{"value", "who", "countries", "zones", "us_persons", "note", "evidence", "checked"}, at)
	if err != nil {
		return err
	}
	spec, _ := factByKey(key)
	choice, _ := asStr(value.Get("value"))
	_, isStr := asStr(value.Get("value"))
	if err := req(isStr && (choice == "unknown" || slices.Contains(spec.Values, choice)), at+".value", "an allowed value or unknown"); err != nil {
		return err
	}
	if choice == "unknown" {
		if err := req(!value.Has("evidence"), at, "no evidence for unknown"); err != nil {
			return err
		}
		checked, ok := asList(value.Get("checked"))
		if err := req(ok && len(checked) > 0, at+".checked", "a nonempty list of clean HTTPS URLs"); err != nil {
			return err
		}
		for _, u := range checked {
			if err := req(CleanHTTPSURL(u), at+".checked", "clean HTTPS URLs"); err != nil {
				return err
			}
		}
	} else {
		if err := req(!value.Has("checked"), at, "checked only for unknown"); err != nil {
			return err
		}
		if err := checkEvidence(value.Get("evidence"), at+".evidence", today); err != nil {
			return err
		}
	}
	if slices.Contains(spec.NeedsWho, choice) {
		who, ok := asStr(value.Get("who"))
		if err := req(ok && slices.Contains(rightsWho, who), at+".who", "an allowed holder category"); err != nil {
			return err
		}
	} else if err := req(!value.Has("who"), at, "who only on values that need it"); err != nil {
		return err
	}
	if value.Has("countries") {
		countries, ok := asList(value.Get("countries"))
		if err := req(ok, at+".countries", "a list"); err != nil {
			return err
		}
		for _, c := range countries {
			if err := checkCountry(c, at+".countries"); err != nil {
				return err
			}
		}
	}
	if value.Has("zones") {
		zones, ok := asList(value.Get("zones"))
		if err := req(ok, at+".zones", "a list"); err != nil {
			return err
		}
		for _, z := range zones {
			if err := checkText(z, at+".zones"); err != nil {
				return err
			}
			if err := req(utf8.RuneCountInString(z.(string)) <= 40, at+".zones", "strings of at most 40 characters"); err != nil {
				return err
			}
		}
	}
	usPersons, usIsTrue := false, false
	if value.Has("us_persons") {
		usPersons, usIsTrue = value.Get("us_persons").(bool)
		if err := req(key == "restricted" && usIsTrue, at+".us_persons", "a boolean on restricted only"); err != nil {
			return err
		}
	}
	usTrue := usIsTrue && usPersons
	if key == "restricted" && choice == "listed" {
		if err := req(truthy(value.Get("countries")) || truthy(value.Get("zones")) || usTrue, at, "countries, zones or us_persons: true"); err != nil {
			return err
		}
	}
	if key == "restricted" && choice == "none_stated" {
		if err := req(!value.Has("countries") && !value.Has("zones") && !usTrue, at, "no restriction details for none_stated"); err != nil {
			return err
		}
	}
	if key == "governing_law" && choice == "stated" {
		if err := req(truthy(value.Get("countries")) || truthy(value.Get("zones")), at, "countries or zones"); err != nil {
			return err
		}
	}
	if value.Has("note") {
		return checkNote(value.Get("note"), at+".note")
	}
	return nil
}

// checkProfile is rights.py::_profile: a profile is accepted only after all its fields and bindings pass. It
// returns the review date.
func checkProfile(v any, issuers map[string]*model.Issuer, tokens map[int64]*model.Token, today time.Time) (time.Time, error) {
	keys := []string{"id", "issuer_id", "cmc_label", "product", "token_ids", "scope_basis", "scope_evidence",
		"reviewed_at", "legal_issuer", "facts", "documents"}
	profile, err := closedObject(v, keys, "profile")
	if err != nil {
		return time.Time{}, err
	}
	id, ok := asStr(profile.Get("id"))
	if err := req(ok && profileID.MatchString(id), "id", "lowercase letters, digits and hyphens"); err != nil {
		return time.Time{}, err
	}
	for _, k := range []string{"issuer_id", "cmc_label", "product"} {
		if err := checkText(profile.Get(k), k); err != nil {
			return time.Time{}, err
		}
	}
	tokenList, ok := asList(profile.Get("token_ids"))
	if err := req(ok && len(tokenList) > 0, "token_ids", "a nonempty list of integers"); err != nil {
		return time.Time{}, err
	}
	var tokenIDs []int64
	allInts := true
	for _, t := range tokenList {
		n, ok := isInt(t)
		allInts = allInts && ok
		tokenIDs = append(tokenIDs, n)
	}
	if err := req(allInts, "token_ids", "integers without booleans"); err != nil {
		return time.Time{}, err
	}
	uniq := map[int64]bool{}
	for _, t := range tokenIDs {
		uniq[t] = true
	}
	if err := req(len(uniq) == len(tokenIDs), "token_ids", "unique token IDs"); err != nil {
		return time.Time{}, err
	}
	basis, _ := asStr(profile.Get("scope_basis"))
	if err := req(basis == "per_token_documents" || basis == "programme_documents", "scope_basis", "per_token_documents or programme_documents"); err != nil {
		return time.Time{}, err
	}
	if basis == "per_token_documents" {
		if err := req(len(tokenIDs) == 1, "token_ids", "exactly one token for per_token_documents"); err != nil {
			return time.Time{}, err
		}
	}
	if basis == "programme_documents" || profile.Has("scope_evidence") {
		if err := checkEvidence(profile.Get("scope_evidence"), "scope_evidence", today); err != nil {
			return time.Time{}, err
		}
	}
	reviewedAt, err := checkDate(profile.Get("reviewed_at"), "reviewed_at", today)
	if err != nil {
		return time.Time{}, err
	}
	issuerID, _ := asStr(profile.Get("issuer_id"))
	cmcLabel, _ := asStr(profile.Get("cmc_label"))
	if issuer := issuers[issuerID]; issuer == nil {
		present := false
		for _, t := range tokenIDs {
			present = present || tokens[t] != nil
		}
		if err := req(!present, "issuer_id", "an issuer present in the export"); err != nil {
			return time.Time{}, err
		}
	} else if err := req(issuer.CMCLabel != nil && *issuer.CMCLabel == cmcLabel, "cmc_label", "the export issuer label"); err != nil {
		return time.Time{}, err
	}
	for _, t := range tokenIDs {
		token := tokens[t]
		if err := req(token == nil || (token.IssuerID != nil && *token.IssuerID == issuerID), fmt.Sprintf("token_ids[%d]", t), "a token of this issuer"); err != nil {
			return time.Time{}, err
		}
	}
	legal, err := closedObject(profile.Get("legal_issuer"), []string{"name", "country", "zone", "note", "evidence"}, "legal_issuer")
	if err != nil {
		return time.Time{}, err
	}
	if err := checkText(legal.Get("name"), "legal_issuer.name"); err != nil {
		return time.Time{}, err
	}
	if err := checkCountry(legal.Get("country"), "legal_issuer.country"); err != nil {
		return time.Time{}, err
	}
	if legal.Has("zone") {
		if err := checkText(legal.Get("zone"), "legal_issuer.zone"); err != nil {
			return time.Time{}, err
		}
	}
	if legal.Has("note") {
		if err := checkNote(legal.Get("note"), "legal_issuer.note"); err != nil {
			return time.Time{}, err
		}
	}
	if err := checkEvidence(legal.Get("evidence"), "legal_issuer.evidence", today); err != nil {
		return time.Time{}, err
	}
	factNames := make([]string, len(rightsFactDef))
	for i, f := range rightsFactDef {
		factNames[i] = f.Key
	}
	facts, err := closedObject(profile.Get("facts"), factNames, "facts")
	if err != nil {
		return time.Time{}, err
	}
	for _, key := range factNames {
		if err := checkFact(key, facts.Get(key), today); err != nil {
			return time.Time{}, err
		}
	}
	documents, ok := asList(profile.Get("documents"))
	if err := req(ok && len(documents) > 0, "documents", "a nonempty document list"); err != nil {
		return time.Time{}, err
	}
	for i, d := range documents {
		at := fmt.Sprintf("documents[%d]", i)
		doc, err := closedObject(d, []string{"title", "url", "type", "doc_date"}, at)
		if err != nil {
			return time.Time{}, err
		}
		for _, k := range []string{"title", "doc_date"} {
			if err := checkText(doc.Get(k), at+"."+k); err != nil {
				return time.Time{}, err
			}
		}
		if err := req(CleanHTTPSURL(doc.Get("url")), at+".url", "a clean HTTPS URL"); err != nil {
			return time.Time{}, err
		}
		typ, _ := asStr(doc.Get("type"))
		if err := req(slices.Contains([]string{"terms", "prospectus", "risk", "faq", "register", "other"}, typ), at+".type", "an allowed document type"); err != nil {
			return time.Time{}, err
		}
	}
	return reviewedAt, nil
}

// schemaJSON is the rights_schema of the export: the schema of the facts the frontend renders.
func schemaJSON() json.RawMessage {
	facts := make([]any, len(rightsFactDef))
	for i, f := range rightsFactDef {
		o := newObj()
		o.Set("key", f.Key)
		o.Set("group", f.Group)
		o.Set("values", stringList(f.Values))
		o.Set("needs_who", stringList(f.NeedsWho))
		facts[i] = o
	}
	root := newObj()
	root.Set("unknown_value", "unknown")
	root.Set("who", stringList(rightsWho))
	root.Set("facts", facts)
	b, _ := root.MarshalJSON() // strings only
	return b
}

func stringList(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// AttachRights is rights.py::attach_rights: valid profiles are published as copies, excluded ones never bind
// tokens. It fills export.rights_*, token.rights_profile_id, issuer.rights_* and the rights_* summary fields in
// place and returns the warnings. today is the UTC date of the build. A nil rights means no profiles.
func AttachRights(exp *model.Export, rights *Rights, today time.Time) []string {
	var warnings []string
	exp.RightsProfiles = []json.RawMessage{}
	exp.RightsSchema = schemaJSON()
	issuers := make(map[string]*model.Issuer, len(exp.Issuers))
	for i := range exp.Issuers {
		issuers[exp.Issuers[i].ID] = &exp.Issuers[i]
		exp.Issuers[i].RightsProfileIDs = []string{}
		exp.Issuers[i].RightsReviewedTokensCount = 0
	}
	tokens := make(map[int64]*model.Token, len(exp.Tokens))
	for i := range exp.Tokens {
		tokens[exp.Tokens[i].ID] = &exp.Tokens[i]
		exp.Tokens[i].RightsProfileID = nil
	}
	validDays := DefaultReviewValidDays
	var profiles []any
	if rights != nil {
		validDays, profiles = rights.ReviewValidDays, rights.profiles
	}
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	excluded := 0
	for index, raw := range profiles {
		name := "#" + strconv.Itoa(index+1)
		if p, ok := asObj(raw); ok && p.Has("id") {
			name = pyStr(p.Get("id"))
		}
		reviewedAt, err := checkProfile(raw, issuers, tokens, today)
		if err != nil {
			var pe *profileError
			if !errors.As(err, &pe) {
				// Only profileError is produced by the checks; keep the guard so a bug is not reported as a bad profile.
				panic(err)
			}
			warnings = append(warnings, fmt.Sprintf("rights profile %s: excluded: %s", name, err))
			excluded++
			continue
		}
		profile := raw.(*Obj)
		var present, missing []int64
		for _, t := range profile.Get("token_ids").([]any) {
			id, _ := isInt(t)
			if tokens[id] != nil {
				present = append(present, id)
			} else if !slices.Contains(missing, id) {
				missing = append(missing, id)
			}
		}
		if len(present) == 0 {
			warnings = append(warnings, fmt.Sprintf("rights profile %s: outside the export; absent tokens: %s", name, joinInts(missing)))
			continue
		}
		if len(missing) > 0 {
			warnings = append(warnings, fmt.Sprintf("rights profile %s: tokens %s are absent from the export", name, joinInts(missing)))
		}
		published := deepCopy(profile).(*Obj)
		presentList := make([]any, len(present))
		for i, id := range present {
			presentList[i] = json.Number(strconv.FormatInt(id, 10))
		}
		published.Set("token_ids", presentList)
		normalizeQuotes(published)
		stale := wholeDays(today, reviewedAt) > int64(validDays)
		published.Set("stale", stale)
		b, err := published.MarshalJSON()
		if err != nil {
			panic(err) // parsed JSON values always marshal
		}
		exp.RightsProfiles = append(exp.RightsProfiles, b)
		if stale {
			warnings = append(warnings, fmt.Sprintf("rights profile %s: review is stale (older than %d days)", name, validDays))
		}
		profileID := profile.Get("id").(string)
		issuer := issuers[profile.Get("issuer_id").(string)]
		issuer.RightsProfileIDs = append(issuer.RightsProfileIDs, profileID)
		for _, id := range present {
			tokens[id].RightsProfileID = &profileID
		}
	}
	reviewed := 0
	for i := range exp.Tokens {
		if exp.Tokens[i].RightsProfileID != nil {
			reviewed++
			if issuer := issuers[deref(exp.Tokens[i].IssuerID)]; issuer != nil {
				issuer.RightsReviewedTokensCount++
			}
		}
	}
	exp.Summary.RightsProfilesCount = len(exp.RightsProfiles)
	exp.Summary.RightsReviewedTokensCount = reviewed
	exp.Summary.RightsReviewValidDays = validDays
	exp.Summary.RightsExcludedProfilesCount = excluded
	exp.Summary.RightsWarningsCount = len(warnings)
	return warnings
}

// wholeDays is Python's (a - b).days for two dates: the floor of the calendar difference in days, which unlike
// time.Duration arithmetic does not saturate for dates centuries apart.
func wholeDays(a, b time.Time) int64 {
	d := a.Unix() - b.Unix()
	q := d / 86400
	if d%86400 < 0 {
		q--
	}
	return q
}

// normalizeQuotes collapses whitespace in every citation quote of a published profile.
func normalizeQuotes(profile *Obj) {
	var citations []any
	if l, ok := asList(profile.Get("scope_evidence")); ok {
		citations = append(citations, l...)
	}
	if legal, ok := asObj(profile.Get("legal_issuer")); ok {
		l, _ := asList(legal.Get("evidence"))
		citations = append(citations, l...)
	}
	if facts, ok := asObj(profile.Get("facts")); ok {
		for _, k := range facts.Keys() {
			if fact, ok := asObj(facts.Get(k)); ok {
				l, _ := asList(fact.Get("evidence"))
				citations = append(citations, l...)
			}
		}
	}
	for _, c := range citations {
		if o, ok := asObj(c); ok {
			if q, ok := asStr(o.Get("quote")); ok {
				o.Set("quote", model.CollapseSpace(q))
			}
		}
	}
}

func joinInts(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ", ")
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// pyStr is str() of a parsed value for profile names: strings as they are, anything else as its JSON text.
func pyStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return "None"
	}
	b, _ := json.Marshal(v)
	return string(b)
}
