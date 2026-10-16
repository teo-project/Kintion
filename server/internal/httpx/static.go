package httpx

import (
	"bytes"
	"errors"
	"html"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

const (
	cacheImmutable  = "public, max-age=31536000, immutable"
	cacheRevalidate = "no-cache"
)

// spa отдаёт собранный фронтенд: файлы из dist, ассеты с хешами в именах
// (assets/) — immutable; любой другой путь — index.html (маршруты Vue).
// lang — язык инстанса: подставляется в <html lang> (по нему фронтенд выбирает
// языковой файл, Д-16).
func spa(dist fs.FS, lang string) http.Handler {
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		index = nil
	}
	if index != nil && lang != "" {
		index = bytes.Replace(index, []byte(`<html lang="ru">`),
			[]byte(`<html lang="`+html.EscapeString(lang)+`">`), 1)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "метод не поддерживается")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")

		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name != "" && name != "index.html" {
			st, err := fs.Stat(dist, name)
			if hasDotSegment(name) || name == legacyLicenses {
				st, err = nil, fs.ErrNotExist
			}
			if err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", cacheImmutable)
				} else {
					w.Header().Set("Cache-Control", cacheRevalidate)
				}
				http.ServeFileFS(w, r, dist, name) //nolint:gosec // fs.FS не выходит за свой корень, путь очищен path.Clean
				return
			} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			// пропавший ассет старой сборки — не index.html, иначе браузер получит HTML вместо JS
			if strings.HasPrefix(name, "assets/") {
				http.NotFound(w, r)
				return
			}
		}

		if index == nil {
			http.Error(w, "Kintion: интерфейс не собран (npm run build в web/)", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Cache-Control", cacheRevalidate)
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
	})
}

// legacyLicenses — тексты лицензий из старых сборок (web/public): теперь они
// только за входом (GET /api/licenses, F-91), даже если файл остался в dist.
const legacyLicenses = "THIRD_PARTY_LICENSES.txt"

// hasDotSegment — путь содержит скрытый файл или папку (.gitkeep и т. п.).
func hasDotSegment(name string) bool {
	for seg := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}
