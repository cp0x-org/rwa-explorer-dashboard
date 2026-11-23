package publish_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/curated"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/publish"
)

// The published build carries the curated fields: links and referrals judged at the publish time from the newest deals
// row, rights profiles judged at the UTC date of it, issuer websites from issuers.json; the warnings go to the run.
func TestPublishAttachesCuratedData(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	cfg, err := curated.Load(filepath.Join("..", "..", "config"))
	require.NoError(t, err)
	settings, err := model.LoadSettings(filepath.Join("..", "..", "config", "settings.json"))
	require.NoError(t, err)
	at := time.Date(2026, 11, 14, 3, 1, 51, 0, time.UTC)
	pub := publish.New(e.pool, settings, assets, slog.New(slog.DiscardHandler)).WithCurated(cfg).WithClock(func() time.Time { return at })

	// No deals yet: referrals are unavailable and every referral platform warns once.
	run1 := e.addRun(t, "r1", "refresh", 0, 1, 2)
	res, err := pub.Publish(ctx, run1, "refresh")
	require.NoError(t, err)
	exp := currentExport(t, e)
	assert.Equal(t, "2026-11-14T03:01:51Z", exp.GeneratedAt)
	assert.NotEmpty(t, exp.RightsSchema)
	assert.Nil(t, exp.Summary.LinksDealsFetchedAt)
	require.NotNil(t, exp.Summary.LinksDealsMaxAgeDays)
	assert.Equal(t, 14.0, *exp.Summary.LinksDealsMaxAgeDays)
	assert.Equal(t, 180, exp.Summary.RightsReviewValidDays)
	linked := 0
	for _, o := range exp.Opportunities {
		assert.Nil(t, o.Referral)
		if o.Link != nil {
			linked++
		}
	}
	assert.Positive(t, linked, "platform links do not depend on the deals")
	for _, w := range res.Warnings {
		assert.NotContains(t, w, "excluded", "the shipped rights profiles are valid")
	}
	assert.GreaterOrEqual(t, len(res.Warnings), exp.Summary.LinksWarningsCount+exp.Summary.RightsWarningsCount, "curated warnings are the run's warnings")
	assert.Positive(t, exp.Summary.LinksWarningsCount, "no deals: the referral platforms warn")
	var runWarnings []string
	var raw []byte
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT warnings FROM collection_runs WHERE id = $1`, run1).Scan(&raw))
	require.NoError(t, json.Unmarshal(raw, &runWarnings))
	assert.ElementsMatch(t, res.Warnings, runWarnings)

	// A fresh deals row (and a stale newer-by-id one is not preferred over the newest fetched_at).
	fresh := `{"fetched_at":"2026-11-14T01:07:38Z","deals":{"backpack":{"title":"Backpack","link":"https://backpack.exchange/refer/cp0x","bonus":"b","page_url":"https://cp0x.com/deals/backpack"},"hyperliquid":{"title":"HL","link":"https://app.hyperliquid.xyz/join/CP0X","bonus":null,"page_url":null}}}`
	_, err = e.st.InsertDeals(ctx, time.Date(2026, 11, 14, 1, 7, 38, 0, time.UTC), []byte(fresh))
	require.NoError(t, err)
	old := `{"fetched_at":"2026-01-01T00:00:00Z","deals":{}}`
	_, err = e.st.InsertDeals(ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), []byte(old))
	require.NoError(t, err)

	run2 := e.addRun(t, "r2", "rebuild")
	_, err = pub.Publish(ctx, run2, "rebuild")
	require.NoError(t, err)
	exp = currentExport(t, e)
	require.NotNil(t, exp.Summary.LinksDealsFetchedAt)
	assert.Equal(t, "2026-11-14T01:07:38Z", *exp.Summary.LinksDealsFetchedAt)
	var referrals int
	for _, is := range exp.Issuers {
		if is.Referral != nil {
			referrals++
			assert.Contains(t, []string{"backpack", "hyperliquid"}, is.Referral.Deal)
			assert.Equal(t, "2026-11-14T01:07:38Z", is.Referral.AsOf)
		}
	}
	t.Logf("issuers with a referral in this partial basket: %d", referrals)

	// The deals go stale with the publish clock: the same row 20 days later no longer yields referrals.
	later := publish.New(e.pool, settings, assets, slog.New(slog.DiscardHandler)).WithCurated(cfg).WithClock(func() time.Time { return at.AddDate(0, 0, 20) })
	run3 := e.addRun(t, "r3", "rebuild")
	_, err = later.Publish(ctx, run3, "rebuild")
	require.NoError(t, err)
	exp = currentExport(t, e)
	for _, o := range exp.Opportunities {
		assert.Nil(t, o.Referral)
	}
	for _, is := range exp.Issuers {
		assert.Nil(t, is.Referral)
	}
	assert.Equal(t, "2026-11-14T01:07:38Z", *exp.Summary.LinksDealsFetchedAt, "the timestamp is published even when stale")
}

func currentExport(t *testing.T, e *env) *model.Export {
	t.Helper()
	cur, err := e.st.CurrentBuild(context.Background())
	require.NoError(t, err)
	var exp model.Export
	require.NoError(t, json.Unmarshal(cur.Export, &exp))
	return &exp
}

// Without a curated config the build carries the fields, empty: no links, no profiles.
func TestPublishWithoutCuratedConfig(t *testing.T) {
	e := newEnv(t)
	run := e.addRun(t, "r1", "refresh", 0)
	_, err := e.pub.Publish(context.Background(), run, "refresh")
	require.NoError(t, err)
	exp := currentExport(t, e)
	assert.NotNil(t, exp.RightsProfiles)
	assert.Empty(t, exp.RightsProfiles)
	assert.Nil(t, exp.Summary.LinksDealsMaxAgeDays)
	for _, o := range exp.Opportunities {
		assert.Nil(t, o.Link)
	}
}

// The issuer website override of issuers.json decorates the export only: the issuers table keeps what CMC said (here
// nothing), so changing or removing an override takes effect on the next publish.
func TestIssuerWebsiteOverrideNeverReachesTheStoredIssuers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	settings, err := model.LoadSettings(filepath.Join("..", "..", "config", "settings.json"))
	require.NoError(t, err)
	run := e.addRun(t, "r1", "refresh", 0, 1)
	probe, err := e.pub.Publish(ctx, run, "refresh")
	require.NoError(t, err)
	require.NotZero(t, probe.BuildID)
	exp := currentExport(t, e)
	var ids []string
	for _, is := range exp.Issuers {
		if is.Website == nil || *is.Website == "" {
			ids = append(ids, is.ID)
		}
	}
	require.NotEmpty(t, ids, "the 1.x snapshots have issuers without a CMC website")

	publishWith := func(key, site string) *model.Export {
		t.Helper()
		cfg := &curated.Config{IssuerWebsites: map[string]string{}}
		if site != "" {
			for _, id := range ids {
				cfg.IssuerWebsites[id] = site
			}
		}
		pub := publish.New(e.pool, settings, assets, slog.New(slog.DiscardHandler)).WithCurated(cfg)
		_, err := pub.Publish(ctx, e.addRun(t, key, "rebuild"), "rebuild")
		require.NoError(t, err)
		return currentExport(t, e)
	}
	byID := func(x *model.Export) map[string]*string {
		out := map[string]*string{}
		for _, is := range x.Issuers {
			out[is.ID] = is.Website
		}
		return out
	}
	stored := func() int {
		return e.count(t, `SELECT count(*) FROM issuers WHERE website LIKE 'https://override-%'`)
	}

	first := byID(publishWith("r2", "https://override-one.example"))
	second := byID(publishWith("r3", "https://override-two.example"))
	for _, id := range ids {
		require.NotNil(t, first[id])
		assert.Equal(t, "https://override-one.example", *first[id])
		require.NotNil(t, second[id])
		assert.Equal(t, "https://override-two.example", *second[id], "a changed override applies without a CMC refresh")
	}
	assert.Zero(t, stored(), "issuers.website never holds an override")
	third := byID(publishWith("r4", ""))
	for _, id := range ids {
		assert.Nil(t, third[id], "a removed override disappears from the export")
	}
	assert.Zero(t, stored())
}

// Excluded rights profiles are counted in the summary and leave one run warning; the publish itself succeeds.
func TestExcludedRightsProfilesLeaveARunWarning(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	settings, err := model.LoadSettings(filepath.Join("..", "..", "config", "settings.json"))
	require.NoError(t, err)
	rights, err := curated.ParseRights("rights.json", []byte(`{"schema_version":1,"review_valid_days":180,"profiles":["x","y"]}`))
	require.NoError(t, err)
	pub := publish.New(e.pool, settings, assets, slog.New(slog.DiscardHandler)).WithCurated(&curated.Config{Rights: rights})
	run := e.addRun(t, "r1", "refresh", 0)
	res, err := pub.Publish(ctx, run, "refresh")
	require.NoError(t, err)
	assert.Equal(t, 2, res.Summary.RightsExcludedProfilesCount)
	assert.Contains(t, res.Warnings, "2 rights profiles excluded (see journal)")
	var raw []byte
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT warnings FROM collection_runs WHERE id = $1`, run).Scan(&raw))
	assert.Contains(t, string(raw), "2 rights profiles excluded (see journal)")
	var status string
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT status FROM collection_runs WHERE id = $1`, run).Scan(&status))
	assert.NotEqual(t, "failed", status)
}

// The collector loads the curated config once and reuses it for every publish: publishing must not change it.
func TestPublishLeavesTheCuratedConfigUnchanged(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	dir := filepath.Join("..", "..", "config")
	cfg, err := curated.Load(dir)
	require.NoError(t, err)
	pristine, err := curated.Load(dir)
	require.NoError(t, err)
	settings, err := model.LoadSettings(filepath.Join(dir, "settings.json"))
	require.NoError(t, err)
	_, err = e.st.InsertDeals(ctx, time.Date(2026, 11, 14, 1, 7, 38, 0, time.UTC),
		[]byte(`{"fetched_at":"2026-11-14T01:07:38Z","deals":{"backpack":{"title":"B","link":"https://backpack.exchange/refer/cp0x","bonus":null,"page_url":null}}}`))
	require.NoError(t, err)
	pub := publish.New(e.pool, settings, assets, slog.New(slog.DiscardHandler)).WithCurated(cfg).
		WithClock(func() time.Time { return time.Date(2026, 11, 14, 3, 0, 0, 0, time.UTC) })
	for i, key := range []string{"r1", "r2"} {
		_, err := pub.Publish(ctx, e.addRun(t, key, "refresh", 0, 1), "refresh")
		require.NoError(t, err, i)
		require.Equal(t, pristine, cfg, "after publish %d", i+1)
	}
}
