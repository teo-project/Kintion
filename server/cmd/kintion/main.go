// Команда kintion — сервер Kintion.
//
//	kintion [serve]                 применить миграции и запустить сервер (по умолчанию)
//	kintion migrate up|down|status  миграции схемы БД (down — откат одной последней)
//	kintion reset-password --login <логин>  новый случайный пароль владельцу (В-4)
//	kintion healthcheck             проверить /healthz запущенного сервера (для Docker HEALTHCHECK)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"kintion/i18n"
	"kintion/server/internal/api"
	"kintion/server/internal/auth"
	"kintion/server/internal/config"
	"kintion/server/internal/dashboards"
	"kintion/server/internal/db"
	"kintion/server/internal/files"
	"kintion/server/internal/httpx"
	"kintion/server/internal/packs"
	"kintion/server/internal/version"
	"kintion/web"
)

const usage = `использование: kintion [serve | migrate up|down|status | reset-password --login <логин> | healthcheck]`

// dbWaitTimeout — сколько ждать БД при старте (Compose поднимает её параллельно).
const dbWaitTimeout = 60 * time.Second

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "kintion:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	sub := ""
	switch {
	case cmd == "migrate" && len(args) == 2:
		sub = args[1]
	case cmd == "reset-password" && len(args) == 3 && args[1] == "--login":
		sub = args[2]
	case cmd == "reset-password" && len(args) == 2 && strings.HasPrefix(args[1], "--login="):
		sub = strings.TrimPrefix(args[1], "--login=")
	case len(args) > 1:
		return errors.New(usage)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !i18n.Has(cfg.Language) {
		return fmt.Errorf("TEO_KIN_LANGUAGE: нет языкового файла для %q (есть: %v)", cfg.Language, i18n.Languages())
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cmd {
	case "serve":
		return serve(ctx, cfg, newLogger(cfg))
	case "migrate":
		return migrate(ctx, cfg, newLogger(cfg), sub)
	case "reset-password":
		if sub == "" {
			return errors.New(usage)
		}
		return resetPassword(ctx, cfg, newLogger(cfg), sub)
	case "healthcheck":
		return healthcheck(cfg)
	default:
		return errors.New(usage)
	}
}

func newLogger(cfg config.Config) *slog.Logger {
	level, _ := config.ParseLogLevel(cfg.LogLevel) // уже проверено в Load
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)
	return log
}

func migrate(ctx context.Context, cfg config.Config, log *slog.Logger, sub string) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.WaitReady(ctx, pool, dbWaitTimeout, log); err != nil {
		return fmt.Errorf("БД недоступна: %w", err)
	}
	m := db.NewMigrator(pool, version.Version, log)
	switch sub {
	case "up":
		return m.Up(ctx)
	case "down":
		return m.Down(ctx)
	case "status":
		st, err := m.Status(ctx)
		if err != nil {
			return err
		}
		for _, s := range st {
			state := "pending"
			if s.Applied {
				state = "applied"
			}
			fmt.Printf("%5d  %-8s %s\n", s.Version, state, s.Name)
		}
		return nil
	default:
		return errors.New(usage)
	}
}

// loadPacks читает паки; ошибки формата — приложение не стартует (план §3).
func loadPacks(cfg config.Config, log *slog.Logger) (*packs.Registry, error) {
	src, err := packs.Source(cfg.PacksPath, cfg.Language)
	if err != nil {
		return nil, err
	}
	reg, diags := packs.Load(src, cfg.Language, cfg.PackList())
	for _, d := range diags {
		level := slog.LevelInfo
		switch d.Level {
		case packs.LevelError:
			level = slog.LevelError
		case packs.LevelWarn:
			level = slog.LevelWarn
		}
		log.Log(context.Background(), level, "пак: "+d.Message, slog.String("file", d.File))
	}
	if packs.HasErrors(diags) {
		return nil, errors.New("паки содержат ошибки (см. журнал выше), запуск невозможен")
	}
	ids := make([]string, 0, len(reg.Packs))
	for _, m := range reg.Packs {
		ids = append(ids, m.ID+"@"+m.Version)
	}
	log.Info("паки загружены", slog.Any("packs", ids))
	return reg, nil
}

func serve(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	reg, err := loadPacks(cfg, log)
	if err != nil {
		return err
	}

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.WaitReady(ctx, pool, dbWaitTimeout, log); err != nil {
		return fmt.Errorf("БД недоступна: %w", err)
	}
	if err := db.NewMigrator(pool, version.Version, log).Up(ctx); err != nil {
		return err
	}
	if err := packs.Sync(ctx, pool, reg); err != nil {
		return fmt.Errorf("таблица pack: %w", err)
	}

	authSvc := auth.New(pool)
	token, err := authSvc.EnsureBootstrap(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	if token != "" {
		// F-28: первый аккаунт — по токену из журнала первого запуска
		log.Warn("первый запуск: откройте приложение и введите токен настройки",
			slog.String("bootstrap_token", token))
	}
	if err := dashboards.SyncTemplates(ctx, pool, reg); err != nil {
		return fmt.Errorf("шаблоны дашбордов: %w", err)
	}
	apiHandler := api.New(api.Deps{
		Log: log, Auth: authSvc, PublicURL: cfg.PublicURL,
		Pool: pool, Registry: reg, Lang: cfg.Language,
		Files:    &files.Store{Root: cfg.StoragePath, MaxBytes: cfg.UploadMaxBytes()},
		Licenses: web.Licenses(),
	})

	srv := &http.Server{
		Addr: cfg.HTTPBind,
		Handler: httpx.NewHandler(httpx.Deps{
			Log:    log,
			DB:     pool,
			Static: web.Dist(),
			Lang:   cfg.Language,
			API:    apiHandler,
		}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("start", slog.String("version", version.Version),
			slog.Int64("schema_version", db.SchemaVersion()), slog.String("bind", cfg.HTTPBind))
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Info("shutdown")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// resetPassword — восстановление доступа без почты (В-4): новый случайный
// пароль печатается один раз, все сессии аккаунта завершаются.
func resetPassword(ctx context.Context, cfg config.Config, log *slog.Logger, login string) error {
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.WaitReady(ctx, pool, dbWaitTimeout, log); err != nil {
		return fmt.Errorf("БД недоступна: %w", err)
	}
	password := auth.GeneratePassword()
	if err := auth.New(pool).ResetPassword(ctx, login, password); err != nil {
		return err
	}
	fmt.Printf("Новый пароль для %s: %s\nВсе сессии завершены. Смените пароль после входа.\n", login, password)
	return nil
}

// healthcheck запрашивает /healthz своего же сервера по адресу из TEO_KIN_HTTP_BIND.
func healthcheck(cfg config.Config) error {
	host, port, err := net.SplitHostPort(cfg.HTTPBind)
	if err != nil {
		return fmt.Errorf("TEO_KIN_HTTP_BIND: %w", err)
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(host, port) + "/healthz"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: %s", resp.Status)
	}
	return nil
}
