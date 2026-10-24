package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"fleetmgr/internal/api"
	"fleetmgr/internal/command"
	"fleetmgr/internal/config"
	"fleetmgr/internal/demo"
	"fleetmgr/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.LoadServer()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	if dir := filepath.Dir(cfg.DBPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Error("mkdir", "err", err)
			os.Exit(1)
		}
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Error("db", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	sqlFS, err := fs.Sub(store.SQLFS, "sql")
	if err != nil {
		log.Error("sqlfs", "err", err)
		os.Exit(1)
	}
	if err := st.Migrate(sqlFS); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	cat, err := command.Load(cfg.CommandsPath)
	if err != nil {
		log.Error("catalog", "err", err)
		os.Exit(1)
	}

	srv := &api.Server{
		Cfg:     cfg,
		Store:   st,
		Catalog: cat,
		Log:     log,
	}

	if cfg.DemoMode {
		srv.Demo = demo.New(demo.Config{
			Socket:       cfg.DemoSocket,
			Image:        cfg.DemoImage,
			Network:      cfg.DemoNetwork,
			ManagerURL:   cfg.DemoManagerURL,
			ExporterAddr: cfg.DemoExporterAddr,
		})
		log.Info("demo mode on: pretend nodes run as docker containers", "image", cfg.DemoImage, "network", cfg.DemoNetwork)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go srv.ReaperLoop(ctx)
	go srv.SeedDemo(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		srv.CleanupDemo(c)
		_ = httpSrv.Shutdown(c)
	}()

	log.Info("listen", "addr", cfg.HTTPAddr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("http", "err", err)
		os.Exit(1)
	}
}
