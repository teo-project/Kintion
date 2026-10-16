// Package httpx — HTTP-обвязка: middleware, JSON-ответы, маршруты.
package httpx

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// statusWriter запоминает код ответа.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Unwrap нужен http.ResponseController (Flush и т. п.).
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Logging пишет строку журнала на каждый запрос.
func Logging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(sw, r)
			if sw.status == 0 {
				sw.status = http.StatusOK
			}
			log.LogAttrs(r.Context(), levelFor(sw.status), "http",
				slog.String("request_id", RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.Int("bytes", sw.bytes),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

func levelFor(status int) slog.Level {
	if status >= 500 {
		return slog.LevelError
	}
	return slog.LevelInfo
}

// Recover перехватывает панику обработчика: пишет стек в журнал и отвечает 500.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w}
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler { //nolint:errorlint // сравнение значения паники, не ошибки
					panic(rec)
				}
				log.ErrorContext(r.Context(), "panic",
					slog.String("request_id", RequestIDFrom(r.Context())),
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
				)
				if sw.status == 0 {
					WriteError(sw, http.StatusInternalServerError, "internal", "внутренняя ошибка")
				}
			}()
			next.ServeHTTP(sw, r)
		})
	}
}

// Chain оборачивает h в middleware; первый в списке — внешний.
func Chain(h http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}
