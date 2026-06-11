// Command xray-api is the control-plane server for managing fleets of
// Xray-core nodes. It runs the REST API, the subscription endpoint, and the
// background convergence/stats workers.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ImErdis/xray-api/internal/config"
	"github.com/ImErdis/xray-api/internal/httpapi"
	"github.com/ImErdis/xray-api/internal/service"
	"github.com/ImErdis/xray-api/internal/store"
	"github.com/ImErdis/xray-api/internal/version"
	"github.com/ImErdis/xray-api/internal/worker"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			os.Args = append(os.Args[:1], os.Args[2:]...)
		case "gen-key":
			os.Exit(runGenKey(os.Args[2:]))
		case "version":
			fmt.Printf("xray-api %s (%s)\n", version.Version, version.Commit)
			return
		case "-h", "--help", "help":
			usage()
			return
		}
	}
	os.Exit(runServe(os.Args[1:]))
}

func usage() {
	fmt.Print(`xray-api - control plane for Xray-core fleets

Usage:
  xray-api serve [-config path]     Run the API server and workers (default)
  xray-api gen-key -name NAME [-config path]   Create an admin API key
  xray-api version

Configuration is read from -config (YAML) with XRAY_API_* env overrides.
`)
}

func configPath(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "-config" || args[i] == "--config" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
	}
	return os.Getenv("XRAY_API_CONFIG")
}

func newLogger(cfg *config.Config) *slog.Logger {
	var level slog.Level
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if cfg.Log.Format == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.New(h)
}

func runServe(args []string) int {
	cfg, err := config.Load(configPath(args))
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	log := newLogger(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.Database.DSN)
	if err != nil {
		log.Error("open store", "err", err)
		return 1
	}
	defer st.Close()
	log.Info("database ready, migrations applied")

	mgr := worker.NewManager(st, worker.DefaultDial, worker.Config{
		HealthInterval:        cfg.Workers.HealthInterval,
		StatsInterval:         cfg.Workers.StatsInterval,
		ReconcileInterval:     cfg.Workers.ReconcileInterval,
		SnapshotRetentionDays: cfg.Workers.SnapshotRetentionDays,
	}, log)
	if err := mgr.Start(ctx); err != nil {
		log.Error("start workers", "err", err)
		return 1
	}
	defer mgr.Stop()

	users := service.NewUserService(st, mgr, cfg.PublicBaseURL)
	deps := httpapi.Deps{
		Store:   st,
		Nodes:   service.NewNodeService(st, mgr, worker.DefaultDial),
		Plans:   service.NewPlanService(st),
		Users:   users,
		APIKeys: service.NewAPIKeyService(st),
		Billing: service.NewBillingService(st, users),
		Log:     log,
		Config:  cfg,
	}
	srv := httpapi.NewServer(deps).HTTPServer(cfg.Listen)

	errCh := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", cfg.Listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		log.Error("http server", "err", err)
		return 1
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown", "err", err)
		return 1
	}
	log.Info("stopped")
	return 0
}

func runGenKey(args []string) int {
	name := "admin"
	var cfgPath string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-name", "--name":
			if i+1 < len(args) {
				name = args[i+1]
				i++
			}
		case "-config", "--config":
			if i+1 < len(args) {
				cfgPath = args[i+1]
				i++
			}
		}
	}
	if cfgPath == "" {
		cfgPath = os.Getenv("XRAY_API_CONFIG")
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, err := store.Open(ctx, cfg.Database.DSN)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open store:", err)
		return 1
	}
	defer st.Close()

	created, err := service.NewAPIKeyService(st).Create(ctx, name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create key:", err)
		return 1
	}
	fmt.Printf("Created API key %q\n", created.Key.Name)
	fmt.Printf("  id:  %s\n", created.Key.ID)
	fmt.Printf("  key: %s\n", created.Plaintext)
	fmt.Println("\nStore this now — it will not be shown again.")
	return 0
}
