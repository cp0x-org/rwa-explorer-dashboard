package model

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// OrderedMap is a JSON object whose key order is significant. The 1.x export writes breakdowns sorted by
// (-eligible, -raw, key) and the frontend iterates them in file order, so a plain Go map would lose information.
type OrderedMap[V any] struct {
	Keys   []string
	Values map[string]V
}

// Set appends or replaces a key, keeping first-insertion order.
func (m *OrderedMap[V]) Set(k string, v V) {
	if m.Values == nil {
		m.Values = map[string]V{}
	}
	if _, ok := m.Values[k]; !ok {
		m.Keys = append(m.Keys, k)
	}
	m.Values[k] = v
}

// Get returns the value for k.
func (m OrderedMap[V]) Get(k string) (V, bool) {
	v, ok := m.Values[k]
	return v, ok
}

// Len returns the number of keys.
func (m OrderedMap[V]) Len() int { return len(m.Keys) }

// MarshalJSON writes keys in order; an empty map is {}, never null.
func (m OrderedMap[V]) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range m.Keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		vb, err := json.Marshal(m.Values[k])
		if err != nil {
			return nil, fmt.Errorf("model: ordered map key %q: %w", k, err)
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalJSON reads an object preserving key order. null yields an empty map.
func (m *OrderedMap[V]) UnmarshalJSON(b []byte) error {
	m.Keys, m.Values = nil, map[string]V{}
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	t, err := dec.Token()
	if err != nil {
		return fmt.Errorf("model: ordered map: %w", err)
	}
	if t != json.Delim('{') {
		return fmt.Errorf("model: ordered map: expected object")
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return fmt.Errorf("model: ordered map: %w", err)
		}
		k, _ := kt.(string)
		var v V
		if err := dec.Decode(&v); err != nil {
			return fmt.Errorf("model: ordered map key %q: %w", k, err)
		}
		m.Set(k, v)
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("model: ordered map: %w", err)
	}
	return nil
}
