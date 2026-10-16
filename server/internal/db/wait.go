package db

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WaitReady ждёт, пока БД ответит, не дольше timeout.
func WaitReady(ctx context.Context, pool *pgxpool.Pool, timeout time.Duration, log *slog.Logger) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	delay := 500 * time.Millisecond
	for {
		pingCtx, pingCancel := context.WithTimeout(ctx, 3*time.Second)
		err := pool.Ping(pingCtx)
		pingCancel()
		if err == nil {
			return nil
		}
		log.WarnContext(ctx, "БД недоступна, ждём", slog.String("error", err.Error()))
		select {
		case <-ctx.Done():
			return err
		case <-time.After(delay):
		}
		delay = min(delay*2, 5*time.Second)
	}
}
