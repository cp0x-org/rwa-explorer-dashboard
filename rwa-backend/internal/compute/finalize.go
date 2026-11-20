package compute

import (
	"strconv"
	"time"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

// SchemaVersion is the export schema_version.
const SchemaVersion = 1

// RwaInfo is one entry of rwa_info.json (CMC RWA metadata, keyed by rwa_id as a string).
type RwaInfo struct {
	Website  *string `json:"website"`
	Logo     *string `json:"logo"`
	Industry *string `json:"industry"`
}

// LogoResolver finds the logo file of an entity. kind is "tokens", "issuers" or "underlyings"; entityID is the token
// id, the issuer id or the storage id of the underlying (model.StorageID). ext is the file extension without the dot
// ("png"); when several files exist the resolver returns the first by name, like logos.py::find_logo.
type LogoResolver interface {
	Find(kind, entityID string) (ext string, ok bool)
}

// NoLogos is a LogoResolver that finds nothing.
type NoLogos struct{}

// Find implements LogoResolver.
func (NoLogos) Find(string, string) (string, bool) { return "", false }

// MapLogos is a LogoResolver backed by a map "kind/entityID" -> ext.
type MapLogos map[string]string

// Find implements LogoResolver.
func (m MapLogos) Find(kind, entityID string) (string, bool) {
	ext, ok := m[kind+"/"+entityID]
	return ext, ok
}

// Finalize is the part of export.py::build_export that follows analyze.build: schema_version, generated_at,
// the website of every underlying (rwa_info by cmc_rwa_id) and local logo paths "logos/<kind>/<id>.<ext>".
// Logos point to files only: the site never loads images from CMC or issuer websites, so a logo without a file is
// null. It fills res.Export in place and returns it.
func Finalize(res *Result, now time.Time, rwaInfo map[string]RwaInfo, logos LogoResolver) *model.Export {
	e := &res.Export
	e.SchemaVersion = SchemaVersion
	e.GeneratedAt = now.UTC().Format("2006-01-02T15:04:05Z")
	if logos == nil {
		logos = NoLogos{}
	}
	logoPath := func(kind, entityID string) *string {
		ext, ok := logos.Find(kind, entityID)
		if !ok {
			return nil
		}
		p := "logos/" + kind + "/" + entityID + "." + ext
		return &p
	}
	for i := range e.Underlyings {
		u := &e.Underlyings[i]
		id := "None" // str(None): a missing cmc_rwa_id never matches an entry
		if u.CMCRwaID != nil {
			id = strconv.Itoa(*u.CMCRwaID)
		}
		u.Website = rwaInfo[id].Website
		u.Logo = logoPath("underlyings", model.StorageID(u.ID))
	}
	for i := range e.Tokens {
		e.Tokens[i].Logo = logoPath("tokens", strconv.FormatInt(e.Tokens[i].ID, 10))
	}
	for i := range e.Issuers {
		e.Issuers[i].Logo = logoPath("issuers", e.Issuers[i].ID)
	}
	return e
}
