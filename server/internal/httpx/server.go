package httpx

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

// Pinger — проверка доступности БД.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps — зависимости HTTP-слоя.
type Deps struct {
	Log *slog.Logger
	// DB — пул соединений с PostgreSQL.
	DB Pinger
	// Static — собранный фронтенд (web.Dist()).
	Static fs.FS
	// Lang — язык инстанса (TEO_KIN_LANGUAGE).
	Lang string
	// API — обработчик /api/* (nil — только 404).
	API http.Handler
}

// NewHandler собирает маршруты и middleware.
func NewHandler(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", healthz(d.DB, d.Log))
	mux.HandleFunc("/api", apiNotFound)
	if d.API != nil {
		mux.Handle("/api/", d.API)
	} else {
		mux.HandleFunc("/api/", apiNotFound)
	}
	mux.Handle("/", spa(d.Static, d.Lang))
	return Chain(mux, RequestID, Logging(d.Log), Recover(d.Log))
}

type healthBody struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

const healthDBTimeout = 2 * time.Second

func healthz(db Pinger, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthDBTimeout)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			log.WarnContext(r.Context(), "healthz: БД недоступна",
				slog.String("request_id", RequestIDFrom(r.Context())),
				slog.String("error", err.Error()))
			WriteJSON(w, http.StatusServiceUnavailable, healthBody{Status: "error", DB: "error"})
			return
		}
		WriteJSON(w, http.StatusOK, healthBody{Status: "ok", DB: "ok"})
	}
}

func apiNotFound(w http.ResponseWriter, _ *http.Request) {
	WriteError(w, http.StatusNotFound, "not_found", "нет такого метода API")
}
