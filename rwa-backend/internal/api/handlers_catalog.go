package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
)

const (
	defaultPageLimit = 100
	maxPageLimit     = 1000
)

func (s *Server) handleUnderlyings(w http.ResponseWriter, r *http.Request, sn *snapshot) {
	writeJSON(w, r, http.StatusOK, map[string]any{"items": sn.export.Underlyings})
}

func (s *Server) handleUnderlying(w http.ResponseWriter, r *http.Request, sn *snapshot) {
	key := r.PathValue("idOrTicker")
	u := sn.underlying(key)
	if u == nil {
		writeError(w, r, http.StatusNotFound, codeNotFound, fmt.Sprintf("underlying %q is not in the current build", key))
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{
		"underlying":      u,
		"tokens":          sn.tokens(u.TokenIDs),
		"excluded_tokens": sn.tokens(u.ExcludedTokenIDs),
	})
}

// tokens resolves ids to tokens (unknown ids are skipped); never nil.
func (s *snapshot) tokens(ids []int64) []*model.Token {
	out := make([]*model.Token, 0, len(ids))
	for _, id := range ids {
		if t := s.tokenByID[id]; t != nil {
			out = append(out, t)
		}
	}
	return out
}

func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request, sn *snapshot) {
	q := r.URL.Query()
	var underlyingID string
	if v := q.Get("underlying"); v != "" {
		if u := sn.underlying(v); u != nil {
			underlyingID = u.ID
		} else {
			underlyingID = "\x00" // matches nothing
		}
	}
	issuer, kind := q.Get("issuer"), q.Get("kind")
	inDefi := false
	if v := q.Get("in_defi"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, codeBadRequest, "in_defi must be true or false")
			return
		}
		inDefi = b
	}
	items := make([]model.Token, 0)
	for _, t := range sn.export.Tokens {
		if underlyingID != "" && t.UnderlyingID != underlyingID {
			continue
		}
		if issuer != "" && (t.IssuerID == nil || *t.IssuerID != issuer) {
			continue
		}
		if kind != "" && t.Kind != kind {
			continue
		}
		if inDefi && t.Counts.EligibleOpportunitiesCount == 0 {
			continue
		}
		items = append(items, t)
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type tokenOpportunity struct {
	Opportunity *model.Opportunity `json:"opportunity"`
	Relation    *model.Relation    `json:"relation"`
}

func validateTokenID(w http.ResponseWriter, r *http.Request) bool {
	if _, err := strconv.ParseInt(r.PathValue("id"), 10, 64); err != nil {
		writeError(w, r, http.StatusBadRequest, codeBadRequest, "token id must be an integer")
		return false
	}
	return true
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request, sn *snapshot) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, codeBadRequest, "token id must be an integer")
		return
	}
	t := sn.tokenByID[id]
	if t == nil {
		writeError(w, r, http.StatusNotFound, codeNotFound, fmt.Sprintf("token %d is not in the current build", id))
		return
	}
	var issuer *model.Issuer
	if t.IssuerID != nil {
		issuer = sn.issuerByID[*t.IssuerID]
	}
	opps := make([]tokenOpportunity, 0, len(t.Relations)+len(t.UnconfirmedKeys))
	for i := range t.Relations {
		if o := sn.oppByKey[t.Relations[i].Key]; o != nil {
			opps = append(opps, tokenOpportunity{o, &t.Relations[i]})
		}
	}
	for _, k := range t.UnconfirmedKeys {
		if o := sn.oppByKey[k]; o != nil {
			opps = append(opps, tokenOpportunity{o, nil})
		}
	}
	writeJSON(w, r, http.StatusOK, map[string]any{
		"token": t, "underlying": sn.underlyingByID[t.UnderlyingID], "issuer": issuer, "opportunities": opps,
	})
}

func (s *Server) handleIssuers(w http.ResponseWriter, r *http.Request, sn *snapshot) {
	writeJSON(w, r, http.StatusOK, map[string]any{"items": sn.export.Issuers})
}

func (s *Server) handleIssuer(w http.ResponseWriter, r *http.Request, sn *snapshot) {
	id := r.PathValue("id")
	is := sn.issuerByID[id]
	if is == nil {
		writeError(w, r, http.StatusNotFound, codeNotFound, fmt.Sprintf("issuer %q is not in the current build", id))
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"issuer": is, "tokens": sn.tokens(is.TokenIDs)})
}

type oppItem struct {
	TokenID      int64              `json:"token_id"`
	UnderlyingID string             `json:"underlying_id"`
	Opportunity  *model.Opportunity `json:"opportunity"`
	Relation     *model.Relation    `json:"relation"`
}

// encodeCursor makes the opaque cursor of the last returned (key, token id).
func encodeCursor(key string, tokenID int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(tokenID, 10) + "\n" + key))
}

var errBadCursor = errors.New("invalid cursor")

func decodeCursor(c string) (key string, tokenID int64, err error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return "", 0, errBadCursor
	}
	idStr, key, ok := strings.Cut(string(b), "\n")
	if !ok || key == "" {
		return "", 0, errBadCursor
	}
	tokenID, err = strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return "", 0, errBadCursor
	}
	return key, tokenID, nil
}

func (s *Server) handleOpportunities(w http.ResponseWriter, r *http.Request, sn *snapshot) {
	q := r.URL.Query()
	limit, ok := parseLimit(w, r, defaultPageLimit, maxPageLimit)
	if !ok {
		return
	}
	link := q.Get("link")
	switch link {
	case "", "direct", "indirect", "unconfirmed":
	default:
		writeError(w, r, http.StatusBadRequest, codeBadRequest, "link must be direct, indirect or unconfirmed")
		return
	}
	underlyingID := q.Get("underlying")
	if underlyingID != "" {
		if u := sn.underlying(underlyingID); u != nil {
			underlyingID = u.ID
		}
	}
	issuer, network, category, platform, eligibility := q.Get("issuer"), q.Get("network"), q.Get("category"), q.Get("platform"), q.Get("eligibility")

	start := 0
	if c := q.Get("cursor"); c != "" {
		key, tid, err := decodeCursor(c)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, codeBadRequest, "cursor is invalid")
			return
		}
		// First row strictly after (key, tid).
		start = sort.Search(len(sn.rows), func(i int) bool { return rowAfter(sn.rows[i], key, tid) })
	}
	items := make([]oppItem, 0, min(limit, len(sn.rows)))
	var next *string
	for i := start; i < len(sn.rows); i++ {
		row := &sn.rows[i]
		if underlyingID != "" && row.underlyingID != underlyingID {
			continue
		}
		if issuer != "" && row.issuerID != issuer {
			continue
		}
		o := row.opp
		if (network != "" && o.Network != network) || (category != "" && o.Category != category) ||
			(platform != "" && o.Platform != platform) || (eligibility != "" && o.Eligibility != eligibility) {
			continue
		}
		switch link {
		case "direct":
			if row.rel == nil || row.rel.Depth != 1 {
				continue
			}
		case "indirect":
			if row.rel == nil || row.rel.Depth <= 1 {
				continue
			}
		case "unconfirmed":
			if row.rel != nil {
				continue
			}
		}
		if len(items) == limit {
			last := items[len(items)-1]
			c := encodeCursor(last.Opportunity.Key, last.TokenID)
			next = &c
			break
		}
		items = append(items, oppItem{row.tokenID, row.underlyingID, o, row.rel})
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"items": items, "next_cursor": next})
}

// parseLimit reads ?limit=; on an invalid value it writes the 400 itself and returns false.
func parseLimit(w http.ResponseWriter, r *http.Request, def, maxLimit int) (int, bool) {
	v := r.URL.Query().Get("limit")
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > maxLimit {
		writeError(w, r, http.StatusBadRequest, codeBadRequest, fmt.Sprintf("limit must be an integer between 1 and %d", maxLimit))
		return 0, false
	}
	return n, true
}
