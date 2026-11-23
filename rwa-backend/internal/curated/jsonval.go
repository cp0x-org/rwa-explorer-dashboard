// Package curated is the port of the curated data rules of the 1.x pipeline: outbound links and cp0x referrals
// (curated.py), reviewed holder rights (rights.py), issuer website overrides (export.py) and the cp0x deals cache
// (deals.py). Everything here is pure: configs are parsed from bytes or files, `now` and `today` are parameters.
//
// The config files are read into a generic ordered JSON value (Obj, []any, string, json.Number, bool, nil) instead
// of structs: the validation rules of the Python loaders are written against loosely typed dicts, and rights
// profiles are published as copies whose key order is the one of the file.
package curated

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ConfigError is an invalid curated config (the Python ConfigError). Its text is the message.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func configErrorf(format string, args ...any) error {
	return &ConfigError{Msg: fmt.Sprintf(format, args...)}
}

// Obj is a JSON object that keeps its key order (Python dict).
type Obj struct {
	keys []string
	m    map[string]any
}

func newObj() *Obj { return &Obj{m: map[string]any{}} }

// Get is dict.get: nil when the key is absent (or null).
func (o *Obj) Get(k string) any { return o.m[k] }

// Has is `k in dict`.
func (o *Obj) Has(k string) bool { _, ok := o.m[k]; return ok }

// Keys returns the keys in file order.
func (o *Obj) Keys() []string { return o.keys }

// Len returns the number of keys.
func (o *Obj) Len() int { return len(o.keys) }

// Set adds or replaces a key; an existing key keeps its position, like a Python dict.
func (o *Obj) Set(k string, v any) {
	if _, ok := o.m[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.m[k] = v
}

// MarshalJSON writes the keys in order.
func (o *Obj) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, fmt.Errorf("curated: marshal key: %w", err)
		}
		vb, err := json.Marshal(o.m[k])
		if err != nil {
			return nil, fmt.Errorf("curated: marshal %q: %w", k, err)
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// parseJSON decodes one JSON document into Obj / []any / string / json.Number / bool / nil, rejecting trailing data.
// Numbers stay literal so that 1 (a Python int) and 1.0 (a float) remain distinguishable.
func parseJSON(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	v, err := readValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected data after the JSON document")
	}
	return v, nil
}

func readValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return tok, nil // string, json.Number, bool, nil
	}
	switch d {
	case '{':
		o := newObj()
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			k, _ := kt.(string)
			v, err := readValue(dec)
			if err != nil {
				return nil, err
			}
			o.Set(k, v)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return o, nil
	case '[':
		list := []any{}
		for dec.More() {
			v, err := readValue(dec)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return list, nil
	}
	return nil, fmt.Errorf("unexpected delimiter %q", d)
}

// deepCopy copies a parsed value (Python copy.deepcopy).
func deepCopy(v any) any {
	switch x := v.(type) {
	case *Obj:
		c := newObj()
		for _, k := range x.keys {
			c.Set(k, deepCopy(x.m[k]))
		}
		return c
	case []any:
		c := make([]any, len(x))
		for i, e := range x {
			c[i] = deepCopy(e)
		}
		return c
	}
	return v
}

func asObj(v any) (*Obj, bool) { o, ok := v.(*Obj); return o, ok }

func asList(v any) ([]any, bool) { l, ok := v.([]any); return l, ok }

// asStr is `isinstance(v, str)`: the string and whether it is one.
func asStr(v any) (string, bool) { s, ok := v.(string); return s, ok }

// isInt is Python `type(v) is int` for a parsed JSON value: an integer literal (no fraction or exponent, no bool).
func isInt(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := strconv.ParseInt(string(n), 10, 64)
	if err != nil {
		return 0, false
	}
	return i, true
}

// isNumber is `isinstance(v, (int, float)) and not bool`.
func isNumber(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	return f, err == nil
}

// isOne is Python's `v == 1`: the number 1 (1.0 too) and, because True == 1, the boolean true.
func isOne(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	n, ok := isNumber(v)
	return ok && n == 1
}

// truthy is Python bool(v) for parsed values.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case bool:
		return x
	case json.Number:
		f, err := x.Float64()
		return err != nil || f != 0
	case []any:
		return len(x) > 0
	case *Obj:
		return x.Len() > 0
	}
	return true
}

// pyRepr is repr() of a Python str: single quotes unless the text has a single and no double quote.
func pyRepr(s string) string {
	q := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = '"'
	}
	var b strings.Builder
	b.WriteByte(q)
	for _, r := range s {
		switch {
		case r == rune(q) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(q)
	return b.String()
}
