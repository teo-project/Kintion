package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// RequestIDHeader — заголовок ответа с идентификатором запроса.
const RequestIDHeader = "X-Request-Id"

type ctxKey int

const requestIDKey ctxKey = iota

// RequestID присваивает каждому запросу новый идентификатор, кладёт его
// в контекст и в заголовок ответа. Входящий X-Request-Id не принимается:
// идентификатор журнала создаёт только сервер.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// RequestIDFrom возвращает идентификатор запроса из контекста.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read не возвращает ошибок
	return hex.EncodeToString(b[:])
}
