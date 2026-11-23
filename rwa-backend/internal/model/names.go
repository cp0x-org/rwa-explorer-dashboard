package model

import "strings"

// isPySpace is Python's str.isspace() for one character: unicode whitespace, which also covers the separators
// \x1c-\x1f that unicode.IsSpace does not know.
func isPySpace(r rune) bool {
	return r == ' ' || (r >= '\t' && r <= '\r') || (r >= 0x1c && r <= 0x1f) || r == 0x85 || r == 0xa0 ||
		r == 0x1680 || (r >= 0x2000 && r <= 0x200a) || r == 0x2028 || r == 0x2029 || r == 0x202f || r == 0x205f || r == 0x3000
}

// PyFields is Python str.split() without arguments.
func PyFields(s string) []string { return strings.FieldsFunc(s, isPySpace) }

// CollapseSpace is `" ".join(value.split())`: whitespace runs become one space, the ends are trimmed.
func CollapseSpace(s string) string { return strings.Join(PyFields(s), " ") }

// NormalizeName is analyze.py::normalize_name: display names lose repeated and surrounding whitespace; nil stays nil.
// The source records are never changed, only the exported copy.
func NormalizeName(s *string) *string {
	if s == nil {
		return nil
	}
	n := CollapseSpace(*s)
	return &n
}

// PyStrip is Python str.strip(): trims unicode whitespace on both ends.
func PyStrip(s string) string { return strings.TrimFunc(s, isPySpace) }
