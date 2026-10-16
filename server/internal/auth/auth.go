// Package auth — аккаунты, пароли (argon2id), сессии и первый запуск
// по bootstrap-токену (F-26, F-27, F-28). В 0.1 аккаунт один — владелец (Д-6).
//
// В БД хранятся только хеши токенов (сессии, bootstrap). CSRF-токен не
// хранится вовсе: он выводится из токена сессии (HMAC), поэтому привязан
// к сессии и не годится для другой (Д-13).
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kintion/server/internal/db/sqlc"
)

// SessionTTL — срок жизни сессии.
const SessionTTL = 30 * 24 * time.Hour

// Ошибки сервиса.
var (
	ErrSetupDone       = errors.New("настройка уже выполнена")
	ErrBadToken        = errors.New("неверный токен первого запуска")
	ErrBadCredentials  = errors.New("неверный логин или пароль")
	ErrTooManyAttempts = errors.New("слишком много попыток входа, попробуйте позже")
	ErrNoSession       = errors.New("нет сессии")
	ErrNoAccount       = errors.New("нет такого аккаунта")
)

// ValidationError — неверные данные формы (логин, пароль).
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// MinPasswordLen — минимальная длина пароля.
const MinPasswordLen = 8

// Session — действующая сессия с аккаунтом.
type Session struct {
	ID      uuid.UUID
	Token   string // открытый токен — только в памяти и cookie
	Account sqlc.Account
}

// CSRF — токен для заголовка X-Kintion-CSRF этой сессии.
func (s Session) CSRF() string { return CSRFToken(s.Token) }

// CSRFToken выводит CSRF-токен из токена сессии.
func CSRFToken(sessionToken string) string {
	m := hmac.New(sha256.New, []byte(sessionToken))
	m.Write([]byte("kintion-csrf"))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// CheckCSRF сверяет токен из заголовка за постоянное время.
func (s Session) CheckCSRF(header string) bool {
	return header != "" && subtle.ConstantTimeCompare([]byte(header), []byte(s.CSRF())) == 1
}

// Service — вход и сессии.
type Service struct {
	pool    *pgxpool.Pool
	limiter *limiter
	now     func() time.Time
}

// New создаёт сервис.
func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, limiter: newLimiter(10, 15*time.Minute), now: time.Now}
}

func newToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read не возвращает ошибок
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken — хеш токена для хранения в БД.
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// NewID — новый UUIDv7 (Д-8).
func NewID() uuid.UUID { return uuid.Must(uuid.NewV7()) }

// EnsureBootstrap: если аккаунтов нет — создаёт новый bootstrap-токен
// (старые неиспользованные удаляются, их открытый вид не сохранился) и
// возвращает его для журнала. Если аккаунт есть — пустая строка.
func (s *Service) EnsureBootstrap(ctx context.Context) (string, error) {
	var token string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if _, err := tx.Exec(ctx, "LOCK TABLE bootstrap IN EXCLUSIVE MODE"); err != nil {
			return err
		}
		n, err := q.CountAccounts(ctx)
		if err != nil || n > 0 {
			return err
		}
		if err := q.DeleteUnusedBootstrap(ctx); err != nil {
			return err
		}
		token = newToken()
		return q.InsertBootstrap(ctx, HashToken(token))
	})
	return token, err
}

// NeedsSetup — аккаунтов ещё нет.
func (s *Service) NeedsSetup(ctx context.Context) (bool, error) {
	n, err := sqlc.New(s.pool).CountAccounts(ctx)
	return n == 0, err
}

func normLogin(login string) string { return strings.ToLower(strings.TrimSpace(login)) }

func validate(login, password string) error {
	if login == "" || utf8.RuneCountInString(login) > 64 || strings.ContainsAny(login, " \t\r\n") {
		return &ValidationError{Field: "login", Message: "логин: от 1 до 64 символов без пробелов"}
	}
	return validatePassword(password)
}

func validatePassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLen || len(password) > 1024 {
		return &ValidationError{Field: "password", Message: fmt.Sprintf("пароль: не короче %d символов", MinPasswordLen)}
	}
	return nil
}

// SetupHook — что ещё сделать в транзакции создания владельца (первый дашборд).
type SetupHook func(ctx context.Context, tx pgx.Tx, account sqlc.Account) error

// Setup создаёт владельца по bootstrap-токену — один раз — и открывает сессию.
func (s *Service) Setup(ctx context.Context, token, login, password, userAgent string, hook SetupHook) (Session, error) {
	login = normLogin(login)
	if err := validate(login, password); err != nil {
		return Session{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return Session{}, err
	}
	var sess Session
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if _, err := tx.Exec(ctx, "LOCK TABLE bootstrap IN EXCLUSIVE MODE"); err != nil {
			return err
		}
		if n, err := q.CountAccounts(ctx); err != nil {
			return err
		} else if n > 0 {
			return ErrSetupDone
		}
		b, err := q.LockBootstrap(ctx, HashToken(token))
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && b.UsedAt != nil) {
			return ErrBadToken
		} else if err != nil {
			return err
		}
		acc, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{ID: NewID(), Login: login, PasswordHash: hash, IsAdmin: true})
		if err != nil {
			return err
		}
		if err := q.MarkBootstrapUsed(ctx, b.TokenHash); err != nil {
			return err
		}
		if hook != nil {
			if err := hook(ctx, tx, acc); err != nil {
				return err
			}
		}
		sess, err = s.createSession(ctx, q, acc, userAgent)
		return err
	})
	return sess, err
}

func (s *Service) createSession(ctx context.Context, q *sqlc.Queries, acc sqlc.Account, userAgent string) (Session, error) {
	token := newToken()
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	row, err := q.CreateSession(ctx, sqlc.CreateSessionParams{
		ID: NewID(), AccountID: acc.ID, TokenHash: HashToken(token),
		ExpiresAt: s.now().Add(SessionTTL), UserAgent: strings.ToValidUTF8(userAgent, ""),
	})
	if err != nil {
		return Session{}, err
	}
	return Session{ID: row.ID, Token: token, Account: acc}, nil
}

// dummyHash — чтобы неизвестный логин проверялся так же долго, как известный.
var dummyHash, _ = HashPassword("kintion-dummy-password")

// Login проверяет пароль и создаёт новую сессию (без фиксации старой).
// client — адрес клиента для ограничения частоты попыток.
func (s *Service) Login(ctx context.Context, login, password, userAgent, client string) (Session, error) {
	login = normLogin(login)
	keys := []string{"ip:" + client, "login:" + login}
	if !s.limiter.allow(keys...) {
		return Session{}, ErrTooManyAttempts
	}
	q := sqlc.New(s.pool)
	acc, err := q.GetAccountByLogin(ctx, login)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = CheckPassword(dummyHash, password)
		s.limiter.fail(keys...)
		return Session{}, ErrBadCredentials
	} else if err != nil {
		return Session{}, err
	}
	ok, err := CheckPassword(acc.PasswordHash, password)
	if err != nil {
		return Session{}, err
	}
	if !ok {
		s.limiter.fail(keys...)
		return Session{}, ErrBadCredentials
	}
	s.limiter.reset("login:" + login)
	_ = q.DeleteExpiredSessions(ctx)
	return s.createSession(ctx, q, acc, userAgent)
}

// Resolve находит действующую сессию по открытому токену из cookie.
func (s *Service) Resolve(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrNoSession
	}
	q := sqlc.New(s.pool)
	row, err := q.GetSessionByTokenHash(ctx, HashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNoSession
	} else if err != nil {
		return Session{}, err
	}
	if s.now().Sub(row.Session.LastSeenAt) > time.Minute {
		_ = q.TouchSession(ctx, row.Session.ID)
	}
	return Session{ID: row.Session.ID, Token: token, Account: row.Account}, nil
}

// Logout удаляет сессию.
func (s *Service) Logout(ctx context.Context, sess Session) error {
	return sqlc.New(s.pool).DeleteSession(ctx, sess.ID)
}

// ChangePassword меняет пароль после проверки текущего и завершает все
// остальные сессии аккаунта.
func (s *Service) ChangePassword(ctx context.Context, sess Session, current, next string) error {
	if err := validatePassword(next); err != nil {
		return err
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		acc, err := q.GetAccount(ctx, sess.Account.ID)
		if err != nil {
			return err
		}
		ok, err := CheckPassword(acc.PasswordHash, current)
		if err != nil {
			return err
		}
		if !ok {
			return &ValidationError{Field: "current", Message: "текущий пароль неверен"}
		}
		if err := q.UpdatePasswordHash(ctx, sqlc.UpdatePasswordHashParams{ID: acc.ID, PasswordHash: hash}); err != nil {
			return err
		}
		return q.DeleteOtherSessions(ctx, sqlc.DeleteOtherSessionsParams{AccountID: acc.ID, ID: sess.ID})
	})
}

// ResetPassword — восстановление доступа из консоли (В-4): задаёт новый
// пароль и завершает все сессии аккаунта.
func (s *Service) ResetPassword(ctx context.Context, login, password string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		acc, err := q.GetAccountByLogin(ctx, normLogin(login))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoAccount
		} else if err != nil {
			return err
		}
		if err := q.UpdatePasswordHash(ctx, sqlc.UpdatePasswordHashParams{ID: acc.ID, PasswordHash: hash}); err != nil {
			return err
		}
		return q.DeleteAccountSessions(ctx, acc.ID)
	})
}

// GeneratePassword — случайный пароль для сброса из консоли.
func GeneratePassword() string { return newToken()[:20] }
