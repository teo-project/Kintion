package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"kintion/server/internal/api"
)

// testPNG — 40×30 с цветными четвертями (разные обрезки дают разные превью).
func testPNG(t *testing.T) []byte {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for y := range 30 {
		for x := range 40 {
			m.Set(x, y, color.RGBA{R: uint8(x * 6), G: uint8(y * 8), B: 128, A: 255}) //nolint:gosec // < 256
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, m); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// rawResp — код и заголовки ответа.
type rawResp struct {
	StatusCode int
	Header     http.Header
}

// raw — запрос с произвольным телом и заголовками; тело ответа как есть.
func (c *client) raw(method, path string, body io.Reader, header map[string]string) (rawResp, []byte) {
	c.e.t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, c.e.srv.URL+path, body)
	if err != nil {
		c.e.t.Fatal(err)
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if c.csrf != "" {
		req.Header.Set(api.CSRFHeader, c.csrf)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	data, _ := io.ReadAll(res.Body)
	return rawResp{StatusCode: res.StatusCode, Header: res.Header}, data
}

func (c *client) upload(dash string, content []byte) (int, map[string]any) {
	c.e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "photo.bin")
	_, _ = fw.Write(content)
	_ = mw.Close()
	res, data := c.raw("POST", "/api/dashboards/"+dash+"/files", &buf, map[string]string{"Content-Type": mw.FormDataContentType()})
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return res.StatusCode, out
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func TestFiles(t *testing.T) {
	e := newEnv(t, "kin.core")
	c := e.setupOwner()
	dash := dashboardOf(t, c)
	img := testPNG(t)

	st, f := c.upload(dash, img)
	if st != http.StatusCreated || f["mime"] != "image/png" || f["width"] != float64(40) || f["height"] != float64(30) {
		t.Fatalf("upload %d %v", st, f)
	}
	sha := f["sha256"].(string)
	// повторная загрузка — та же строка
	if st, again := c.upload(dash, img); st != http.StatusOK || again["sha256"] != sha || again["created_at"] != f["created_at"] {
		t.Fatalf("again %d %v", st, again)
	}
	var n int
	if err := e.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_log WHERE object = 'file' AND op = 'create'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit %d %v", n, err)
	}
	if _, err := os.Stat(filepath.Join(e.storage, "files", sha[:2], sha[2:4], sha)); err != nil {
		t.Fatal(err)
	}

	// не изображение и слишком большой файл
	if st, _ := c.upload(dash, []byte("%PDF-1.4 не картинка")); st != http.StatusUnsupportedMediaType {
		t.Fatalf("pdf %d", st)
	}
	if st, _ := c.upload(dash, make([]byte, testUploadMax+10)); st != http.StatusRequestEntityTooLarge {
		t.Fatalf("large %d", st)
	}

	// раздача: ETag, кеш, 304
	path := "/api/files/" + dash + "/" + sha
	res, body := c.raw("GET", path, nil, nil)
	if res.StatusCode != http.StatusOK || !bytes.Equal(body, img) || res.Header.Get("ETag") != `"`+sha+`"` ||
		res.Header.Get("Cache-Control") != "private, max-age=31536000, immutable" || res.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("get %d %v", res.StatusCode, res.Header)
	}
	if res, _ := c.raw("GET", path, nil, map[string]string{"If-None-Match": `"` + sha + `"`}); res.StatusCode != http.StatusNotModified {
		t.Fatalf("If-None-Match %d", res.StatusCode)
	}
	// без сессии — 401
	if res, _ := e.client().raw("GET", path, nil, nil); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous %d", res.StatusCode)
	}

	// превью: одинаковые параметры — один файл кеша, разные обрезки — разные превью
	previews := filepath.Join(e.storage, "previews")
	p1 := path + "/preview?shape=square&x=0&y=0&w=20&h=20&size=64"
	r1, b1 := c.raw("GET", p1, nil, nil)
	r2, b2 := c.raw("GET", p1, nil, nil)
	if r1.StatusCode != http.StatusOK || r2.StatusCode != http.StatusOK || !bytes.Equal(b1, b2) || countFiles(t, previews) != 1 ||
		r1.Header.Get("Content-Type") != "image/jpeg" || r1.Header.Get("ETag") == "" {
		t.Fatalf("preview %d %d files %d", r1.StatusCode, r2.StatusCode, countFiles(t, previews))
	}
	if res, _ := c.raw("GET", p1, nil, map[string]string{"If-None-Match": r1.Header.Get("ETag")}); res.StatusCode != http.StatusNotModified {
		t.Fatalf("preview 304: %d", res.StatusCode)
	}
	r3, b3 := c.raw("GET", path+"/preview?shape=square&x=20&y=10&w=20&h=20&size=64", nil, nil)
	if r3.StatusCode != http.StatusOK || bytes.Equal(b1, b3) || countFiles(t, previews) != 2 {
		t.Fatalf("other crop %d files %d", r3.StatusCode, countFiles(t, previews))
	}
	if r, _ := c.raw("GET", path+"/preview?shape=circle&size=128", nil, nil); r.StatusCode != http.StatusOK || r.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("circle %d", r.StatusCode)
	}
	for _, q := range []string{"shape=square&x=30&y=0&w=20&h=20", "shape=square&size=100", "shape=oval", "shape=square&x=0"} {
		if r, _ := c.raw("GET", path+"/preview?"+q, nil, nil); r.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d", q, r.StatusCode)
		}
	}

	// чужой дашборд и свой другой дашборд без этого файла — 404
	ctx := context.Background()
	other, foreign := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	if _, err := e.pool.Exec(ctx, "INSERT INTO account (id, login, password_hash) VALUES ($1, 'other', 'x')", other); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, "INSERT INTO dashboard (id, owner_id, name) VALUES ($1, $2, 'чужой')", foreign, other); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, "INSERT INTO file (id, dashboard_id, sha256, mime, size, width, height) VALUES ($1, $2, $3, 'image/png', 1, 40, 30)",
		uuid.Must(uuid.NewV7()), foreign, sha); err != nil {
		t.Fatal(err)
	}
	r := c.do("POST", "/api/dashboards", map[string]string{"name": "второй"})
	expect(t, r, http.StatusCreated, "second dashboard")
	second := r.body["id"].(string)
	for _, d := range []string{foreign.String(), second, "not-a-uuid"} {
		if res, _ := c.raw("GET", "/api/files/"+d+"/"+sha, nil, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("file in %s: %d", d, res.StatusCode)
		}
		if res, _ := c.raw("GET", "/api/files/"+d+"/"+sha+"/preview?shape=square", nil, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("preview in %s: %d", d, res.StatusCode)
		}
	}
	if st, _ := c.upload(foreign.String(), img); st != http.StatusNotFound {
		t.Fatalf("upload to foreign %d", st)
	}
	if res, _ := c.raw("GET", "/api/files/"+dash+"/"+sha[:60], nil, nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("bad sha %d", res.StatusCode)
	}
}
