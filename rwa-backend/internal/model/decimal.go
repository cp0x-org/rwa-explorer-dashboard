// Package model holds the data structures shared by the collector, compute, store and API:
// the rwa.json export (1.x format), the 1.x snapshot document and the configuration files.
package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// Decimal is a nullable exact decimal. Money, APY and liquidity are never floats: the export carries
// them as JSON strings without an exponent, and null means "not reported", never zero.
// The zero value is null.
type Decimal struct {
	V *decimal.Decimal
}

// NewDecimal wraps d.
func NewDecimal(d decimal.Decimal) Decimal { return Decimal{V: &d} }

// ParseDecimal parses a plain or exponent-notation decimal string.
func ParseDecimal(s string) (Decimal, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return Decimal{}, fmt.Errorf("model: parse decimal %q: %w", s, err)
	}
	return NewDecimal(d), nil
}

// MustDecimal is ParseDecimal that panics; for tests and constants.
func MustDecimal(s string) Decimal {
	d, err := ParseDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

// IsNull reports whether the value is absent.
func (d Decimal) IsNull() bool { return d.V == nil }

// String returns the plain decimal notation (no exponent), or "" for null.
func (d Decimal) String() string {
	if d.V == nil {
		return ""
	}
	return d.V.String()
}

// StringPtr returns nil for null, otherwise a pointer to the plain string.
func (d Decimal) StringPtr() *string {
	if d.V == nil {
		return nil
	}
	s := d.V.String()
	return &s
}

// Cmp compares two values; null sorts before any number and equals null.
func (d Decimal) Cmp(o Decimal) int {
	switch {
	case d.V == nil && o.V == nil:
		return 0
	case d.V == nil:
		return -1
	case o.V == nil:
		return 1
	}
	return d.V.Cmp(*o.V)
}

// MarshalJSON writes a JSON string, or null.
func (d Decimal) MarshalJSON() ([]byte, error) {
	if d.V == nil {
		return []byte("null"), nil
	}
	return json.Marshal(d.V.String())
}

// UnmarshalJSON accepts a JSON string, a JSON number or null.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		d.V = nil
		return nil
	}
	s := string(b)
	if b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("model: decimal: %w", err)
		}
	}
	v, err := decimal.NewFromString(s)
	if err != nil {
		return fmt.Errorf("model: decimal %q: %w", s, err)
	}
	d.V = &v
	return nil
}

// LenientDecimal is a Decimal for snapshot fields, mirroring the 1.x dec(): anything that is not a finite
// number (null, bool, "", "NaN", "Infinity", words, objects) becomes null instead of an error, so one
// odd Portals value cannot fail a whole snapshot. Surrounding whitespace in strings is ignored.
type LenientDecimal struct {
	Decimal
}

// UnmarshalJSON never fails on a well-formed JSON value.
func (d *LenientDecimal) UnmarshalJSON(b []byte) error {
	d.V = nil
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	switch b[0] {
	case '"':
		if err := json.Unmarshal(b, &s); err != nil {
			return nil
		}
		s = strings.TrimSpace(s)
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
	default: // null, true, false, objects, arrays
		return nil
	}
	if v, err := decimal.NewFromString(s); err == nil {
		d.V = &v
	}
	return nil
}
