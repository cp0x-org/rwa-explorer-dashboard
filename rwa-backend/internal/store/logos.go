package store

import (
	"context"
	"fmt"
	"time"
)

// Logo is a logos row.
type Logo struct {
	Kind        string // tokens | issuers | underlyings
	EntityID    string
	Ext         string
	ContentType string
	Bytes       []byte
	SHA256      string
	Source      string
	FetchedAt   time.Time
}

// InsertLogoIfAbsent stores a logo unless one exists for (kind, entity_id); an existing logo is never
// overwritten. It reports whether a row was inserted.
func (s *Store) InsertLogoIfAbsent(ctx context.Context, l Logo) (bool, error) {
	tag, err := s.q.Exec(ctx, `INSERT INTO logos (kind, entity_id, ext, content_type, bytes, sha256, source)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (kind, entity_id) DO NOTHING`,
		l.Kind, l.EntityID, l.Ext, l.ContentType, l.Bytes, l.SHA256, l.Source)
	if err != nil {
		return false, fmt.Errorf("store: insert logo %s/%s: %w", l.Kind, l.EntityID, err)
	}
	return tag.RowsAffected() == 1, nil
}

// GetLogo returns a logo or ErrNotFound.
func (s *Store) GetLogo(ctx context.Context, kind, entityID string) (*Logo, error) {
	var l Logo
	err := s.q.QueryRow(ctx, `SELECT kind, entity_id, ext, content_type, bytes, sha256, source, fetched_at
		FROM logos WHERE kind = $1 AND entity_id = $2`, kind, entityID).
		Scan(&l.Kind, &l.EntityID, &l.Ext, &l.ContentType, &l.Bytes, &l.SHA256, &l.Source, &l.FetchedAt)
	if err != nil {
		return nil, fmt.Errorf("store: get logo %s/%s: %w", kind, entityID, notFound(err))
	}
	return &l, nil
}

// HasLogo reports whether a logo exists.
func (s *Store) HasLogo(ctx context.Context, kind, entityID string) (bool, error) {
	var ok bool
	if err := s.q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM logos WHERE kind = $1 AND entity_id = $2)`, kind, entityID).Scan(&ok); err != nil {
		return false, fmt.Errorf("store: has logo %s/%s: %w", kind, entityID, err)
	}
	return ok, nil
}
