package curated

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// File names of the curated configs inside the config directory.
const (
	LinksFile   = "links.json"
	RightsFile  = "rights_profiles.json"
	IssuersFile = "issuers.json"
)

// Config is the curated configuration of one process. Every part is optional: a missing file disables its feature
// (no links, no rights profiles, no issuer websites), exactly like the Python loaders.
type Config struct {
	Links *Links
	// Rights is the rights_profiles.json content.
	Rights *Rights
	// IssuerWebsites maps an issuer id to the website used when CMC has none (issuers.json).
	IssuerWebsites map[string]string
}

// Load reads links.json, rights_profiles.json and issuers.json from dir. An invalid file is an error (a *ConfigError).
func Load(dir string) (*Config, error) {
	links, err := LoadLinks(filepath.Join(dir, LinksFile))
	if err != nil {
		return nil, fmt.Errorf("curated: %w", err)
	}
	rights, err := LoadRights(filepath.Join(dir, RightsFile))
	if err != nil {
		return nil, fmt.Errorf("curated: %w", err)
	}
	websites, err := LoadIssuerWebsites(filepath.Join(dir, IssuersFile))
	if err != nil {
		return nil, fmt.Errorf("curated: %w", err)
	}
	return &Config{Links: links, Rights: rights, IssuerWebsites: websites}, nil
}

// LoadIssuerWebsites is export.py::load_issuer_overrides: the websites used when CMC has none for an issuer.
// A missing file is an empty map; the file needs schema_version 1 and an issuers object whose entries contain
// only a clean HTTPS website.
func LoadIssuerWebsites(path string) (map[string]string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path comes from operator configuration
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, configErrorf("%s: invalid issuers config: %v", path, err)
	}
	return ParseIssuerWebsites(path, b)
}

// ParseIssuerWebsites validates the content of an issuers.json; path only labels the errors.
func ParseIssuerWebsites(path string, b []byte) (map[string]string, error) {
	v, err := parseJSON(b)
	if err != nil {
		return nil, configErrorf("%s: invalid issuers config: %v", path, err)
	}
	root, ok := asObj(v)
	var issuers *Obj
	if ok {
		issuers, ok = asObj(root.Get("issuers"))
	}
	if !ok || !isOne(rootGet(root, "schema_version")) { // export.py: != 1 is the error, so true passes like 1
		return nil, configErrorf("%s: expected an object with schema_version 1 and an issuers object", path)
	}
	out := make(map[string]string, issuers.Len())
	for _, id := range issuers.Keys() {
		entry, ok := asObj(issuers.Get(id))
		if !ok || entry.Len() != 1 || !entry.Has("website") || !CleanHTTPSURL(entry.Get("website")) {
			return nil, configErrorf("%s: issuers.%s: expected an object containing only a clean HTTPS website", path, id)
		}
		out[id] = entry.Get("website").(string)
	}
	return out, nil
}

func rootGet(o *Obj, k string) any {
	if o == nil {
		return nil
	}
	return o.Get(k)
}

// ApplyIssuerWebsites sets the override website on issuers that have none (export.py::build_export_with_warnings):
// an issuer without a website and without an override ends up with a null website.
func ApplyIssuerWebsites(issuers []model.Issuer, websites map[string]string) {
	for i := range issuers {
		is := &issuers[i]
		if is.Website != nil && *is.Website != "" {
			continue
		}
		is.Website = nil
		if w, ok := websites[is.ID]; ok {
			is.Website = &w
		}
	}
}

// Apply is the curated part of export.py::build_export_with_warnings, to run after compute.Finalize: issuer website
// overrides, then links and referrals, then holder rights, in that order. cfg and deals may be nil (no files, no
// cache). now is the build time; its UTC date is "today" for the review dates. It returns the warnings of links
// followed by those of rights.
func Apply(exp *model.Export, cfg *Config, deals *DealsCache, positionMetadata map[string]map[string]string, now time.Time) []string {
	if cfg == nil {
		cfg = &Config{}
	}
	now = now.UTC()
	ApplyIssuerWebsites(exp.Issuers, cfg.IssuerWebsites)
	warnings := AttachLinks(exp, cfg.Links, deals, positionMetadata, now)
	return append(warnings, AttachRights(exp, cfg.Rights, now)...)
}
