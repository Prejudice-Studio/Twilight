package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prejudice-studio/twilight/internal/api"
	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

func main() {
	if err := run(os.Args); err != nil {
		slog.Error("twilight exited", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		return runAPI(args[1:])
	}

	switch args[1] {
	case "api":
		return runAPI(args[2:])
	case "all":
		return runAPI(args[2:])
	case "scheduler":
		return runScheduler(args[2:])
	case "bot":
		return runBot(args[2:])
	case "version", "--version", "-v":
		fmt.Println("Twilight Go Backend 0.0.4")
		return nil
	case "help", "--help", "-h":
		printHelp()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[1])
	}
}

func runAPI(args []string) error {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	host := fs.String("host", "", "listen host")
	port := fs.Int("port", 0, "listen port")
	configFile := fs.String("config", "", "config file path")
	debug := fs.Bool("debug", false, "enable debug logging")
	if err := fs.Parse(args); err != nil {
		return err
	}
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	api.InstallRuntimeLogger(os.Stdout, logLevel)

	cfg, err := config.Load(*configFile)
	if err != nil {
		return err
	}
	if *host != "" {
		cfg.Host = *host
	}
	if *port > 0 {
		cfg.Port = *port
	}

	state, err := openStore(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer state.Close()
	app, err := api.New(cfg, state)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              cfg.Host + ":" + strconv.Itoa(cfg.Port),
		Handler:           app,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("Twilight Go API listening", "addr", server.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func runScheduler(args []string) error {
	fs := flag.NewFlagSet("scheduler", flag.ContinueOnError)
	_ = fs.String("config", "", "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	api.InstallRuntimeLogger(os.Stdout, slog.LevelInfo)
	slog.Info("scheduler mode is built into the Go backend; background jobs are exposed through /api/v1/admin/scheduler/jobs")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	return nil
}

func runBot(args []string) error {
	fs := flag.NewFlagSet("bot", flag.ContinueOnError)
	configFile := fs.String("config", "", "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	api.InstallRuntimeLogger(os.Stdout, slog.LevelInfo)
	cfg, err := config.Load(*configFile)
	if err != nil {
		return err
	}
	if !cfg.TelegramMode || strings.TrimSpace(cfg.TelegramBotToken) == "" {
		slog.Info("Telegram bot mode is disabled or bot token is not configured; waiting for shutdown")
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		<-ctx.Done()
		return nil
	}
	state, err := openStore(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer state.Close()
	app, err := api.New(cfg, state)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return app.RunTelegramBot(ctx)
}

func openStore(ctx context.Context, cfg config.Config) (*store.Store, error) {
	switch cfg.DatabaseDriver {
	case "", store.BackendJSON, "file":
		return store.Open(cfg.StateFile)
	case store.BackendPostgres, "postgresql":
		dsn := cfg.PostgresDSN()
		if dsn == "" {
			return nil, fmt.Errorf("database driver is postgres but no PostgreSQL URL or host/user/database is configured")
		}
		openCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		st, err := store.OpenPostgres(openCtx, dsn)
		if err != nil {
			return nil, err
		}
		st.ConfigurePostgres(cfg.PostgresMaxOpenConns, cfg.PostgresMaxIdleConns)
		return st, nil
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.DatabaseDriver)
	}
}

func printHelp() {
	fmt.Println(`Twilight Go Backend

Usage:
  twilight api [--host 0.0.0.0] [--port 5000] [--config config.toml]
  twilight all
  twilight scheduler
  twilight bot
  twilight version`)
}
