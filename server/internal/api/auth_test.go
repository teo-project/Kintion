package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"

	"kintion/server/internal/api"
	"kintion/server/internal/auth"
	"kintion/server/internal/db"
	"kintion/server/internal/db/dbtest"
	"kintion/server/internal/files"
	"kintion/server/internal/httpx"
	"kintion/server/internal/packs"
)

func init() {
	auth.Params = auth.Argon2Params{Memory: 1024, Time: 1, Threads: 1} // быстрые тесты
}

// testUploadMax — предел загрузки в тестах.
const testUploadMax = 1 << 20

type env struct {
	t     *testing.T
	pool  *pgxpool.Pool
	auth  *auth.Service
	srv   *httptest.Server
	token string // bootstrap
	reg   *packs.Registry
	// storage — TEO_KIN_STORAGE_PATH сервера
	storage string
}

// newEnv — сервер с БД; enabled — включённые паки (по умолчанию оба встроенных).
func newEnv(t *testing.T, enabled ...string) *env {
	t.Helper()
	if len(enabled) == 0 {
		enabled = []string{"kin.core", "kin.christian.orthodox"}
	}
	src, err := packs.Source("", "ru")
	if err != nil {
		t.Fatal(err)
	}
	return newEnvFS(t, src, "ru", enabled...)
}

// newEnvAlt — сервер на синтетическом паке test.alt (план §3 п.7).
func newEnvAlt(t *testing.T) *env {
	t.Helper()
	return newEnvFS(t, os.DirFS("../../../testdata/packs/xx"), "xx", "test.alt")
}

func newEnvFS(t *testing.T, src fs.FS, lang string, enabled ...string) *env {
	t.Helper()
	pool := dbtest.New(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := db.NewMigrator(pool, "test", log).Up(ctx); err != nil {
		t.Fatal(err)
	}
	svc := auth.New(pool)
	token, err := svc.EnsureBootstrap(ctx)
	if err != nil || token == "" {
		t.Fatalf("bootstrap %q %v", token, err)
	}
	reg, ds := packs.Load(src, lang, enabled)
	if packs.HasErrors(ds) {
		t.Fatal(ds)
	}
	storage := t.TempDir()
	h := api.New(api.Deps{Log: log, Auth: svc, Pool: pool, Registry: reg, Lang: "ru",
		Files: &files.Store{Root: storage, MaxBytes: testUploadMax}, Licenses: []byte(testLicenses)})
	srv := httptest.NewServer(httpx.NewHandler(httpx.Deps{Log: log, DB: pool, API: h, Static: fstest.MapFS{}}))
	t.Cleanup(srv.Close)
	return &env{t: t, pool: pool, auth: svc, srv: srv, token: token, reg: reg, storage: storage}
}

// client — «браузер»: cookie сессии и CSRF-токен.
type client struct {
	e      *env
	cookie *http.Cookie
	csrf   string
}

type resp struct {
	status int
	body   map[string]any
	header http.Header
}

func (c *client) do(method, path string, body any, mod ...func(*http.Request)) resp {
	c.e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.e.srv.URL+path, rd)
	if err != nil {
		c.e.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if c.csrf != "" {
		req.Header.Set(api.CSRFHeader, c.csrf)
	}
	for _, m := range mod {
		m(req)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	data, _ := io.ReadAll(res.Body)
	out := resp{status: res.StatusCode, header: res.Header}
	_ = json.Unmarshal(data, &out.body)
	for _, ck := range res.Cookies() {
		if ck.Name == api.SessionCookie {
			if ck.MaxAge < 0 {
				c.cookie = nil
			} else {
				c.cookie = ck
			}
		}
	}
	if s, ok := out.body["csrf"].(string); ok {
		c.csrf = s
	}
	return out
}

func (e *env) client() *client { return &client{e: e} }

func expect(t *testing.T, r resp, status int, what string) {
	t.Helper()
	if r.status != status {
		t.Fatalf("%s: status %d, want %d (%v)", what, r.status, status, r.body)
	}
}

// setupOwner — настройка по bootstrap-токену; клиент с сессией владельца.
func (e *env) setupOwner() *client {
	e.t.Helper()
	c := e.client()
	r := c.do("POST", "/api/setup", map[string]string{"token": e.token, "login": "Owner", "password": "secret-pass"})
	expect(e.t, r, http.StatusCreated, "setup")
	return c
}

func TestSetup(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	r := c.do("GET", "/api/setup", nil)
	if r.status != 200 || r.body["needed"] != true {
		t.Fatalf("setup status %v", r.body)
	}
	expect(t, c.do("POST", "/api/setup", map[string]string{"token": "wrong", "login": "owner", "password": "secret-pass"}),
		http.StatusForbidden, "wrong token")
	expect(t, c.do("POST", "/api/setup", map[string]string{"token": e.token, "login": "owner", "password": "short"}),
		http.StatusBadRequest, "short password")

	owner := e.setupOwner()
	r = owner.do("GET", "/api/me", nil)
	expect(t, r, http.StatusOK, "me")
	acc, _ := r.body["account"].(map[string]any)
	if acc["login"] != "owner" || acc["is_admin"] != true || r.body["csrf"] == "" {
		t.Fatalf("me %v", r.body)
	}
	// повторная настройка — отказ, и тем же токеном, и новым
	expect(t, e.client().do("POST", "/api/setup", map[string]string{"token": e.token, "login": "x", "password": "secret-pass"}),
		http.StatusConflict, "repeated setup")
	if tok, err := e.auth.EnsureBootstrap(context.Background()); err != nil || tok != "" {
		t.Fatalf("bootstrap after setup: %q %v", tok, err)
	}
	if r := e.client().do("GET", "/api/setup", nil); r.body["needed"] != false {
		t.Fatal("setup still needed")
	}
}

const testLicenses = "Third-party licenses: test text"

// TestLicenses — тексты лицензий (F-91) только вошедшим: без сессии — 401
// без текста, с сессией — сам текст.
func TestLicenses(t *testing.T) {
	e := newEnv(t)
	get := func(c *client) (int, string) {
		t.Helper()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, e.srv.URL+"/api/licenses", nil)
		if c != nil && c.cookie != nil {
			req.AddCookie(c.cookie)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		data, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(data)
	}
	if st, body := get(nil); st != http.StatusUnauthorized || strings.Contains(body, testLicenses) {
		t.Fatalf("anonymous: %d %q", st, body)
	}
	owner := e.setupOwner()
	if st, body := get(owner); st != http.StatusOK || body != testLicenses {
		t.Fatalf("owner: %d %q", st, body)
	}
}

func TestBootstrapTokenRotates(t *testing.T) {
	e := newEnv(t)
	second, err := e.auth.EnsureBootstrap(context.Background())
	if err != nil || second == e.token {
		t.Fatal(err)
	}
	expect(t, e.client().do("POST", "/api/setup", map[string]string{"token": e.token, "login": "o", "password": "secret-pass"}),
		http.StatusForbidden, "old bootstrap token")
	e.token = second
	e.setupOwner()
}

func TestUnauthorized(t *testing.T) {
	e := newEnv(t)
	e.setupOwner()
	c := e.client()
	for _, p := range []string{"/api/me", "/api/unknown", "/api/dashboards"} {
		expect(t, c.do("GET", p, nil), http.StatusUnauthorized, p)
	}
	expect(t, c.do("POST", "/api/logout", nil), http.StatusUnauthorized, "logout")
	c.cookie = &http.Cookie{Name: api.SessionCookie, Value: "forged"}
	expect(t, c.do("GET", "/api/me", nil), http.StatusUnauthorized, "forged cookie")
}

func TestLoginLogout(t *testing.T) {
	e := newEnv(t)
	first := e.setupOwner()
	c := e.client()
	expect(t, c.do("POST", "/api/login", map[string]string{"login": "owner", "password": "wrong-pass"}), http.StatusUnauthorized, "wrong password")
	expect(t, c.do("POST", "/api/login", map[string]string{"login": "nobody", "password": "wrong-pass"}), http.StatusUnauthorized, "unknown login")
	r := c.do("POST", "/api/login", map[string]string{"login": " OWNER ", "password": "secret-pass"})
	expect(t, r, http.StatusOK, "login")
	ck := c.cookie
	if ck == nil || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie %+v", ck)
	}
	if c.csrf == first.csrf || c.cookie.Value == first.cookie.Value {
		t.Fatal("login must create a new session")
	}
	expect(t, c.do("GET", "/api/me", nil), http.StatusOK, "me after login")

	oldCookie, oldCSRF := c.cookie, c.csrf
	expect(t, c.do("POST", "/api/logout", nil), http.StatusNoContent, "logout")
	if c.cookie != nil {
		t.Fatal("cookie not cleared")
	}
	// после выхода старые токен сессии и CSRF не работают
	c.cookie, c.csrf = oldCookie, oldCSRF
	expect(t, c.do("GET", "/api/me", nil), http.StatusUnauthorized, "old session")
	expect(t, c.do("POST", "/api/logout", nil), http.StatusUnauthorized, "old session logout")
	// первая сессия жива
	expect(t, first.do("GET", "/api/me", nil), http.StatusOK, "other session")
	// CSRF вышедшей сессии не годится и для живой
	stale := &client{e: e, cookie: first.cookie, csrf: oldCSRF}
	expect(t, stale.do("POST", "/api/logout", nil), http.StatusForbidden, "csrf of logged out session")
}

func TestCSRF(t *testing.T) {
	e := newEnv(t)
	a := e.setupOwner()
	b := e.client()
	expect(t, b.do("POST", "/api/login", map[string]string{"login": "owner", "password": "secret-pass"}), http.StatusOK, "login b")
	body := map[string]string{"current": "secret-pass", "new": "another-pass"}

	cases := map[string]*client{
		"no csrf":                  {e: e, cookie: a.cookie},
		"wrong csrf":               {e: e, cookie: a.cookie, csrf: "wrong"},
		"csrf of another session":  {e: e, cookie: a.cookie, csrf: b.csrf},
		"csrf token as the cookie": {e: e, cookie: a.cookie, csrf: a.cookie.Value},
	}
	for name, c := range cases {
		expect(t, c.do("POST", "/api/me/password", body), http.StatusForbidden, name)
	}
	textPlain := func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }
	expect(t, a.do("POST", "/api/me/password", body, textPlain), http.StatusForbidden, "text/plain")
	foreign := func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }
	expect(t, a.do("POST", "/api/me/password", body, foreign), http.StatusForbidden, "foreign origin")
	expect(t, e.client().do("POST", "/api/login", map[string]string{"login": "owner", "password": "secret-pass"}, foreign),
		http.StatusForbidden, "login from foreign origin")
	same := func(r *http.Request) { r.Header.Set("Origin", e.srv.URL) }
	expect(t, a.do("POST", "/api/me/password", body, same), http.StatusNoContent, "same origin")
}

func TestChangePasswordEndsOtherSessions(t *testing.T) {
	e := newEnv(t)
	a := e.setupOwner()
	b := e.client()
	expect(t, b.do("POST", "/api/login", map[string]string{"login": "owner", "password": "secret-pass"}), http.StatusOK, "login b")

	expect(t, a.do("POST", "/api/me/password", map[string]string{"current": "wrong-pass", "new": "another-pass"}),
		http.StatusBadRequest, "wrong current")
	expect(t, a.do("POST", "/api/me/password", map[string]string{"current": "secret-pass", "new": "another-pass"}),
		http.StatusNoContent, "change")
	expect(t, a.do("GET", "/api/me", nil), http.StatusOK, "current session kept")
	expect(t, b.do("GET", "/api/me", nil), http.StatusUnauthorized, "other session ended")
	expect(t, e.client().do("POST", "/api/login", map[string]string{"login": "owner", "password": "secret-pass"}),
		http.StatusUnauthorized, "old password")
	expect(t, e.client().do("POST", "/api/login", map[string]string{"login": "owner", "password": "another-pass"}),
		http.StatusOK, "new password")
}

func TestResetPassword(t *testing.T) {
	e := newEnv(t)
	a := e.setupOwner()
	ctx := context.Background()
	if err := e.auth.ResetPassword(ctx, "nobody", "whatever-pass"); err == nil {
		t.Fatal("reset of unknown login")
	}
	if err := e.auth.ResetPassword(ctx, "Owner", "console-pass"); err != nil {
		t.Fatal(err)
	}
	expect(t, a.do("GET", "/api/me", nil), http.StatusUnauthorized, "sessions after reset")
	expect(t, e.client().do("POST", "/api/login", map[string]string{"login": "owner", "password": "console-pass"}),
		http.StatusOK, "login after reset")
}

func TestOnlyHashesInDB(t *testing.T) {
	e := newEnv(t)
	a := e.setupOwner()
	ctx := context.Background()
	var n int
	// открытые токены нигде не лежат: ищем их во всех текстовых и бинарных колонках
	for _, tok := range []string{e.token, a.cookie.Value, a.csrf} {
		q := `SELECT (SELECT count(*) FROM session WHERE token_hash = convert_to($1, 'UTF8') OR user_agent = $1)
			+ (SELECT count(*) FROM bootstrap WHERE token_hash = convert_to($1, 'UTF8'))
			+ (SELECT count(*) FROM account WHERE password_hash LIKE '%' || $1 || '%')`
		if err := e.pool.QueryRow(ctx, q, tok).Scan(&n); err != nil || n != 0 {
			t.Fatalf("plain token in db: %d %v", n, err)
		}
	}
	if err := e.pool.QueryRow(ctx, "SELECT count(*) FROM session WHERE token_hash = $1", auth.HashToken(a.cookie.Value)).Scan(&n); err != nil || n != 1 {
		t.Fatalf("session hash: %d %v", n, err)
	}
	var hash string
	if err := e.pool.QueryRow(ctx, "SELECT password_hash FROM account").Scan(&hash); err != nil || !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("password hash %q %v", hash, err)
	}
}

func TestLoginRateLimit(t *testing.T) {
	e := newEnv(t)
	e.setupOwner()
	c := e.client()
	var last int
	for range 12 {
		last = c.do("POST", "/api/login", map[string]string{"login": "owner", "password": "wrong-pass"}).status
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("status after many failures: %d", last)
	}
	expect(t, c.do("POST", "/api/login", map[string]string{"login": "owner", "password": "secret-pass"}),
		http.StatusTooManyRequests, "correct password while limited")
}

func (c *client) getJSON(path string, v any) {
	c.e.t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, c.e.srv.URL+path, nil)
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		c.e.t.Fatalf("GET %s: %d", path, res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		c.e.t.Fatal(err)
	}
}

func (c *client) doList(path string) []map[string]any {
	c.e.t.Helper()
	var out []map[string]any
	c.getJSON(path, &out)
	return out
}
