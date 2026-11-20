package publish_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/model"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/publish"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/runlog"
)

func TestPublishAndRebuildJournal(t *testing.T) {
	e := newEnv(t)
	h := runlog.New(slog.NewTextHandler(io.Discard, nil), runlog.Options{})
	h.SetSink(e.st)
	t.Cleanup(h.Close)
	settings, err := model.LoadSettings(filepath.Join("..", "..", "config", "settings.json"))
	require.NoError(t, err)
	pub := publish.New(e.pool, settings, assets, slog.New(h))
	ctx := context.Background()

	run := e.addRun(t, "r1", "refresh", 0, 1)
	res, err := pub.Publish(ctx, run, "refresh")
	require.NoError(t, err)

	// Publish flushes itself: no waiting for the timer.
	events, _, err := e.st.ListRunEvents(ctx, run, 0, "debug", 100)
	require.NoError(t, err)
	require.Len(t, events, 2+len(res.Warnings))
	assert.Equal(t, "publish started", events[0].Message)
	last := events[len(events)-1]
	assert.Equal(t, "build published", last.Message)
	for _, ev := range events {
		assert.Equal(t, "publish", ev.Step)
	}
	assert.Contains(t, string(last.Attrs), "underlyings")

	// A rebuild has its own run with start, publish and finish events.
	rebuildRun, _, err := pub.Rebuild(ctx, publish.RebuildParams{Trigger: "cli", CollectorVersion: "test", SettingsJSON: []byte("{}")})
	require.NoError(t, err)
	rb, _, err := e.st.ListRunEvents(ctx, rebuildRun, 0, "info", 100)
	require.NoError(t, err)
	var msgs []string
	for _, ev := range rb {
		msgs = append(msgs, ev.Message)
	}
	require.GreaterOrEqual(t, len(msgs), 4)
	assert.Equal(t, "rebuild started", msgs[0])
	assert.Equal(t, "publish started", msgs[1])
	assert.Equal(t, "run finished", msgs[len(msgs)-2])
	assert.Equal(t, "journal closed", msgs[len(msgs)-1])
}
