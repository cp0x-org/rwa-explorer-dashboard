package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"DATABASE_URL", "CMC_API_KEY", "PORTALS_API_KEY", "ASSETS_PATH", "SETTINGS_PATH",
		"COLLECT_CRON", "COLLECT_INTERVAL", "COLLECT_ON_START", "SNAPSHOT_RETENTION_DAYS", "RUN_TIMEOUT", "RAW_CAPTURE_DIR",
		"HTTP_ADDR", "CORS_ORIGINS", "ADMIN_TOKEN", "LOG_LEVEL", "LOG_FORMAT", "ENV_FILE"} {
		t.Setenv(k, "") // registers restore of the original value
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
}

func mustLoad(t *testing.T) *Config {
	t.Helper()
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDefaults(t *testing.T) {
	clearEnv(t)
	c := mustLoad(t)
	if c.AssetsPath != "/config/assets.json" || c.SettingsPath != "/config/settings.json" ||
		c.CollectCron != "0 6 * * *" || !c.CollectOnStart || c.SnapshotRetentionDays != 90 ||
		c.HTTPAddr != ":8080" || c.LogLevel != "info" || c.LogFormat != "json" || c.CollectInterval != 0 || c.RunTimeout != 2*time.Hour {
		t.Errorf("unexpected defaults: %s", c)
	}
}

func TestValidate(t *testing.T) {
	clearEnv(t)
	c := mustLoad(t)
	if err := c.ValidateAPI(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("api: %v", err)
	}
	err := c.ValidateCollector()
	for _, want := range []string{"DATABASE_URL", "CMC_API_KEY", "PORTALS_API_KEY"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("collector missing %s: %v", want, err)
		}
	}

	t.Setenv("DATABASE_URL", "postgres://u:p@h/db")
	if err := mustLoad(t).ValidateAPI(); err != nil {
		t.Errorf("api with only DATABASE_URL: %v", err)
	}
	t.Setenv("CMC_API_KEY", "a")
	t.Setenv("PORTALS_API_KEY", "b")
	if err := mustLoad(t).ValidateCollector(); err != nil {
		t.Errorf("defaults should be valid: %v", err)
	}
	t.Setenv("COLLECT_CRON", "0 1 * * *")
	if err := mustLoad(t).ValidateCollector(); err != nil {
		t.Errorf("explicit cron alone should be valid: %v", err)
	}
	os.Unsetenv("COLLECT_CRON")

	t.Setenv("COLLECT_INTERVAL", "12h")
	c = mustLoad(t)
	if err := c.ValidateCollector(); err != nil {
		t.Errorf("interval only (default cron) should be valid: %v", err)
	}
	if c.CollectInterval != 12*time.Hour {
		t.Errorf("interval = %v", c.CollectInterval)
	}
	t.Setenv("COLLECT_CRON", "0 1 * * *")
	if err := mustLoad(t).ValidateCollector(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("both set: %v", err)
	}
	os.Unsetenv("COLLECT_CRON")

	t.Setenv("COLLECT_INTERVAL", "-1h")
	if err := mustLoad(t).ValidateCollector(); err == nil || !strings.Contains(err.Error(), "COLLECT_INTERVAL") {
		t.Errorf("negative interval: %v", err)
	}
	os.Unsetenv("COLLECT_INTERVAL")
	t.Setenv("SNAPSHOT_RETENTION_DAYS", "0")
	if err := mustLoad(t).ValidateCollector(); err == nil || !strings.Contains(err.Error(), "SNAPSHOT_RETENTION_DAYS") {
		t.Errorf("retention 0: %v", err)
	}
	os.Unsetenv("SNAPSHOT_RETENTION_DAYS")
	t.Setenv("RUN_TIMEOUT", "30m")
	if c := mustLoad(t); c.RunTimeout != 30*time.Minute || !strings.Contains(c.String(), "RunTimeout:30m0s") {
		t.Errorf("RUN_TIMEOUT=30m: %s", c)
	}
	t.Setenv("RUN_TIMEOUT", "0s")
	if err := mustLoad(t).ValidateCollector(); err == nil || !strings.Contains(err.Error(), "RUN_TIMEOUT") {
		t.Errorf("run timeout 0: %v", err)
	}
}

func TestValidateLog(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("LOG_LEVEL", "verbose")
	t.Setenv("LOG_FORMAT", "xml")
	err := mustLoad(t).ValidateAPI()
	if err == nil || !strings.Contains(err.Error(), "LOG_LEVEL") || !strings.Contains(err.Error(), "LOG_FORMAT") {
		t.Errorf("got %v", err)
	}
}

func TestValidateSources(t *testing.T) {
	clearEnv(t)
	t.Setenv("CMC_API_KEY", "k")
	c := mustLoad(t)
	if err := c.ValidateSources("cmc"); err != nil {
		t.Errorf("cmc-only probe needs only CMC_API_KEY: %v", err)
	}
	if err := c.ValidateSources("portals"); err == nil || !strings.Contains(err.Error(), "PORTALS_API_KEY") {
		t.Errorf("portals: %v", err)
	}
	if err := c.ValidateSources(""); err == nil || !strings.Contains(err.Error(), "PORTALS_API_KEY") {
		t.Errorf("both: %v", err)
	}
	if err := c.ValidateSources("nope"); err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("unknown: %v", err)
	}
	t.Setenv("PORTALS_API_KEY", "k")
	if err := mustLoad(t).ValidateSources(""); err != nil {
		t.Errorf("both keys: %v", err)
	}
}

func TestBearerStripped(t *testing.T) {
	for in, want := range map[string]string{
		"Bearer tok": "tok", "bearer tok": "tok", "BEARER   tok": "tok", "Bearer\ttok": "tok",
		"tok": "tok", "Bearer": "", "  Bearer  ": "",
	} {
		clearEnv(t)
		t.Setenv("PORTALS_API_KEY", in)
		if got := mustLoad(t).PortalsAPIKey; got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestCORSTrimmed(t *testing.T) {
	clearEnv(t)
	t.Setenv("CORS_ORIGINS", " http://a , http://b,, ")
	got := mustLoad(t).CORSOrigins
	if len(got) != 2 || got[0] != "http://a" || got[1] != "http://b" {
		t.Errorf("got %q", got)
	}
}

func TestStringMasksSecrets(t *testing.T) {
	clearEnv(t)
	secrets := []string{"s3cretpass", "cmc-secret-key", "portals-secret-key", "admin-secret-token"}
	t.Setenv("DATABASE_URL", "postgres://rwa:"+secrets[0]+"@localhost:5433/rwa")
	t.Setenv("CMC_API_KEY", secrets[1])
	t.Setenv("PORTALS_API_KEY", "Bearer "+secrets[2])
	t.Setenv("ADMIN_TOKEN", secrets[3])
	c := mustLoad(t)

	var jb, tb bytes.Buffer
	slog.New(slog.NewJSONHandler(&jb, nil)).Info("cfg", "config", c)
	slog.New(slog.NewJSONHandler(&jb, nil)).Info("cfg", "config", *c)
	slog.New(slog.NewTextHandler(&tb, nil)).Info("cfg", "config", c)
	slog.New(slog.NewTextHandler(&tb, nil)).Info("cfg", "config", *c)
	js, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	outs := map[string]string{
		"String":     c.String(),
		"%+v ptr":    fmt.Sprintf("%+v", c),
		"%+v value":  fmt.Sprintf("%+v", *c),
		"%v ptr":     fmt.Sprintf("%v", c),
		"%v value":   fmt.Sprintf("%v", *c),
		"slog json":  jb.String(),
		"slog text":  tb.String(),
		"json.Marsh": string(js),
	}
	for name, out := range outs {
		for _, sec := range secrets {
			if strings.Contains(out, sec) {
				t.Errorf("%s leaks %q: %s", name, sec, out)
			}
		}
	}
	if s := c.String(); !strings.Contains(s, "***") || !strings.Contains(s, "localhost:5433") {
		t.Errorf("unexpected: %s", s)
	}
}

func TestMaskDSN(t *testing.T) {
	for _, in := range []string{
		"postgres://u:SECRET@h:5432/db?sslmode=disable",
		"postgresql://u:SECRET@h/db",
		"postgres://u@h/db?password=SECRET",
		"postgres://u@h/db?sslpassword=SECRET&sslmode=disable",
		"postgres://SECRET:SECRET@h/db",
		"host=h user=u password=SECRET dbname=rwa",
		"not a url SECRET",
		"%zz SECRET",
	} {
		out := maskDSN(in)
		if strings.Contains(out, "SECRET") && !strings.HasPrefix(in, "postgres://SECRET:") {
			t.Errorf("maskDSN(%q) leaks: %s", in, out)
		}
	}
	if got := maskDSN("host=h password=SECRET"); got != "***" {
		t.Errorf("keyword dsn: %q", got)
	}
	if got := maskDSN("postgres://u:p@h:5432/db?sslmode=disable"); got != "postgres://u:***@h:5432/db?sslmode=disable" {
		t.Errorf("url form: %q", got)
	}
	if got := maskDSN("postgres://u@h/db?password=SECRET"); got != "postgres://u@h/db?password=***" {
		t.Errorf("query password: %q", got)
	}
	// username equal to the old sentinel must not break masking
	if got := maskDSN("postgres://xxxxx:SECRET@h/db"); strings.Contains(got, "SECRET") {
		t.Errorf("sentinel username: %q", got)
	}
}

func writeEnvFile(t *testing.T, body string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "env")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV_FILE", p)
}

func TestLoadDotEnv(t *testing.T) {
	clearEnv(t)
	keys := []string{"CMC_API_KEY", "LOG_LEVEL", "HTTP_ADDR", "ADMIN_TOKEN", "RAW_CAPTURE_DIR", "ASSETS_PATH", "SETTINGS_PATH"}
	writeEnvFile(t, "\xef\xbb\xbf# c\nCMC_API_KEY=fromfile # trailing\nLOG_LEVEL=\"debug\"\nexport\tHTTP_ADDR=:9\n"+
		"ADMIN_TOKEN='a # b'\nRAW_CAPTURE_DIR=/tmp/x#y\nASSETS_PATH=\n=novalue\nbad line\n")
	t.Setenv("HTTP_ADDR", ":1") // already set: must win
	t.Cleanup(func() {
		for _, k := range keys {
			_ = os.Unsetenv(k)
		}
	})
	if err := LoadDotEnv(); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"CMC_API_KEY": "fromfile", "LOG_LEVEL": "debug", "HTTP_ADDR": ":1",
		"ADMIN_TOKEN": "a # b", "RAW_CAPTURE_DIR": "/tmp/x#y", "ASSETS_PATH": "",
	}
	for k, v := range want {
		if got := os.Getenv(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if _, ok := os.LookupEnv(""); ok {
		t.Error("empty key set")
	}
}

func TestLoadDotEnvMissing(t *testing.T) {
	clearEnv(t)
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "missing"))
	if err := LoadDotEnv(); err != nil {
		t.Errorf("missing file: %v", err)
	}
}
