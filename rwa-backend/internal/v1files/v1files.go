// Package v1files reads the files of the 1.x pipeline (data/issuers.json, rwa_info.json, portals_platforms.json)
// into the types compute takes. It is the only place where compute's inputs touch the file system.
package v1files

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/compute"
)

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path) //nolint:gosec // path comes from operator configuration
	if err != nil {
		return fmt.Errorf("v1files: read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("v1files: parse %s: %w", path, err)
	}
	return nil
}

// LoadIssuers reads issuers.json (a missing file is an error; pass an empty map for "no cache").
func LoadIssuers(path string) (map[string]compute.IssuerMeta, error) {
	var m map[string]compute.IssuerMeta
	if err := readJSON(path, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// LoadRwaInfo reads rwa_info.json.
func LoadRwaInfo(path string) (map[string]compute.RwaInfo, error) {
	var m map[string]compute.RwaInfo
	if err := readJSON(path, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// LoadPlatformNames reads portals_platforms.json (slug -> protocol name).
func LoadPlatformNames(path string) (map[string]string, error) {
	var m map[string]string
	if err := readJSON(path, &m); err != nil {
		return nil, err
	}
	return m, nil
}
