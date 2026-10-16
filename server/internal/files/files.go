// Package files — файлы на локальном томе (F-29): изображения записей и их
// превью. Имя файла — sha256 содержимого, путь STORAGE_PATH/files/ab/cd/<sha256>;
// запись — во временный файл и атомарное переименование. Доступ проверяет
// вызывающий через internal/authz (F-30, Д-25).
package files

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // декодеры форматов Д-26
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	_ "golang.org/x/image/webp"
)

// Ошибки загрузки.
var (
	// ErrTooLarge — файл больше TEO_KIN_UPLOAD_MAX_MB или изображение больше MaxPixels.
	ErrTooLarge = errors.New("файл слишком большой")
	// ErrUnsupported — не изображение поддерживаемого формата (Д-26).
	ErrUnsupported = errors.New("поддерживаются только изображения JPEG, PNG, WebP, GIF")
)

// MaxPixels — предел размера изображения в пикселях: защита от «бомб»
// (маленький файл, огромная картинка при декодировании).
const MaxPixels = 50_000_000

// Mimes — допустимые типы, определяются по содержимому (Д-26).
var Mimes = map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true}

var shaRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidSHA — строка похожа на имя файла (64 hex-символа в нижнем регистре).
func ValidSHA(s string) bool { return shaRe.MatchString(s) }

// Store — файлы на томе.
type Store struct {
	Root     string // TEO_KIN_STORAGE_PATH
	MaxBytes int64  // TEO_KIN_UPLOAD_MAX_MB в байтах
}

// Info — сведения о сохранённом файле. Width и Height — с учётом ориентации EXIF.
type Info struct {
	SHA256 string
	MIME   string
	Size   int64
	Width  int
	Height int
}

func shard(dir, sha string) string { return filepath.Join(dir, sha[:2], sha[2:4]) }

// Path — путь оригинала на томе.
func (s *Store) Path(sha string) string {
	return filepath.Join(shard(filepath.Join(s.Root, "files"), sha), sha)
}

func (s *Store) tmpDir() (string, error) {
	dir := filepath.Join(s.Root, "tmp")
	return dir, os.MkdirAll(dir, 0o750)
}

// Put сохраняет содержимое r: проверяет размер и тип, пишет во временный
// файл и переносит на место. Повторная загрузка того же содержимого — тот же путь.
func (s *Store) Put(r io.Reader) (Info, error) {
	dir, err := s.tmpDir()
	if err != nil {
		return Info{}, err
	}
	f, err := os.CreateTemp(dir, "upload-*")
	if err != nil {
		return Info{}, err
	}
	tmp := f.Name()
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmp) // после переноса файла уже нет
	}()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, s.MaxBytes+1))
	if err != nil {
		return Info{}, err
	}
	if n > s.MaxBytes {
		return Info{}, ErrTooLarge
	}
	info := Info{SHA256: hex.EncodeToString(h.Sum(nil)), Size: n}

	head := make([]byte, 512)
	k, _ := f.ReadAt(head, 0)
	info.MIME = http.DetectContentType(head[:k])
	if !Mimes[info.MIME] {
		return Info{}, ErrUnsupported
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return Info{}, err
	}
	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return Info{}, ErrUnsupported
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return Info{}, ErrTooLarge
	}
	// заголовок может быть цел, а данные — обрезаны: декодируем полностью,
	// чтобы битый файл не стал оригиналом без превью
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return Info{}, err
	}
	if _, _, err := image.Decode(f); err != nil {
		return Info{}, ErrUnsupported
	}
	info.Width, info.Height = cfg.Width, cfg.Height
	if info.MIME == "image/jpeg" {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return Info{}, err
		}
		if swapsAxes(jpegOrientation(f)) {
			info.Width, info.Height = info.Height, info.Width
		}
	}
	if err := f.Sync(); err != nil {
		return Info{}, err
	}
	if err := f.Close(); err != nil {
		return Info{}, err
	}

	dst := s.Path(info.SHA256)
	if _, err := os.Stat(dst); err == nil {
		return info, nil // такой файл уже есть (тот же хеш — то же содержимое)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return Info{}, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return Info{}, fmt.Errorf("files: %w", err)
	}
	return info, nil
}

// Open открывает оригинал.
func (s *Store) Open(sha string) (*os.File, error) {
	if !ValidSHA(sha) {
		return nil, os.ErrNotExist
	}
	return os.Open(s.Path(sha))
}
