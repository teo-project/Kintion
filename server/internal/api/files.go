package api

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kintion/server/internal/audit"
	"kintion/server/internal/authz"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/files"
	"kintion/server/internal/httpx"
)

// cacheFile — оригиналы и превью неизменны: имя — хеш, параметры — в адресе (F-30).
const cacheFile = "private, max-age=31536000, immutable"

// multipartSlack — запас на заголовки multipart сверх предела файла.
const multipartSlack = 1 << 20

func (a *API) fileRoutes() {
	a.mux.HandleFunc("POST /api/dashboards/{d}/files", a.uploadFile)
	a.mux.HandleFunc("GET /api/files/{d}/{sha}", a.getFile)
	a.mux.HandleFunc("GET /api/files/{d}/{sha}/preview", a.getPreview)
}

// FileDTO — загруженный файл; значение поля image хранит SHA256.
type FileDTO struct {
	SHA256    string    `json:"sha256"`
	MIME      string    `json:"mime"`
	Size      int64     `json:"size"`
	Width     *int32    `json:"width"`
	Height    *int32    `json:"height"`
	CreatedAt time.Time `json:"created_at"`
}

func fileDTO(f sqlc.File) FileDTO {
	return FileDTO{SHA256: f.Sha256, MIME: f.Mime, Size: f.Size, Width: f.Width, Height: f.Height, CreatedAt: f.CreatedAt}
}

// fileError — ответ на ошибку загрузки или раздачи.
func (a *API) fileError(w http.ResponseWriter, r *http.Request, err error) {
	var pe *files.ParamError
	var mbe *http.MaxBytesError
	switch {
	case errors.Is(err, authz.ErrNotFound), errors.Is(err, pgx.ErrNoRows), errors.Is(err, os.ErrNotExist):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "не найдено")
	case errors.Is(err, files.ErrTooLarge), errors.As(err, &mbe):
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "too_large", files.ErrTooLarge.Error())
	case errors.Is(err, files.ErrUnsupported):
		httpx.WriteError(w, http.StatusUnsupportedMediaType, "unsupported", err.Error())
	case errors.As(err, &pe):
		httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: pe.Param, Message: pe.Message})
	default:
		a.internal(w, r, err)
	}
}

// dashboardOf — дашборд из пути {d}, если он доступен (authz).
func (a *API) dashboardOf(r *http.Request) (sqlc.Dashboard, error) {
	sess, _ := SessionFrom(r.Context())
	id, err := pathID(r, "d")
	if err != nil {
		return sqlc.Dashboard{}, err
	}
	return authz.Dashboard(r.Context(), sqlc.New(a.d.Pool), sess.Account.ID, id)
}

// uploadFile — POST /api/dashboards/{d}/files, multipart с частью «file».
// Тот же файл повторно — та же строка (200), новый — 201.
func (a *API) uploadFile(w http.ResponseWriter, r *http.Request) {
	d, err := a.dashboardOf(r)
	if err != nil {
		a.fileError(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.d.Files.MaxBytes+multipartSlack)
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: "file", Message: "ожидается multipart/form-data с частью file"})
		return
	}
	var info files.Info
	found := false
	for {
		part, err := mr.NextPart()
		if err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				a.fileError(w, r, err)
				return
			}
			break
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}
		info, err = a.d.Files.Put(part)
		_ = part.Close()
		if err != nil {
			a.fileError(w, r, err)
			return
		}
		found = true
		break
	}
	if !found {
		httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: "file", Message: "нет части file"})
		return
	}

	sess, _ := SessionFrom(r.Context())
	ctx := r.Context()
	status := http.StatusCreated
	var row sqlc.File
	err = pgx.BeginFunc(ctx, a.d.Pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		width, height := int32(info.Width), int32(info.Height) //nolint:gosec // ограничено MaxPixels
		row, err = q.InsertFile(ctx, sqlc.InsertFileParams{
			ID: uuid.Must(uuid.NewV7()), DashboardID: d.ID, Sha256: info.SHA256, Mime: info.MIME,
			Size: info.Size, Width: &width, Height: &height, UploadedBy: &sess.Account.ID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			status = http.StatusOK
			row, err = q.GetFile(ctx, sqlc.GetFileParams{DashboardID: d.ID, Sha256: info.SHA256})
			return err
		}
		if err != nil {
			return err
		}
		return operation(r, sess.Account.ID).Write(ctx, q, audit.Entry{
			Dashboard: &d.ID, Object: "file", ObjectID: &row.ID, Op: audit.OpCreate, After: fileDTO(row),
		})
	})
	if err != nil {
		a.fileError(w, r, err)
		return
	}
	httpx.WriteJSON(w, status, fileDTO(row))
}

// fileOf — строка файла дашборда из пути; чужое и несуществующее — 404.
func (a *API) fileOf(r *http.Request) (sqlc.File, error) {
	d, err := a.dashboardOf(r)
	if err != nil {
		return sqlc.File{}, err
	}
	sha := r.PathValue("sha")
	if !files.ValidSHA(sha) {
		return sqlc.File{}, authz.ErrNotFound
	}
	return sqlc.New(a.d.Pool).GetFile(r.Context(), sqlc.GetFileParams{DashboardID: d.ID, Sha256: sha})
}

// serveFile отдаёт неизменный файл: ETag, долгий кеш, 304 на If-None-Match.
func serveFile(w http.ResponseWriter, r *http.Request, path, mime, etag string) error {
	f, err := os.Open(path) //nolint:gosec // путь собран из проверенного хеша
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Cache-Control", cacheFile)
	h.Set("Content-Type", mime)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	http.ServeContent(w, r, "", time.Time{}, f) // сам отвечает 304 по If-None-Match
	return nil
}

// getFile — GET /api/files/{d}/{sha256}: оригинал.
func (a *API) getFile(w http.ResponseWriter, r *http.Request) {
	row, err := a.fileOf(r)
	if err == nil {
		err = serveFile(w, r, a.d.Files.Path(row.Sha256), row.Mime, `"`+row.Sha256+`"`)
	}
	if err != nil {
		a.fileError(w, r, err)
	}
}

// getPreview — GET /api/files/{d}/{sha256}/preview?shape=&x=&y=&w=&h=&size=.
func (a *API) getPreview(w http.ResponseWriter, r *http.Request) {
	row, err := a.fileOf(r)
	if err != nil {
		a.fileError(w, r, err)
		return
	}
	c, err := files.ParsePreview(r.URL.Query())
	if err == nil && (row.Width == nil || row.Height == nil) {
		err = errors.New("у файла нет размеров: " + strconv.Quote(row.Sha256))
	}
	if err == nil {
		c, err = c.Fit(int(*row.Width), int(*row.Height))
	}
	var path string
	if err == nil {
		path, err = a.d.Files.Preview(row.Sha256, row.Mime, c)
	}
	if err == nil {
		err = serveFile(w, r, path, c.MIME(), c.ETag(row.Sha256))
	}
	if err != nil {
		a.fileError(w, r, err)
	}
}
