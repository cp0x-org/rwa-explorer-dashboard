package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/publish"
)

// Excluded rights profiles make a successful publish exit 2 (Python `_build`), unless a stronger code is already
// set; valid or merely stale profiles keep the code.
func TestPublishExit(t *testing.T) {
	res := func(excluded int) *publish.Result {
		return &publish.Result{Summary: model.Summary{RightsExcludedProfilesCount: excluded}}
	}
	for _, tc := range []struct {
		name     string
		code     int
		excluded int
		want     int
		out      string
	}{
		{"clean", 0, 0, 0, ""},
		{"partial run stays partial", 2, 0, 2, ""},
		{"excluded profiles", 0, 3, 2, "3 rights profiles excluded"},
		{"partial and excluded", 2, 1, 2, "1 rights profiles excluded"},
		{"a stronger code wins", 1, 1, 1, "1 rights profiles excluded"},
	} {
		var out bytes.Buffer
		assert.Equal(t, tc.want, publishExit(&out, tc.code, res(tc.excluded)), tc.name)
		if tc.out == "" {
			assert.Empty(t, out.String(), tc.name)
		} else {
			assert.Contains(t, out.String(), tc.out, tc.name)
		}
	}
}
