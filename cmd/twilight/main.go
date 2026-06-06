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
		fmt.Println("Twilight Go Backend 0.0.6")
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
	app, err := api.New(cfg, state)
	if err != nil {
		state.Close()
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

	// 三个 supervisor goroutine 各自把退出原因送到独立 channel：
	//   - serverErr：ListenAndServe 退出（http.ErrServerClosed = 正常 shutdown）
	//   - schedulerErr / botErr：scheduler / bot 退出。bot 在未配置 token 时
	//     return nil，不能让这条 nil 当成"server 挂了"触发整进程退出（旧实现把
	//     scheduler/bot 任意一个先收到的退出当作 fatal，bot 因为 1s 内 nil 退出
	//     直接拖走 scheduler 与 server）。
	// shutdown 路径：select 看到任一 fatal 后 stop()，等待所有 goroutine drain
	// 完再 state.Close()，避免 scheduler/bot 持的 store 引用在 Close 后被解引用。
	serverErr := make(chan error, 1)
	schedulerErr := make(chan error, 1)
	botErr := make(chan error, 1)
	go func() {
		zap.L().Info("Twilight Go API listening", zap.String("addr", server.Addr))
		serverErr <- server.ListenAndServe()
	}()
	go func() {
		schedulerErr <- app.RunScheduler(ctx)
	}()
	go func() {
		botErr <- app.RunTelegramBot(ctx)
	}()

	closeStore := func() {
		// 三条 goroutine 都 drain 完才 Close store：scheduler 退出顺序里若仍持
		// 一个未醒的 PG 调用，提前 Close 会让那条 ExecContext 拿到 nil db。
		state.Close()
	}

	var fatal error
	select {
	case <-ctx.Done():
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			fatal = err
		}
	case err := <-schedulerErr:
		// scheduler 主循环 panic recover 后会重启，正常路径只有 ctx.Done 才返回
		// nil；非 nil 表示长时间崩溃后 RunScheduler 自己放弃。视作 fatal。
		if err != nil {
			fatal = err
		}
	case err := <-botErr:
		// bot 在未配置 token 时立即 return nil，这条不能视为 fatal：进入
		// "server + scheduler 继续跑，bot 不参与"模式，并继续等下一个事件。
		if err != nil {
			fatal = err
		} else {
			zap.L().Info("telegram bot exited cleanly; remaining services keep running")
			select {
			case <-ctx.Done():
			case err := <-serverErr:
				if !errors.Is(err, http.ErrServerClosed) {
					fatal = err
				}
			case err := <-schedulerErr:
				if err != nil {
					fatal = err
				}
			}
		}
	}

	stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)

	// drain 剩余 channel：每个 goroutine 退出时都会向自己 channel 发一条，这里
	// 先排空再 Close store。serverErr 已可能在 select 里被消费，再读一次会阻塞，
	// 用 select+default 保证不卡死。
	drain := func(ch <-chan error) {
		select {
		case <-ch:
		case <-time.After(15 * time.Second):
		}
	}
	drain(serverErr)
	drain(schedulerErr)
	drain(botErr)
	closeStore()

	if fatal != nil && !errors.Is(fatal, http.ErrServerClosed) {
		return fatal
	}
	return nil
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
			return nil
		})
		if err == nil {
			zap.L().Info("configured administrator applied", zap.Int64("uid", updated.UID), zap.String("username", updated.Username))
		}
	}
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
