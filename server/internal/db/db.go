// Package db — подключение к PostgreSQL (F-18).
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open создаёт пул соединений. Соединение устанавливается лениво:
// недоступная БД не мешает старту, её состояние видно в /healthz.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	if url == "" {
		return nil, errors.New("TEO_KIN_DATABASE_URL не задан")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		// текст ошибки pgx может содержать строку подключения с паролем
		return nil, errors.New("TEO_KIN_DATABASE_URL: неверная строка подключения")
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = "kintion"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("пул соединений: %w", err)
	}
	return pool, nil
}
