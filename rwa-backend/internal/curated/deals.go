package curated

import (
	"math"
	"time"
)

// DealsFile is the name of the cache file of the 1.x pipeline (data/cp0x_deals.json).
const DealsFile = "cp0x_deals.json"

// Deal is one cached cp0x deal. Entries that are not objects have a nil *Deal in DealsCache.Deals.
type Deal struct {
	Link    any
	Bonus   any
	PageURL any
}

// DealsCache is the deals cache `{fetched_at, deals}` as read by export.py: lenient, because the file is written by
// another process and an unusable cache only disables referrals.
type DealsCache struct {
	FetchedAt any
	// Deals is nil when the "deals" value is not an object (the cache is then never fresh).
	Deals map[string]*Deal
}

// ParseDealsCache reads the cache JSON. Anything that is not a JSON object yields nil (Python: the deals are None
// or not a dict, so no referral and no fetched_at).
func ParseDealsCache(b []byte) *DealsCache {
	v, err := parseJSON(b)
	if err != nil {
		return nil
	}
	root, ok := asObj(v)
	if !ok {
		return nil
	}
	c := &DealsCache{FetchedAt: root.Get("fetched_at")}
	if d, ok := asObj(root.Get("deals")); ok {
		c.Deals = make(map[string]*Deal, d.Len())
		for _, slug := range d.Keys() {
			if item, ok := asObj(d.Get(slug)); ok {
				c.Deals[slug] = &Deal{Link: item.Get("link"), Bonus: item.Get("bonus"), PageURL: item.Get("page_url")}
			} else {
				c.Deals[slug] = nil
			}
		}
	}
	return c
}

// timestampLayouts are the offset-carrying forms of Python datetime.fromisoformat that a cache writer produces.
var timestampLayouts = []string{
	"2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05Z07:00",
	"2006-01-02T15:04:05Z0700", "2006-01-02 15:04:05Z0700",
	"2006-01-02T15:04Z07:00", "2006-01-02 15:04Z07:00",
}

// ParseTimestamp is curated.py::_parse_timestamp: a string with a UTC offset (Z counts); anything else is invalid.
func ParseTimestamp(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, false
	}
	for _, layout := range timestampLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// maxTimedeltaDays is Python's timedelta limit: timedelta(days=x) raises OverflowError (ValueError for NaN) beyond it,
// which _fresh turns into "not fresh".
const maxTimedeltaDays = 999999999

// fresh is curated.py::_fresh: the cache is not older than maxAgeDays and not more than an hour in the future.
// Ages are compared as seconds from calendar arithmetic, not as a time.Duration (which saturates at 292 years); an
// unrepresentable maxAgeDays (|x| > 999999999 or NaN) is "not fresh" like Python's OverflowError.
func (c *DealsCache) fresh(maxAgeDays float64, now time.Time) bool {
	if c == nil || c.Deals == nil {
		return false
	}
	fetched, ok := ParseTimestamp(c.FetchedAt)
	if !ok {
		return false
	}
	if math.IsNaN(maxAgeDays) || math.Abs(maxAgeDays) > maxTimedeltaDays {
		return false
	}
	age := float64(now.Unix()-fetched.Unix()) + float64(now.Nanosecond()-fetched.Nanosecond())/1e9
	return age >= -3600 && age <= maxAgeDays*86400
}

func (c *DealsCache) fetchedAtString() string {
	s, _ := c.FetchedAt.(string)
	return s
}
