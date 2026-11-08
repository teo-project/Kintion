package auth

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"kintion/server/internal/db/sqlc"
)

// Регистрация (F-28): open — любой, invite — по приглашению админа,
// invite-all — по приглашению любого пользователя, closed — аккаунты заводит
// только админ.
const (
	RegistrationOpen      = "open"
	RegistrationInvite    = "invite"
	RegistrationInviteAll = "invite-all"
	RegistrationClosed    = "closed"
)

// ByInvite — в режиме mode регистрируются по ссылке-приглашению.
func ByInvite(mode string) bool { return mode == RegistrationInvite || mode == RegistrationInviteAll }

// CanInvite — может ли аккаунт создавать ссылки-приглашения в режиме mode.
func CanInvite(mode string, admin bool) bool {
	return mode == RegistrationInviteAll || (mode == RegistrationInvite && admin)
}

// InviteTTL — срок действия приглашения на инстанс.
const InviteTTL = 7 * 24 * time.Hour

// Ошибки регистрации.
var (
	ErrRegistrationClosed = errors.New("регистрация закрыта")
	ErrBadInvite          = errors.New("приглашение недействительно или уже использовано")
)

// NormNick — ник без пробелов по краям; пустой — nil (ника нет).
func NormNick(nick string) *string {
	n := strings.TrimSpace(nick)
	if n == "" {
		return nil
	}
	return &n
}

// validateNick — ник: от 2 до 32 символов, буквы, цифры, «.», «_», «-» (F-03:
// адрес для приглашений, вводится руками).
func validateNick(nick *string) error {
	if nick == nil {
		return nil
	}
	n := utf8.RuneCountInString(*nick)
	ok := n >= 2 && n <= 32
	for _, r := range *nick {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("._-", r) {
			ok = false
		}
	}
	if !ok {
		return &ValidationError{Field: "nick", Message: "ник: от 2 до 32 символов — буквы, цифры, «.», «_», «-»"}
	}
	return nil
}

// uniqueErr переводит нарушение уникальности логина или ника в ошибку поля.
func uniqueErr(err error) error {
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "23505" {
		return err
	}
	switch pe.ConstraintName {
	case "account_login_key":
		return &ValidationError{Field: "login", Message: "логин занят"}
	case "account_nick_idx":
		return &ValidationError{Field: "nick", Message: "ник занят"}
	}
	return err
}

// NewAccount — данные нового аккаунта.
type NewAccount struct {
	Login    string
	Password string
	Nick     string
}

func (n NewAccount) prepare() (string, *string, string, error) {
	login, nick := normLogin(n.Login), NormNick(n.Nick)
	if err := validate(login, n.Password); err != nil {
		return "", nil, "", err
	}
	if err := validateNick(nick); err != nil {
		return "", nil, "", err
	}
	hash, err := HashPassword(n.Password)
	return login, nick, hash, err
}

// Register — регистрация по режиму mode (F-28): open — без приглашения,
// invite — по токену приглашения; создаёт аккаунт (не админ), выполняет hook
// (первый дашборд) и открывает сессию.
func (s *Service) Register(ctx context.Context, mode, token string, in NewAccount, userAgent string, hook SetupHook) (Session, error) {
	if mode != RegistrationOpen && !ByInvite(mode) {
		return Session{}, ErrRegistrationClosed
	}
	login, nick, hash, err := in.prepare()
	if err != nil {
		return Session{}, err
	}
	var sess Session
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		// до первичной настройки регистрации нет: первый аккаунт — админ по
		// bootstrap-токену (F-28); блокировка — та же, что у Setup
		if _, err := tx.Exec(ctx, "LOCK TABLE bootstrap IN SHARE MODE"); err != nil {
			return err
		}
		if n, err := q.CountAccounts(ctx); err != nil {
			return err
		} else if n == 0 {
			return ErrRegistrationClosed
		}
		var inv sqlc.InstanceInvite
		if ByInvite(mode) {
			var err error
			inv, err = q.LockInviteByToken(ctx, HashToken(token))
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && (inv.UsedAt != nil || !inv.ExpiresAt.After(s.now()))) {
				return ErrBadInvite
			} else if err != nil {
				return err
			}
		}
		acc, err := q.CreateAccountWithNick(ctx, sqlc.CreateAccountWithNickParams{ID: NewID(), Login: login, PasswordHash: hash, Nick: nick})
		if err != nil {
			return uniqueErr(err)
		}
		if ByInvite(mode) {
			if err := q.UseInvite(ctx, sqlc.UseInviteParams{ID: inv.ID, UsedBy: &acc.ID}); err != nil {
				return err
			}
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

// InviteValid — приглашение с токеном существует, не использовано и не истекло.
func (s *Service) InviteValid(ctx context.Context, token string) (bool, error) {
	inv, err := sqlc.New(s.pool).LockInviteByToken(ctx, HashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil && inv.UsedAt == nil && inv.ExpiresAt.After(s.now()), err
}

// CreateInvite — новое приглашение на инстанс от админа by; открытый токен
// возвращается один раз (в базе — хеш).
func (s *Service) CreateInvite(ctx context.Context, by uuid.UUID) (sqlc.InstanceInvite, string, error) {
	token := newToken()
	inv, err := sqlc.New(s.pool).InsertInvite(ctx, sqlc.InsertInviteParams{
		ID: NewID(), TokenHash: HashToken(token), CreatedBy: &by, ExpiresAt: s.now().Add(InviteTTL),
	})
	return inv, token, err
}

// CreateAccount — аккаунт, заведённый админом (интерфейс или консоль);
// hook — первый дашборд.
func (s *Service) CreateAccount(ctx context.Context, in NewAccount, hook SetupHook) (sqlc.Account, error) {
	login, nick, hash, err := in.prepare()
	if err != nil {
		return sqlc.Account{}, err
	}
	var acc sqlc.Account
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		var err error
		acc, err = q.CreateAccountWithNick(ctx, sqlc.CreateAccountWithNickParams{ID: NewID(), Login: login, PasswordHash: hash, Nick: nick})
		if err != nil {
			return uniqueErr(err)
		}
		if hook != nil {
			return hook(ctx, tx, acc)
		}
		return nil
	})
	return acc, err
}

// NickHook — действие в транзакции смены ника: аккаунт до и после.
type NickHook func(ctx context.Context, tx pgx.Tx, old, acc sqlc.Account) error

// SetNick задаёт или убирает (пустая строка) ник аккаунта; hook (может быть
// nil) — в той же транзакции.
func (s *Service) SetNick(ctx context.Context, account uuid.UUID, nick string, hook NickHook) (sqlc.Account, error) {
	n := NormNick(nick)
	if err := validateNick(n); err != nil {
		return sqlc.Account{}, err
	}
	var acc sqlc.Account
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		old, err := q.LockAccount(ctx, account)
		if err != nil {
			return err
		}
		if acc, err = q.SetAccountNick(ctx, sqlc.SetAccountNickParams{ID: account, Nick: n}); err != nil {
			return uniqueErr(err)
		}
		if hook != nil {
			return hook(ctx, tx, old, acc)
		}
		return nil
	})
	return acc, err
}
