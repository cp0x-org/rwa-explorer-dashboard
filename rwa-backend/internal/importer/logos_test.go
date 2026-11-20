package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/store"
	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/testdb"
)

func TestImportLogosKeepsExtensionCaseAndUnknownTypes(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	ctx := context.Background()
	dir := t.TempDir()
	tokens := filepath.Join(dir, "logos", "tokens")
	require.NoError(t, os.MkdirAll(tokens, 0o755))
	for name, content := range map[string]string{"1.PNG": "png", "2.svg": "svg", "3.avif": "avif", ".hidden": "x", "noext": "x"} {
		require.NoError(t, os.WriteFile(filepath.Join(tokens, name), []byte(content), 0o644))
	}
	n, err := importLogos(ctx, store.New(pool), dir)
	require.NoError(t, err)
	require.Equal(t, 3, n, "dotfiles and files without an extension are skipped, unknown extensions are not")

	st := store.New(pool)
	for id, want := range map[string][2]string{"1": {"PNG", "image/png"}, "2": {"svg", "image/svg+xml"}, "3": {"avif", "application/octet-stream"}} {
		l, err := st.GetLogo(ctx, "tokens", id)
		require.NoError(t, err)
		require.Equal(t, want[0], l.Ext, id)
		require.Equal(t, want[1], l.ContentType, id)
	}
}
