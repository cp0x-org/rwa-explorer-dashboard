package curated

import (
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// CleanHTTPSURL is deals.py::clean_https_url: an absolute HTTPS URL without credentials, whitespace, control or
// unsafe characters and with a valid port.
func CleanHTTPSURL(u any) bool {
	s, ok := u.(string)
	if !ok || !strings.HasPrefix(s, "https://") {
		return false
	}
	for _, c := range s {
		if unicode.IsSpace(c) || c < 32 || c == 127 || strings.ContainsRune("\\\"<>`{}", c) {
			return false
		}
	}
	parsed, err := url.Parse(s)
	if err != nil {
		return false
	}
	if p := parsed.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n > 65535 {
			return false
		}
	}
	return parsed.Hostname() != "" && parsed.User == nil
}

// CleanDealPageURL is deals.py::clean_deal_page_url: only the exact cp0x page of this deal.
func CleanDealPageURL(u any, slug string) bool {
	s, ok := u.(string)
	return ok && CleanHTTPSURL(s) && s == "https://cp0x.com/deals/"+slug
}

// hostAllowed is curated.py::_allowed: a clean HTTPS URL whose lower-cased host is one of entry.hosts.
func hostAllowed(u any, entry *Obj) bool {
	hosts, ok := asList(entry.Get("hosts"))
	if !ok || !CleanHTTPSURL(u) {
		return false
	}
	parsed, err := url.Parse(u.(string))
	if err != nil {
		return false
	}
	hostname := strings.ToLower(parsed.Hostname())
	for _, h := range hosts {
		if s, ok := asStr(h); ok && s == hostname {
			return true
		}
	}
	return false
}
