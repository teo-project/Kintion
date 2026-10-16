package api

import (
	"errors"
	"net"
	"net/http"

	"github.com/google/uuid"

	"kintion/server/internal/auth"
	"kintion/server/internal/httpx"
)

// AccountDTO — аккаунт в ответах.
type AccountDTO struct {
	ID      uuid.UUID `json:"id"`
	Login   string    `json:"login"`
	IsAdmin bool      `json:"is_admin"`
}

// MeDTO — ответ /api/setup, /api/login, /api/me: аккаунт и CSRF-токен сессии.
type MeDTO struct {
	Account AccountDTO `json:"account"`
	CSRF    string     `json:"csrf"`
}

func meOf(s auth.Session) MeDTO {
	return MeDTO{
		Account: AccountDTO{ID: s.Account.ID, Login: s.Account.Login, IsAdmin: s.Account.IsAdmin},
		CSRF:    s.CSRF(),
	}
}

func (a *API) routes() {
	a.mux.HandleFunc("GET /api/setup", a.setupStatus)
	a.mux.HandleFunc("POST /api/setup", a.setup)
	a.mux.HandleFunc("POST /api/login", a.login)
	a.mux.HandleFunc("POST /api/logout", a.logout)
	a.mux.HandleFunc("GET /api/me", a.me)
	a.mux.HandleFunc("POST /api/me/password", a.changePassword)
}

func (a *API) setCookie(w http.ResponseWriter, s auth.Session) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure — по https в TEO_KIN_PUBLIC_URL (§5): по http cookie с Secure не вернётся
		Name: SessionCookie, Value: s.Token, Path: "/",
		MaxAge: int(auth.SessionTTL.Seconds()), HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode,
	})
}

func (a *API) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure — по https в TEO_KIN_PUBLIC_URL (§5): по http cookie с Secure не вернётся
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode,
	})
}

// authError переводит ошибки auth в ответ; true — ответ записан.
func (a *API) authError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *auth.ValidationError
	switch {
	case errors.As(err, &ve):
		httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: ve.Field, Message: ve.Message})
	case errors.Is(err, auth.ErrSetupDone):
		httpx.WriteError(w, http.StatusConflict, "setup_done", err.Error())
	case errors.Is(err, auth.ErrBadToken):
		httpx.WriteError(w, http.StatusForbidden, "bad_token", err.Error())
	case errors.Is(err, auth.ErrBadCredentials):
		httpx.WriteError(w, http.StatusUnauthorized, "bad_credentials", err.Error())
	case errors.Is(err, auth.ErrTooManyAttempts):
		httpx.WriteError(w, http.StatusTooManyRequests, "too_many_attempts", err.Error())
	default:
		a.internal(w, r, err)
	}
}

func (a *API) setupStatus(w http.ResponseWriter, r *http.Request) {
	needed, err := a.d.Auth.NeedsSetup(r.Context())
	if err != nil {
		a.internal(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"needed": needed})
}

func (a *API) setup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	sess, err := a.d.Auth.Setup(r.Context(), in.Token, in.Login, in.Password, r.UserAgent(), a.firstDashboard(r))
	if err != nil {
		a.authError(w, r, err)
		return
	}
	a.setCookie(w, sess)
	httpx.WriteJSON(w, http.StatusCreated, meOf(sess))
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	sess, err := a.d.Auth.Login(r.Context(), in.Login, in.Password, r.UserAgent(), clientIP(r))
	if err != nil {
		a.authError(w, r, err)
		return
	}
	// старая сессия этого браузера, если была, больше не нужна
	if c, err := r.Cookie(SessionCookie); err == nil {
		if old, err := a.d.Auth.Resolve(r.Context(), c.Value); err == nil {
			_ = a.d.Auth.Logout(r.Context(), old)
		}
	}
	a.setCookie(w, sess)
	httpx.WriteJSON(w, http.StatusOK, meOf(sess))
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	sess, _ := SessionFrom(r.Context())
	if err := a.d.Auth.Logout(r.Context(), sess); err != nil {
		a.internal(w, r, err)
		return
	}
	a.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	sess, _ := SessionFrom(r.Context())
	httpx.WriteJSON(w, http.StatusOK, meOf(sess))
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !decode(w, r, &in) {
		return
	}
	sess, _ := SessionFrom(r.Context())
	if err := a.d.Auth.ChangePassword(r.Context(), sess, in.Current, in.New); err != nil {
		a.authError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
