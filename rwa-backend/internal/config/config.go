// Package config loads the service configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config is the process configuration. Secret fields are excluded from JSON, and
// String/LogValue mask them, so the config is safe to log.
type Config struct {
	DatabaseURL   string `env:"DATABASE_URL" json:"-"`
	CMCAPIKey     string `env:"CMC_API_KEY" json:"-"`
	PortalsAPIKey string `env:"PORTALS_API_KEY" json:"-"`

	AssetsPath   string `env:"ASSETS_PATH" envDefault:"/config/assets.json"`
	SettingsPath string `env:"SETTINGS_PATH" envDefault:"/config/settings.json"`

	CollectCron           string        `env:"COLLECT_CRON" envDefault:"0 6 * * *"`
	CollectInterval       time.Duration `env:"COLLECT_INTERVAL"`
	CollectOnStart        bool          `env:"COLLECT_ON_START" envDefault:"true"`
	SnapshotRetentionDays int           `env:"SNAPSHOT_RETENTION_DAYS" envDefault:"90"`
	RunTimeout            time.Duration `env:"RUN_TIMEOUT" envDefault:"2h"`
	RawCaptureDir         string        `env:"RAW_CAPTURE_DIR"`
	MigrateOnStart        bool          `env:"MIGRATE_ON_START" envDefault:"true"`

	HTTPAddr    string   `env:"HTTP_ADDR" envDefault:":8080"`
	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:","`
	AdminToken  string   `env:"ADMIN_TOKEN" json:"-"`

	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"`

	// cronExplicit records whether COLLECT_CRON was set by the operator: it has a
	// default, which must not conflict with COLLECT_INTERVAL.
	cronExplicit bool
}

// Load reads the configuration from environment variables.
func Load() (*Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	c.PortalsAPIKey = stripBearer(c.PortalsAPIKey)
	c.cronExplicit = os.Getenv("COLLECT_CRON") != ""
	origins := c.CORSOrigins[:0]
	for _, o := range c.CORSOrigins {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	c.CORSOrigins = origins
	return &c, nil
}

// stripBearer removes a leading "Bearer" scheme (case-insensitive, any whitespace) from a key.
func stripBearer(s string) string {
	f := strings.Fields(s)
	if len(f) > 0 && strings.EqualFold(f[0], "bearer") {
		f = f[1:]
	}
	return strings.Join(f, " ")
}

// ValidateAPI checks the settings the api role needs.
func (c *Config) ValidateAPI() error {
	errs := c.validateCommon()
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	return errors.Join(errs...)
}

// ValidateSources checks what the source clients need (logging and the API key of the given provider:
// "cmc", "portals" or "" for both), without a database; used by the dev-only `probe` command.
func (c *Config) ValidateSources(provider string) error {
	errs := c.validateCommon()
	if provider != "cmc" && provider != "portals" && provider != "" {
		errs = append(errs, fmt.Errorf("unknown provider %q", provider))
	}
	if (provider == "" || provider == "cmc") && c.CMCAPIKey == "" {
		errs = append(errs, errors.New("CMC_API_KEY is required"))
	}
	if (provider == "" || provider == "portals") && c.PortalsAPIKey == "" {
		errs = append(errs, errors.New("PORTALS_API_KEY is required"))
	}
	return errors.Join(errs...)
}

// ValidateCollector checks the settings the collector role needs.
func (c *Config) ValidateCollector() error {
	errs := c.validateCommon()
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if c.CMCAPIKey == "" {
		errs = append(errs, errors.New("CMC_API_KEY is required"))
	}
	if c.PortalsAPIKey == "" {
		errs = append(errs, errors.New("PORTALS_API_KEY is required"))
	}
	if c.cronExplicit && c.CollectInterval != 0 {
		errs = append(errs, errors.New("COLLECT_CRON and COLLECT_INTERVAL are mutually exclusive"))
	}
	if c.CollectInterval < 0 {
		errs = append(errs, errors.New("COLLECT_INTERVAL must be > 0"))
	}
	if c.RunTimeout <= 0 {
		errs = append(errs, errors.New("RUN_TIMEOUT must be > 0"))
	}
	if c.SnapshotRetentionDays < 1 {
		errs = append(errs, errors.New("SNAPSHOT_RETENTION_DAYS must be >= 1"))
	}
	return errors.Join(errs...)
}

func (c *Config) validateCommon() []error {
	var errs []error
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL must be debug, info, warn or error, got %q", c.LogLevel))
	}
	switch c.LogFormat {
	case "json", "text":
	default:
		errs = append(errs, fmt.Errorf("LOG_FORMAT must be json or text, got %q", c.LogFormat))
	}
	return errs
}

func mask(s string) string {
	if s == "" {
		return ""
	}
	return "***"
}

// String renders the config with all secrets masked. Value receiver: it applies to
// both Config and *Config in %v / %+v.
func (c Config) String() string {
	return fmt.Sprintf("Config{DatabaseURL:%s CMCAPIKey:%s PortalsAPIKey:%s AssetsPath:%s SettingsPath:%s "+
		"CollectCron:%q CollectInterval:%s CollectOnStart:%t SnapshotRetentionDays:%d RunTimeout:%s RawCaptureDir:%s MigrateOnStart:%t "+
		"HTTPAddr:%s CORSOrigins:%v AdminToken:%s LogLevel:%s LogFormat:%s}",
		maskDSN(c.DatabaseURL), mask(c.CMCAPIKey), mask(c.PortalsAPIKey), c.AssetsPath, c.SettingsPath,
		c.CollectCron, c.CollectInterval, c.CollectOnStart, c.SnapshotRetentionDays, c.RunTimeout, c.RawCaptureDir, c.MigrateOnStart,
		c.HTTPAddr, c.CORSOrigins, mask(c.AdminToken), c.LogLevel, c.LogFormat)
}

// LogValue makes slog print the masked form regardless of the handler.
func (c Config) LogValue() slog.Value { return slog.StringValue(c.String()) }

// maskDSN masks credentials in a postgres:// URL. Anything that is not a
// postgres URL (e.g. a keyword DSN "host=... password=...") is replaced entirely.
func maskDSN(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return "***"
	}
	var b strings.Builder
	b.WriteString(u.Scheme + "://")
	if u.User != nil {
		b.WriteString(url.PathEscape(u.User.Username()))
		if _, ok := u.User.Password(); ok {
			b.WriteString(":***")
		}
		b.WriteString("@")
	}
	b.WriteString(u.Host)
	b.WriteString(u.EscapedPath())
	if u.RawQuery != "" {
		q := u.Query()
		for _, k := range []string{"password", "sslpassword"} {
			if q.Has(k) {
				q.Set(k, "***")
			}
		}
		b.WriteString("?" + strings.ReplaceAll(q.Encode(), "%2A%2A%2A", "***"))
	}
	return b.String()
}
