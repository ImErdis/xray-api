package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaultsAndEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
listen: ":9090"
database:
  dsn: "postgres://u:p@h:5432/db?sslmode=disable"
workers:
  stats_interval: 15s
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XRAY_API_LISTEN", ":7777")
	t.Setenv("XRAY_API_WORKERS_RECONCILE_INTERVAL", "2m")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Listen != ":7777" {
		t.Errorf("env override failed: listen=%q", cfg.Listen)
	}
	if cfg.Workers.StatsInterval != 15*time.Second {
		t.Errorf("file value lost: stats_interval=%v", cfg.Workers.StatsInterval)
	}
	if cfg.Workers.ReconcileInterval != 2*time.Minute {
		t.Errorf("env override failed: reconcile=%v", cfg.Workers.ReconcileInterval)
	}
	if cfg.Workers.HealthInterval != 30*time.Second {
		t.Errorf("default lost: health=%v", cfg.Workers.HealthInterval)
	}
}

func TestValidateRejectsEmptyDSN(t *testing.T) {
	c := Default()
	c.Database.DSN = ""
	if err := c.validate(); err == nil {
		t.Error("expected error for empty dsn")
	}
}
