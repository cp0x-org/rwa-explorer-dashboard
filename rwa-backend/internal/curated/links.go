package curated

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// DefaultDealsMaxAgeDays is the freshness window of the deals cache when links.json does not set deals_max_age_days.
const DefaultDealsMaxAgeDays = 14

// Links is a validated links.json.
type Links struct {
	root *Obj
	// DealsMaxAgeDays is deals_max_age_days (default 14).
	DealsMaxAgeDays float64
}

var isoDate = regexp.MustCompile(`\A[0-9]{4}-[0-9]{2}-[0-9]{2}\z`)

// parseISODate is date.fromisoformat for the YYYY-MM-DD form.
func parseISODate(s string) (time.Time, bool) {
	if !isoDate.MatchString(s) {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil || t.Year() < 1 {
		return time.Time{}, false
	}
	return t, true
}

// verified is curated.py::_verified: a record names the calendar date and the tested sample.
func verified(v any) bool {
	o, ok := asObj(v)
	if !ok {
		return false
	}
	at, ok := asStr(o.Get("at"))
	if !ok {
		return false
	}
	if _, ok := parseISODate(at); !ok {
		return false
	}
	sample, ok := asStr(o.Get("sample"))
	return ok && model.PyStrip(sample) != ""
}

// LoadLinks reads and validates links.json (curated.py::load_links). A missing file disables links (nil, nil);
// an unreadable or invalid one is a *ConfigError.
func LoadLinks(path string) (*Links, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path comes from operator configuration
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, configErrorf("%s: invalid links config: %v", path, err)
	}
	return ParseLinks(path, b)
}

// ParseLinks validates the content of a links.json; path only labels the errors.
func ParseLinks(path string, b []byte) (*Links, error) {
	v, err := parseJSON(b)
	if err != nil {
		return nil, configErrorf("%s: invalid links config: %v", path, err)
	}
	fail := func(location, expected string) error {
		return configErrorf("%s: %s: expected %s", path, location, expected)
	}
	checkEntry := func(entry any, location string) (*Obj, error) {
		o, ok := asObj(entry)
		if !ok {
			return nil, fail(location, "an object")
		}
		if o.Has("verified") && !verified(o.Get("verified")) {
			return nil, fail(location+".verified", "an object with an ISO date at and a nonempty sample")
		}
		if o.Has("deal") {
			if s, ok := asStr(o.Get("deal")); !ok || model.PyStrip(s) == "" {
				return nil, fail(location+".deal", "a nonempty string")
			}
		}
		if o.Has("hosts") {
			hosts, ok := asList(o.Get("hosts"))
			if !ok {
				return nil, fail(location+".hosts", "a list of nonempty strings")
			}
			for i, h := range hosts {
				if s, ok := asStr(h); !ok || s == "" {
					return nil, fail(fmt.Sprintf("%s.hosts[%d]", location, i), "a nonempty string")
				}
			}
		}
		return o, nil
	}

	root, ok := asObj(v)
	if !ok {
		return nil, fail("links", "an object")
	}
	if !isOne(root.Get("schema_version")) { // links.py: == 1, so true passes like 1
		return nil, fail("schema_version", "1")
	}
	platforms, ok := asObj(root.Get("platforms"))
	if !ok {
		return nil, fail("platforms", "an object")
	}
	for _, section := range []string{"issuers", "swap"} {
		if root.Has(section) {
			if _, ok := asObj(root.Get(section)); !ok {
				return nil, fail(section, "an object")
			}
		}
	}
	links := &Links{root: root, DealsMaxAgeDays: DefaultDealsMaxAgeDays}
	if root.Has("deals_max_age_days") {
		age, ok := isNumber(root.Get("deals_max_age_days"))
		if !ok || age < 0 {
			return nil, fail("deals_max_age_days", "a nonnegative number")
		}
		links.DealsMaxAgeDays = age
	}
	for _, name := range platforms.Keys() {
		location := "platforms." + name
		entry, err := checkEntry(platforms.Get(name), location)
		if err != nil {
			return nil, err
		}
		routes := []any{}
		if entry.Has("routes") {
			if routes, ok = asList(entry.Get("routes")); !ok {
				return nil, fail(location+".routes", "a list")
			}
		}
		for i, r := range routes {
			rl := fmt.Sprintf("%s.routes[%d]", location, i)
			route, ok := asObj(r)
			if !ok {
				return nil, fail(rl, "an object")
			}
			if route.Has("when") {
				if _, ok := asObj(route.Get("when")); !ok {
					return nil, fail(rl+".when", "an object")
				}
			}
			networks := newObj()
			if route.Has("networks") {
				if networks, ok = asObj(route.Get("networks")); !ok {
					return nil, fail(rl+".networks", "an object")
				}
			}
			for _, network := range networks.Keys() {
				if _, err := checkEntry(networks.Get(network), rl+".networks."+network); err != nil {
					return nil, err
				}
			}
		}
	}
	if issuers, ok := asObj(root.Get("issuers")); ok {
		for _, name := range issuers.Keys() {
			if _, err := checkEntry(issuers.Get(name), "issuers."+name); err != nil {
				return nil, err
			}
		}
	}
	if swap, ok := asObj(root.Get("swap")); ok {
		for _, network := range swap.Keys() {
			location := "swap." + network
			entries, ok := asList(swap.Get(network))
			if !ok {
				return nil, fail(location, "a list")
			}
			for i, e := range entries {
				el := fmt.Sprintf("%s[%d]", location, i)
				entry, err := checkEntry(e, el)
				if err != nil {
					return nil, err
				}
				if entry.Has("platforms") {
					plats, ok := asList(entry.Get("platforms"))
					if !ok {
						return nil, fail(el+".platforms", "a list of strings")
					}
					for j, p := range plats {
						if _, ok := asStr(p); !ok {
							return nil, fail(fmt.Sprintf("%s.platforms[%d]", el, j), "a string")
						}
					}
				}
			}
		}
	}
	return links, nil
}

// ReferencedDeals is curated.py::referenced_deals: the deal slugs named by platform and issuer actions.
func (l *Links) ReferencedDeals() map[string]bool {
	out := map[string]bool{}
	if l == nil {
		return out
	}
	for _, section := range []string{"platforms", "issuers"} {
		s, ok := asObj(l.root.Get(section))
		if !ok {
			continue
		}
		for _, k := range s.Keys() {
			if entry, ok := asObj(s.Get(k)); ok && entry.Has("deal") {
				if slug, ok := asStr(entry.Get("deal")); ok {
					out[slug] = true
				}
			}
		}
	}
	return out
}

var identifier = regexp.MustCompile(`\A[A-Za-z0-9_.-]+\z`)
var placeholder = regexp.MustCompile(`\{([^{}]+)\}`)

// render is curated.py::_render: every {placeholder} must resolve to a plain identifier string, and no brace may
// remain. values holds strings; anything else (missing, null) is unresolved.
func render(template any, values map[string]any) (string, bool) {
	t, ok := asStr(template)
	if !ok {
		return "", false
	}
	bad := false
	result := placeholder.ReplaceAllStringFunc(t, func(m string) string {
		v, ok := values[m[1:len(m)-1]].(string)
		if !ok || !identifier.MatchString(v) {
			bad = true
			return ""
		}
		return v
	})
	if bad || strings.ContainsAny(result, "{}") {
		return "", false
	}
	return result, true
}

// AttachLinks is curated.py::attach_links: it fills opportunity.link/referral, issuer.app/referral, token.links and
// the links_* summary fields in place and returns the warnings. A nil links disables every action; deals may be nil.
// meta is Side.PositionMetadata (the string-valued metadata of the raw Portals record by position key).
func AttachLinks(exp *model.Export, links *Links, deals *DealsCache, meta map[string]map[string]string, now time.Time) []string {
	var warnings []string
	warn := func(msg string) {
		if !slices.Contains(warnings, msg) {
			warnings = append(warnings, msg)
		}
	}
	var maxAge *float64
	fresh := false
	if links != nil {
		maxAge = &links.DealsMaxAgeDays
		fresh = deals.fresh(links.DealsMaxAgeDays, now)
	}
	section := func(name string) *Obj {
		if links == nil {
			return newObj()
		}
		o, _ := asObj(links.root.Get(name))
		if o == nil {
			o = newObj()
		}
		return o
	}
	platforms, issuerEntries, swap := section("platforms"), section("issuers"), section("swap")

	referral := func(entry *Obj, name string) *model.Referral {
		if !entry.Has("deal") {
			return nil
		}
		slug, ok := asStr(entry.Get("deal"))
		if !ok || model.PyStrip(slug) == "" {
			warn(name + ": invalid deals referral slug")
			return nil
		}
		var deal *Deal
		if fresh {
			deal = deals.Deals[slug]
		}
		if deal == nil || !hostAllowed(deal.Link, entry) {
			warn(fmt.Sprintf("%s: deals referral %s unavailable, stale or invalid", name, slug))
			return nil
		}
		r := &model.Referral{URL: deal.Link.(string), Deal: slug, Bonus: deal.Bonus, AsOf: deals.fetchedAtString()}
		if CleanDealPageURL(deal.PageURL, slug) {
			p := deal.PageURL.(string)
			r.PageURL = &p
		}
		return r
	}

	for i := range exp.Opportunities {
		position := &exp.Opportunities[i]
		position.Link, position.Referral = nil, nil
		entry, ok := asObj(platforms.Get(position.Platform))
		if !ok || entry.Len() == 0 {
			continue
		}
		name := "platform " + position.Platform
		values := map[string]any{"address": position.Address}
		for k, v := range meta[position.Key] {
			values["meta."+k] = v
		}
		routes, _ := asList(entry.Get("routes"))
		for _, r := range routes {
			route, _ := asObj(r)
			if !routeMatches(position, route) {
				continue
			}
			networks, _ := asObj(route.Get("networks"))
			if networks == nil {
				continue
			}
			network, ok := asObj(networks.Get(position.Network))
			if !ok || network.Len() == 0 || !verified(network.Get("verified")) {
				continue
			}
			routeValues := make(map[string]any, len(values)+1)
			for k, v := range values {
				routeValues[k] = v
			}
			routeValues["network"] = network.Get("slug")
			u, ok := render(route.Get("url"), routeValues)
			if !ok {
				continue
			}
			if !hostAllowed(u, entry) {
				warn(name + ": invalid route URL")
				continue
			}
			position.Link = &model.PositionLink{URL: u, Kind: "position"}
			break
		}
		if position.Link == nil {
			if home := entry.Get("home"); hostAllowed(home, entry) {
				position.Link = &model.PositionLink{URL: home.(string), Kind: "home"}
			} else {
				warn(name + ": invalid home URL")
			}
		}
		position.Referral = referral(entry, name)
	}

	for i := range exp.Issuers {
		issuer := &exp.Issuers[i]
		issuer.App, issuer.Referral = nil, nil
		entry, ok := asObj(issuerEntries.Get(issuer.ID))
		if !ok || entry.Len() == 0 {
			continue
		}
		name := "issuer " + issuer.ID
		if app := entry.Get("app"); hostAllowed(app, entry) {
			a := &model.IssuerApp{URL: app.(string)}
			if label, ok := asStr(entry.Get("label")); ok {
				a.Label = &label
			}
			issuer.App = a
		} else {
			warn(name + ": invalid app URL")
		}
		issuer.Referral = referral(entry, name)
	}

	opportunities := make(map[string]*model.Opportunity, len(exp.Opportunities))
	for i := range exp.Opportunities {
		opportunities[exp.Opportunities[i].Key] = &exp.Opportunities[i]
	}
	for i := range exp.Tokens {
		token := &exp.Tokens[i]
		token.Links = []model.SwapLink{}
		if links == nil {
			continue
		}
		var pools []*model.Opportunity
		for _, relation := range token.Relations {
			if o := opportunities[relation.Key]; relation.Depth == 1 && o != nil && o.Category == "dex_lp" && o.Eligibility == "eligible" {
				pools = append(pools, o)
			}
		}
		for _, deployment := range token.Deployments {
			if deployment.Network == nil {
				continue
			}
			network := *deployment.Network
			entries, _ := asList(swap.Get(network))
			for _, e := range entries {
				entry, _ := asObj(e)
				if entry == nil || !verified(entry.Get("verified")) || deployment.Key == nil || *deployment.Key == "" {
					continue
				}
				plats, _ := asList(entry.Get("platforms"))
				if !poolOnPlatform(pools, network, plats, *deployment.Key) {
					continue
				}
				u, ok := render(entry.Get("url"), map[string]any{"address": deployment.Address, "network": network})
				if !ok {
					continue
				}
				if !hostAllowed(u, entry) {
					for _, p := range plats {
						warn(fmt.Sprintf("platform %v: invalid swap URL on %s", p, network))
					}
					continue
				}
				link := model.SwapLink{Kind: "swap", URL: u, Network: network}
				if label, ok := asStr(entry.Get("label")); ok {
					link.Label = &label
				}
				if !slices.ContainsFunc(token.Links, func(l model.SwapLink) bool { return reflect.DeepEqual(l, link) }) {
					token.Links = append(token.Links, link)
				}
			}
		}
	}

	var fetchedAt *string
	if deals != nil {
		if _, ok := ParseTimestamp(deals.FetchedAt); ok {
			s := deals.fetchedAtString()
			fetchedAt = &s
		}
	}
	exp.Summary.LinksDealsFetchedAt = fetchedAt
	exp.Summary.LinksDealsMaxAgeDays = maxAge
	exp.Summary.LinksWarningsCount = len(warnings)
	return warnings
}

// routeMatches is `all(position.get(k) == v for k, v in route.get("when", {}).items())`: every condition names an
// exported field of the position and must equal its value.
func routeMatches(position *model.Opportunity, route *Obj) bool {
	when, ok := asObj(route.Get("when"))
	if !ok || when.Len() == 0 {
		return true
	}
	b, err := json.Marshal(position)
	if err != nil {
		return false
	}
	var fields map[string]any
	if json.Unmarshal(b, &fields) != nil {
		return false
	}
	for _, k := range when.Keys() {
		want, _ := json.Marshal(when.Get(k))
		var w any
		if json.Unmarshal(want, &w) != nil || !reflect.DeepEqual(fields[k], w) {
			return false
		}
	}
	return true
}

// poolOnPlatform is the `any(...)` of curated.py: an eligible direct pool of one of the platforms on this network
// that has the deployment among its components.
func poolOnPlatform(pools []*model.Opportunity, network string, platforms []any, key string) bool {
	for _, o := range pools {
		if o.Network != network || !slices.ContainsFunc(platforms, func(p any) bool { s, ok := p.(string); return ok && s == o.Platform }) {
			continue
		}
		if slices.ContainsFunc(o.Components, func(c model.Component) bool { return c.Key == key }) {
			return true
		}
	}
	return false
}
