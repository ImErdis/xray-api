// Package config loads the application configuration from a YAML file with
// XRAY_API_* environment variable overrides. It is intentionally small —
// a config framework is not worth its dependency tree here.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen        string `yaml:"listen"`
	PublicBaseURL string `yaml:"public_base_url"`

	Database struct {
		DSN string `yaml:"dsn"`
	} `yaml:"database"`

	Auth struct {
		// BootstrapAPIKey, if set, is accepted as a valid admin key in
		// addition to keys stored in the database. Intended for first-run
		// setup; prefer creating real keys and unsetting this.
		BootstrapAPIKey string `yaml:"bootstrap_api_key"`
	} `yaml:"auth"`

	Workers struct {
		HealthInterval        time.Duration `yaml:"health_interval"`
		StatsInterval         time.Duration `yaml:"stats_interval"`
		ReconcileInterval     time.Duration `yaml:"reconcile_interval"`
		SnapshotRetentionDays int           `yaml:"snapshot_retention_days"`
	} `yaml:"workers"`

	Subscription struct {
		ProfileUpdateIntervalHours int    `yaml:"profile_update_interval_hours"`
		ProfileTitle               string `yaml:"profile_title"`
	} `yaml:"subscription"`

	Webhooks struct {
		// BillingSecret is the shared HMAC-SHA256 secret for
		// POST /webhooks/billing. The endpoint is disabled while empty.
		BillingSecret string `yaml:"billing_secret"`
	} `yaml:"webhooks"`

	RateLimit struct {
		// PublicPerMinute caps requests per client IP per minute on the public
		// endpoints (/sub/*, /webhooks/*). 0 disables limiting.
		PublicPerMinute int `yaml:"public_per_minute"`
	} `yaml:"rate_limit"`

	Metrics struct {
		// Enabled exposes Prometheus metrics at /metrics (unauthenticated;
		// firewall accordingly or scrape over a private network).
		Enabled bool `yaml:"enabled"`
	} `yaml:"metrics"`

	Log struct {
		Level  string `yaml:"level"`  // debug|info|warn|error
		Format string `yaml:"format"` // text|json
	} `yaml:"log"`
}

func Default() *Config {
	c := &Config{}
	c.Listen = ":8080"
	c.PublicBaseURL = "http://localhost:8080"
	c.Database.DSN = "postgres://xray:xray@localhost:5432/xray_api?sslmode=disable"
	c.Workers.HealthInterval = 30 * time.Second
	c.Workers.StatsInterval = 30 * time.Second
	c.Workers.ReconcileInterval = 5 * time.Minute
	c.Workers.SnapshotRetentionDays = 90
	c.Subscription.ProfileUpdateIntervalHours = 12
	c.RateLimit.PublicPerMinute = 60
	c.Metrics.Enabled = true
	c.Log.Level = "info"
	c.Log.Format = "text"
	return c
}

// Load reads path (optional, "" skips the file) and applies env overrides.
func Load(path string) (*Config, error) {
	c := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(b, c); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}
	applyEnv(c)
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func applyEnv(c *Config) {
	str := func(key string, dst *string) {
		if v, ok := os.LookupEnv(key); ok {
			*dst = v
		}
	}
	dur := func(key string, dst *time.Duration) {
		if v, ok := os.LookupEnv(key); ok {
			if d, err := time.ParseDuration(v); err == nil {
				*dst = d
			}
		}
	}
	num := func(key string, dst *int) {
		if v, ok := os.LookupEnv(key); ok {
			if n, err := strconv.Atoi(v); err == nil {
				*dst = n
			}
		}
	}
	boolean := func(key string, dst *bool) {
		if v, ok := os.LookupEnv(key); ok {
			if b, err := strconv.ParseBool(v); err == nil {
				*dst = b
			}
		}
	}

	str("XRAY_API_LISTEN", &c.Listen)
	str("XRAY_API_PUBLIC_BASE_URL", &c.PublicBaseURL)
	str("XRAY_API_DATABASE_DSN", &c.Database.DSN)
	str("XRAY_API_AUTH_BOOTSTRAP_API_KEY", &c.Auth.BootstrapAPIKey)
	dur("XRAY_API_WORKERS_HEALTH_INTERVAL", &c.Workers.HealthInterval)
	dur("XRAY_API_WORKERS_STATS_INTERVAL", &c.Workers.StatsInterval)
	dur("XRAY_API_WORKERS_RECONCILE_INTERVAL", &c.Workers.ReconcileInterval)
	num("XRAY_API_WORKERS_SNAPSHOT_RETENTION_DAYS", &c.Workers.SnapshotRetentionDays)
	num("XRAY_API_SUBSCRIPTION_PROFILE_UPDATE_INTERVAL_HOURS", &c.Subscription.ProfileUpdateIntervalHours)
	str("XRAY_API_SUBSCRIPTION_PROFILE_TITLE", &c.Subscription.ProfileTitle)
	str("XRAY_API_WEBHOOKS_BILLING_SECRET", &c.Webhooks.BillingSecret)
	num("XRAY_API_RATE_LIMIT_PUBLIC_PER_MINUTE", &c.RateLimit.PublicPerMinute)
	boolean("XRAY_API_METRICS_ENABLED", &c.Metrics.Enabled)
	str("XRAY_API_LOG_LEVEL", &c.Log.Level)
	str("XRAY_API_LOG_FORMAT", &c.Log.Format)
}

func (c *Config) validate() error {
	if c.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}
	if c.Workers.HealthInterval < time.Second {
		return fmt.Errorf("workers.health_interval must be >= 1s")
	}
	if c.Workers.StatsInterval < time.Second {
		return fmt.Errorf("workers.stats_interval must be >= 1s")
	}
	if c.Workers.ReconcileInterval < 10*time.Second {
		return fmt.Errorf("workers.reconcile_interval must be >= 10s")
	}
	return nil
}
