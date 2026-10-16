package files

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"net/url"
	"os"
	"strings"
	"testing"
)

// halves — 32×16: левая половина красная, правая синяя.
func halves() *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for y := range 16 {
		for x := range 32 {
			c := color.RGBA{R: 255, A: 255}
			if x >= 16 {
				c = color.RGBA{B: 255, A: 255}
			}
			m.Set(x, y, c)
		}
	}
	return m
}

// withOrientation — JPEG с сегментом EXIF Orientation.
func withOrientation(t *testing.T, img image.Image, o uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08\x00\x01\x01\x12\x00\x03\x00\x00\x00\x01")
	tiff = binary.BigEndian.AppendUint16(tiff, o)
	tiff = append(tiff, 0, 0, 0, 0, 0, 0)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1}
	seg = binary.BigEndian.AppendUint16(seg, uint16(len(payload)+2)) //nolint:gosec // маленький сегмент
	seg = append(seg, payload...)
	b := buf.Bytes()
	return append(append(append([]byte{}, b[:2]...), seg...), b[2:]...)
}

func isRed(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r > 0xC000 && g < 0x4000 && b < 0x4000
}

func TestPutOrientationPreview(t *testing.T) {
	s := &Store{Root: t.TempDir(), MaxBytes: 1 << 20}
	data := withOrientation(t, halves(), 6)
	info, err := s.Put(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	// поворот на 90° по часовой: 16×32, левая (красная) половина — сверху
	if info.MIME != "image/jpeg" || info.Width != 16 || info.Height != 32 || info.Size != int64(len(data)) {
		t.Fatalf("info %+v", info)
	}
	if _, err := os.Stat(s.Path(info.SHA256)); err != nil {
		t.Fatal(err)
	}
	c, err := ParsePreview(url.Values{"shape": {"square"}, "x": {"0"}, "y": {"0"}, "w": {"16"}, "h": {"16"}, "size": {"64"}})
	if err != nil {
		t.Fatal(err)
	}
	if c, err = c.Fit(info.Width, info.Height); err != nil {
		t.Fatal(err)
	}
	p, err := s.Preview(info.SHA256, info.MIME, c)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p) //nolint:gosec // путь теста
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 64 || !isRed(img.At(32, 32)) {
		t.Fatalf("preview %v, center %v", img.Bounds(), img.At(32, 32))
	}
	// повторная загрузка — тот же файл
	again, err := s.Put(bytes.NewReader(data))
	if err != nil || again != info {
		t.Fatalf("again %+v %v", again, err)
	}
}

func TestPutRejects(t *testing.T) {
	s := &Store{Root: t.TempDir(), MaxBytes: 1024}
	if _, err := s.Put(strings.NewReader("просто текст")); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("text: %v", err)
	}
	if _, err := s.Put(bytes.NewReader(make([]byte, 2048))); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("large: %v", err)
	}
	// цел заголовок, обрезаны данные
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, halves(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(bytes.NewReader(buf.Bytes()[:buf.Len()/2])); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("truncated: %v", err)
	}
}

func TestOrientAll(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 2)) // пиксель (0,0) красный
	src.Set(0, 0, color.RGBA{R: 255, A: 255})
	// где оказывается левый верхний угол после приведения
	want := map[int]image.Point{1: {0, 0}, 2: {2, 0}, 3: {2, 1}, 4: {0, 1}, 5: {0, 0}, 6: {1, 0}, 7: {1, 2}, 8: {0, 2}}
	for o, p := range want {
		m := orient(src, o)
		if !isRed(m.At(p.X, p.Y)) {
			t.Errorf("orientation %d: corner not at %v", o, p)
		}
	}
}

func TestParsePreview(t *testing.T) {
	bad := []url.Values{
		{},
		{"shape": {"star"}},
		{"shape": {"square"}, "size": {"100"}},
		{"shape": {"square"}, "x": {"1"}},
		{"shape": {"square"}, "x": {"0"}, "y": {"0"}, "w": {"5"}, "h": {"6"}},
		{"shape": {"square"}, "x": {"-1"}, "y": {"0"}, "w": {"5"}, "h": {"5"}},
	}
	for _, q := range bad {
		if _, err := ParsePreview(q); err == nil {
			t.Errorf("%v: expected error", q)
		}
	}
	c, err := ParsePreview(url.Values{"shape": {"circle"}})
	if err != nil || !c.Full || c.Size != DefaultSize {
		t.Fatalf("%+v %v", c, err)
	}
	if c, _ = c.Fit(30, 10); c.X != 10 || c.Y != 0 || c.W != 10 {
		t.Fatalf("center %+v", c)
	}
	c, _ = ParsePreview(url.Values{"shape": {"square"}, "x": {"20"}, "y": {"0"}, "w": {"20"}, "h": {"20"}})
	if _, err := c.Fit(30, 30); err == nil {
		t.Fatal("crop outside image accepted")
	}
	c, _ = ParsePreview(url.Values{"shape": {"square"}, "x": {"9223372036854775807"}, "y": {"0"}, "w": {"20"}, "h": {"20"}})
	if _, err := c.Fit(30, 30); err == nil {
		t.Fatal("overflowing crop accepted")
	}
}

// Прямоугольная обрезка (F-79): портрет дерева и карточки — превью в
// пропорциях обрезки, большая сторона — size.
func TestRectPreview(t *testing.T) {
	s := &Store{Root: t.TempDir(), MaxBytes: 1 << 20}
	info, err := s.Put(bytes.NewReader(withOrientation(t, halves(), 6))) // 16×32
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParsePreview(url.Values{"shape": {"rect"}, "x": {"0"}, "y": {"0"}, "w": {"12"}, "h": {"16"}, "size": {"64"}})
	if err != nil {
		t.Fatal(err)
	}
	if c, err = c.Fit(info.Width, info.Height); err != nil {
		t.Fatal(err)
	}
	p, err := s.Preview(info.SHA256, info.MIME, c)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p) //nolint:gosec // путь теста
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 48 || img.Bounds().Dy() != 64 {
		t.Fatalf("rect preview %v", img.Bounds())
	}
	// без обрезки — наибольший портрет 3:4 по центру
	full, _ := ParsePreview(url.Values{"shape": {"rect"}})
	if full, _ = full.Fit(16, 32); full.X != 0 || full.Y != 5 || full.W != 16 || full.H != 21 {
		t.Fatalf("full %+v", full)
	}
	if full, _ = full.Fit(40, 20); full.W != 15 || full.H != 20 || full.X != 12 {
		t.Fatalf("full wide %+v", full)
	}
	// квадрат и круг — по-прежнему только w = h
	if _, err := ParsePreview(url.Values{"shape": {"circle"}, "x": {"0"}, "y": {"0"}, "w": {"5"}, "h": {"6"}}); err == nil {
		t.Fatal("non-square circle accepted")
	}
}
