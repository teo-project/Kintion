package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params — параметры argon2id (F-27: пароли argon2id).
type Argon2Params struct {
	Memory  uint32 // КиБ
	Time    uint32
	Threads uint8
}

// Params — параметры для новых хешей. Тесты ставят облегчённые.
var Params = Argon2Params{Memory: 64 * 1024, Time: 3, Threads: 2}

const (
	saltLen = 16
	keyLen  = 32
)

var b64 = base64.RawStdEncoding

// HashPassword — строка вида $argon2id$v=19$m=65536,t=3,p=2$<соль>$<хеш>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	p := Params
	key := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Time, p.Threads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

var errBadHash = errors.New("неверный формат хеша пароля")

// CheckPassword сверяет пароль с хешем за постоянное время.
func CheckPassword(hash, password string) (bool, error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errBadHash
	}
	var v int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil || v != argon2.Version {
		return false, errBadHash
	}
	var p Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return false, errBadHash
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false, errBadHash
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, errBadHash
	}
	got := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, uint32(len(want))) //nolint:gosec // длина ключа — из нашего же хеша, 32 байта
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
