package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
		zap.L().Error("twilight exited", zap.Error(err))
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
		return runAll(args[2:])
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
	configFile := fs.String("config", "", "config file path; runtime only accepts the working directory config.toml")
	debug := fs.Bool("debug", false, "enable debug logging")
	if err := fs.Parse(args); err != nil {
		return err
	}
	configPath, err := runtimeConfigPath(*configFile)
	if err != nil {
		return err
	}
	cfg, err := config.NewReader(configPath).Read()
	if err != nil {
		return err
	}
	logLevel := cfg.ZapLevel()
	if *debug {
		logLevel = zapcore.DebugLevel
	}
	api.InstallRuntimeLogger(os.Stdout, logLevel)
	api.ConfigureRuntimeLogging(logLevel, cfg.RuntimeLogLimit)
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
		zap.L().Info("Twilight Go API listening", zap.String("addr", server.Addr))
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

func runAll(args []string) error {
	fs := flag.NewFlagSet("all", flag.ContinueOnError)
	host := fs.String("host", "", "listen host")
	port := fs.Int("port", 0, "listen port")
	configFile := fs.String("config", "", "config file path; runtime only accepts the working directory config.toml")
	debug := fs.Bool("debug", false, "enable debug logging")
	if err := fs.Parse(args); err != nil {
		return err
	}
	configPath, err := runtimeConfigPath(*configFile)
	if err != nil {
		return err
	}
	cfg, err := config.NewReader(configPath).Read()
	if err != nil {
		return err
	}
	logLevel := cfg.ZapLevel()
	if *debug {
		logLevel = zapcore.DebugLevel
	}
	api.InstallRuntimeLogger(os.Stdout, logLevel)
	api.ConfigureRuntimeLogging(logLevel, cfg.RuntimeLogLimit)
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

	errCh := make(chan error, 3)
	go func() {
		zap.L().Info("Twilight Go API listening", zap.String("addr", server.Addr))
		errCh <- server.ListenAndServe()
	}()
	go func() {
		errCh <- app.RunScheduler(ctx)
	}()
	go func() {
		errCh <- app.RunTelegramBot(ctx)
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
		stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return err
	}
}

func runScheduler(args []string) error {
	fs := flag.NewFlagSet("scheduler", flag.ContinueOnError)
	configFile := fs.String("config", "", "config file path; runtime only accepts the working directory config.toml")
	if err := fs.Parse(args); err != nil {
		return err
	}
	configPath, err := runtimeConfigPath(*configFile)
	if err != nil {
		return err
	}
	cfg, err := config.NewReader(configPath).Read()
	if err != nil {
		return err
	}
	api.InstallRuntimeLogger(os.Stdout, cfg.ZapLevel())
	api.ConfigureRuntimeLogging(cfg.ZapLevel(), cfg.RuntimeLogLimit)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	state, err := openStore(context.Background(), cfg)
	if err != nil {
		return err
	}
	defer state.Close()
	app, err := api.New(cfg, state)
	if err != nil {
		return err
	}
	return app.RunScheduler(ctx)
}

func runBot(args []string) error {
	fs := flag.NewFlagSet("bot", flag.ContinueOnError)
	configFile := fs.String("config", "", "config file path; runtime only accepts the working directory config.toml")
	if err := fs.Parse(args); err != nil {
		return err
	}
	configPath, err := runtimeConfigPath(*configFile)
	if err != nil {
		return err
	}
	reader := config.NewReader(configPath)
	cfg, err := reader.Read()
	if err != nil {
		return err
	}
	api.InstallRuntimeLogger(os.Stdout, cfg.ZapLevel())
	api.ConfigureRuntimeLogging(cfg.ZapLevel(), cfg.RuntimeLogLimit)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for !cfg.TelegramMode || strings.TrimSpace(cfg.TelegramBotToken) == "" {
		zap.L().Info("Telegram bot mode is disabled or bot token is not configured; waiting for config reload")
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(3 * time.Second):
		}
		next, err := reader.Read()
		if err != nil {
			zap.L().Warn("Telegram bot config reload failed", zap.Error(err))
			continue
		}
		cfg = next
		api.ConfigureRuntimeLogging(cfg.ZapLevel(), cfg.RuntimeLogLimit)
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
	return app.RunTelegramBot(ctx)
}

func openStore(ctx context.Context, cfg config.Config) (*store.Store, error) {
	switch cfg.DatabaseDriver {
	case "", store.BackendJSON, "file":
		st, err := store.Open(cfg.StateFile)
		if err != nil {
			return nil, err
		}
		bootstrapLegacyAdminsIfNeeded(cfg, st)
		applyConfiguredAdmins(cfg, st)
		return st, nil
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
		applyConfiguredAdmins(cfg, st)
		if !storeHasAdmin(st) {
			legacy, err := openLegacyJSONStoreIfPopulated(cfg)
			if err != nil {
				_ = st.Close()
				return nil, err
			}
			if legacy != nil {
				bootstrapLegacyAdminsIfNeeded(cfg, legacy)
				applyConfiguredAdmins(cfg, legacy)
				if storeHasAdmin(legacy) {
					_ = st.Close()
					zap.L().Warn("PostgreSQL has no administrator; using legacy JSON state so existing admins can log in and run database migration", zap.String("state_file", cfg.StateFile))
					return legacy, nil
				}
				_ = legacy.Close()
			}
			bootstrapLegacyAdminsIfNeeded(cfg, st)
			applyConfiguredAdmins(cfg, st)
		}
		return st, nil
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.DatabaseDriver)
	}
}

func applyConfiguredAdmins(cfg config.Config, st *store.Store) {
	if st == nil {
		return
	}
	uidSet := map[int64]bool{}
	for _, uid := range cfg.AdminUIDs {
		if uid > 0 {
			uidSet[uid] = true
		}
	}
	nameSet := map[string]bool{}
	for _, username := range cfg.AdminUsernames {
		username = strings.ToLower(strings.TrimSpace(username))
		if username != "" {
			nameSet[username] = true
		}
	}
	if len(uidSet) == 0 && len(nameSet) == 0 {
		return
	}
	for _, user := range st.ListUsers() {
		if !uidSet[user.UID] && !nameSet[strings.ToLower(strings.TrimSpace(user.Username))] {
			continue
		}
		updated, err := st.UpdateUser(user.UID, func(u *store.User) error {
			u.Role = store.RoleAdmin
			u.Active = true
			return nil
		})
		if err == nil {
			zap.L().Info("configured administrator applied", zap.Int64("uid", updated.UID), zap.String("username", updated.Username))
		}
	}
}

func storeHasAdmin(st *store.Store) bool {
	if st == nil {
		return false
	}
	for _, user := range st.ListUsers() {
		if user.Role == store.RoleAdmin && user.Active {
			return true
		}
	}
	return false
}

func openLegacyJSONStoreIfPopulated(cfg config.Config) (*store.Store, error) {
	stateFile := strings.TrimSpace(cfg.StateFile)
	if stateFile == "" {
		stateFile = filepath.Join(firstNonEmpty(cfg.DatabaseDir, "db"), "twilight_go_state.json")
	}
	info, err := os.Stat(stateFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if info.IsDir() || !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, nil
	}
	legacy, err := store.Open(stateFile)
	if err != nil {
		return nil, fmt.Errorf("open legacy JSON state %q: %w", stateFile, err)
	}
	if legacy.UserCount() == 0 {
		_ = legacy.Close()
		return nil, nil
	}
	return legacy, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func runtimeConfigPath(path string) (string, error) {
	const fixed = "config.toml"
	path = strings.TrimSpace(path)
	if path == "" {
		return fixed, nil
	}
	clean := filepath.Clean(path)
	if filepath.Base(clean) != fixed {
		return "", fmt.Errorf("configuration file is fixed to the working directory config.toml, got %q", path)
	}
	target, err := filepath.Abs(clean)
	if err != nil {
		return "", err
	}
	expected, err := filepath.Abs(fixed)
	if err != nil {
		return "", err
	}
	if target != expected {
		return "", fmt.Errorf("configuration file is fixed to %s, got %s", expected, target)
	}
	return fixed, nil
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
