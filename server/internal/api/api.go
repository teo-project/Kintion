// Package api — HTTP-обработчики /api и DTO. Таблицы наружу не выходят:
// ответы собираются из DTO (F-48).
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	"kintion/server/internal/auth"
	"kintion/server/internal/files"
	"kintion/server/internal/httpx"
	"kintion/server/internal/packs"
	"kintion/server/internal/records"
)

// CSRFHeader — заголовок с CSRF-токеном сессии (Д-13).
const CSRFHeader = "X-Kintion-CSRF"

// SessionCookie — имя cookie сессии.
const SessionCookie = "kintion_session"

// Deps — зависимости API.
type Deps struct {
	Log  *slog.Logger
	Auth *auth.Service
	// PublicURL — TEO_KIN_PUBLIC_URL: Secure у cookie и проверка Origin.
	PublicURL string
	Pool      *pgxpool.Pool
	Registry  *packs.Registry
	// Lang — язык инстанса: название первого дашборда.
	Lang string
	// Files — файлы на томе (F-29).
	Files *files.Store
	// Licenses — тексты лицензий сторонних библиотек (F-91), только вошедшим.
	Licenses []byte
}

// API — обработчик /api/*.
type API struct {
	d      Deps
	mux    *http.ServeMux
	secure bool
	origin string // ожидаемый Origin из PublicURL, "" — сверять с Host
	rec    *records.Service
}

// New собирает API.
func New(d Deps) *API {
	a := &API{d: d, mux: http.NewServeMux(), rec: records.New(d.Registry)}
	if u, err := url.Parse(d.PublicURL); err == nil && u.Host != "" {
		a.secure = u.Scheme == "https"
		a.origin = u.Scheme + "://" + u.Host
	}
	a.routes()
	a.prefsRoutes()
	a.dashboardRoutes()
	a.recordRoutes()
	a.groupRoutes()
	a.fileRoutes()
	a.auditRoutes()
	a.mux.HandleFunc("GET /api/licenses", a.licenses)
	a.mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "нет такого метода API")
	})
	return a
}

// public — запросы без сессии: настройка и вход.
func public(r *http.Request) bool {
	return r.URL.Path == "/api/setup" || r.URL.Path == "/api/login"
}

// licenses — тексты лицензий сторонних библиотек (F-91). Сессию уже проверил
// ServeHTTP: без входа — 401, как у остального API.
func (a *API) licenses(w http.ResponseWriter, _ *http.Request) {
	if len(a.d.Licenses) == 0 {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "нет такого метода API")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(a.d.Licenses)
}

func unsafeMethod(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

type ctxKey int

const sessionKey ctxKey = iota

// SessionFrom — сессия запроса (есть у всех маршрутов, кроме public).
func SessionFrom(ctx context.Context) (auth.Session, bool) {
	s, ok := ctx.Value(sessionKey).(auth.Session)
	return s, ok
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !public(r) {
		c, err := r.Cookie(SessionCookie)
		token := ""
		if err == nil {
			token = c.Value
		}
		sess, err := a.d.Auth.Resolve(r.Context(), token)
		if errors.Is(err, auth.ErrNoSession) {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "нужен вход")
			return
		} else if err != nil {
			a.internal(w, r, err)
			return
		}
		if unsafeMethod(r.Method) && !sess.CheckCSRF(r.Header.Get(CSRFHeader)) {
			httpx.WriteError(w, http.StatusForbidden, "csrf", "нет или неверный CSRF-токен")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), sessionKey, sess))
	}
	if unsafeMethod(r.Method) {
		if !a.contentTypeOK(r) {
			httpx.WriteError(w, http.StatusForbidden, "csrf", "изменяющий запрос — только application/json")
			return
		}
		if !a.originOK(r) {
			httpx.WriteError(w, http.StatusForbidden, "csrf", "чужой Origin")
			return
		}
	}
	a.mux.ServeHTTP(w, r)
}

// contentTypeOK — изменяющий запрос несёт JSON (или multipart для файлов);
// простые формы с чужого сайта так не отправить.
func (a *API) contentTypeOK(r *http.Request) bool {
	if r.ContentLength == 0 && r.Header.Get("Content-Type") == "" {
		return true // тело пустое (logout и т. п.): CSRF-заголовок уже проверен
	}
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && (mt == "application/json" || mt == "multipart/form-data")
}

// originOK — Origin, если он есть, совпадает с TEO_KIN_PUBLIC_URL (или с Host,
// если адрес не задан).
func (a *API) originOK(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	if a.origin != "" {
		return strings.EqualFold(o, a.origin)
	}
	u, err := url.Parse(o)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && strings.EqualFold(u.Host, r.Host)
}

func (a *API) internal(w http.ResponseWriter, r *http.Request, err error) {
	a.d.Log.ErrorContext(r.Context(), "api", slog.String("request_id", httpx.RequestIDFrom(r.Context())),
		slog.String("path", r.URL.Path), slog.String("error", err.Error()))
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "внутренняя ошибка")
}

// ValidationBody — ошибка проверки одного поля формы.
type ValidationBody struct {
	Error   string `json:"error"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

const maxBody = 1 << 20

// decode читает JSON-тело; лишние ключи и не UTF-8 — ошибка.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "тело запроса: "+err.Error())
		return false
	}
	// сырые значения (json.RawMessage) идут в базу как есть — проверить кодировку сразу
	if !utf8.Valid(data) {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "тело запроса — не UTF-8")
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "неверный JSON: "+err.Error())
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "лишние данные после JSON")
		return false
	}
	return true
}
