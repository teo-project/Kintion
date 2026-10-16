package files

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	xdraw "golang.org/x/image/draw"
)

// Формы превью (F-30, F-72, F-79): квадрат, круг (квадрат с прозрачными
// углами) и прямоугольник любых пропорций (портрет дерева, карточка).
const (
	ShapeSquare = "square"
	ShapeCircle = "circle"
	ShapeRect   = "rect"
)

// Sizes — белый список сторон превью в пикселях; DefaultSize — если size не задан.
var Sizes = []int{64, 128, 256, 512}

// DefaultSize — сторона превью по умолчанию.
const DefaultSize = 256

// ParamError — неверный параметр превью (400).
type ParamError struct {
	Param   string
	Message string
}

func (e *ParamError) Error() string { return e.Param + ": " + e.Message }

// Crop — параметры превью. Обрезка — X, Y, W×H в пикселях изображения с
// учётом ориентации EXIF: у квадрата и круга W = H, у прямоугольника — любые;
// параметры всегда приходят в запросе (UI берёт их из meta.crop значения для
// своего места показа, F-79), без них — наибольший квадрат по центру
// (прямоугольник — наибольший портрет 3:4 по центру). Size — большая
// сторона превью.
type Crop struct {
	Shape      string
	X, Y, W, H int
	Size       int
	Full       bool // обрезка не задана
}

// ParsePreview разбирает ?shape=&x=&y=&w=&h=&size=.
func ParsePreview(q url.Values) (Crop, error) {
	c := Crop{Shape: q.Get("shape"), Size: DefaultSize}
	if c.Shape != ShapeSquare && c.Shape != ShapeCircle && c.Shape != ShapeRect {
		return c, &ParamError{"shape", "ожидается square, circle или rect"}
	}
	if s := q.Get("size"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || !slices.Contains(Sizes, n) {
			return c, &ParamError{"size", fmt.Sprintf("допустимые размеры: %v", Sizes)}
		}
		c.Size = n
	}
	names := []string{"x", "y", "w", "h"}
	ptrs := []*int{&c.X, &c.Y, &c.W, &c.H}
	given := 0
	for i, name := range names {
		s := q.Get(name)
		if s == "" {
			continue
		}
		given++
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return c, &ParamError{name, "ожидается целое число ≥ 0"}
		}
		*ptrs[i] = n
	}
	switch given {
	case 0:
		c.Full = true
	case len(names):
		if c.W == 0 || c.H == 0 {
			return c, &ParamError{"w", "обрезка: w, h > 0"}
		}
		if c.Shape != ShapeRect && c.W != c.H {
			return c, &ParamError{"w", "обрезка квадрата и круга: w = h"}
		}
	default:
		return c, &ParamError{"x", "обрезка задаётся всеми x, y, w, h или не задаётся"}
	}
	return c, nil
}

// Fit проверяет обрезку по размеру изображения; без обрезки — центральный квадрат.
func (c Crop) Fit(width, height int) (Crop, error) {
	if c.Full {
		if c.Shape == ShapeRect { // начальная обрезка — портретная (F-79)
			c.W, c.H = width, width*4/3
			if c.H > height {
				c.W, c.H = height*3/4, height
			}
			c.W, c.H = max(1, c.W), max(1, c.H)
			c.X, c.Y = (width-c.W)/2, (height-c.H)/2
			return c, nil
		}
		side := min(width, height)
		c.X, c.Y, c.W, c.H = (width-side)/2, (height-side)/2, side, side
		return c, nil
	}
	if !c.inside(width, height) {
		return c, &ParamError{"w", fmt.Sprintf("обрезка выходит за изображение %d×%d", width, height)}
	}
	return c, nil
}

// inside — обрезка внутри изображения (без переполнения при больших x, y).
func (c Crop) inside(width, height int) bool {
	return c.X <= width && c.Y <= height && c.W <= width-c.X && c.H <= height-c.Y
}

// key — имя превью в кеше: хеш оригинала и все параметры.
func (c Crop) key(sha string) string {
	ext := "jpg"
	if c.Shape == ShapeCircle {
		ext = "png" // круг — с прозрачными углами
	}
	return fmt.Sprintf("%s_%s_%d_%d_%d_%d_%d.%s", sha, c.Shape, c.X, c.Y, c.W, c.H, c.Size, ext)
}

// outSize — стороны превью: большая — Size, пропорции — как у обрезки.
func (c Crop) outSize() (int, int) {
	if c.Shape != ShapeRect || c.W == c.H || c.W == 0 || c.H == 0 {
		return c.Size, c.Size
	}
	if c.W > c.H {
		return c.Size, max(1, int(math.Round(float64(c.Size)*float64(c.H)/float64(c.W))))
	}
	return max(1, int(math.Round(float64(c.Size)*float64(c.W)/float64(c.H)))), c.Size
}

// MIME — тип превью.
func (c Crop) MIME() string {
	if c.Shape == ShapeCircle {
		return "image/png"
	}
	return "image/jpeg"
}

// ETag — метка превью: те же параметры — то же содержимое.
func (c Crop) ETag(sha string) string { return `"` + c.key(sha) + `"` }

// Preview возвращает путь превью в кеше на томе, создавая его при первом
// запросе. c уже проверен Fit.
func (s *Store) Preview(sha, mime string, c Crop) (string, error) {
	if !ValidSHA(sha) {
		return "", os.ErrNotExist
	}
	dst := filepath.Join(shard(filepath.Join(s.Root, "previews"), sha), c.key(sha))
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	img, err := s.decode(sha, mime)
	if err != nil {
		return "", err
	}
	b := img.Bounds()
	if !c.inside(b.Dx(), b.Dy()) {
		return "", &ParamError{"w", "обрезка выходит за изображение"}
	}
	ow, oh := c.outSize()
	out := image.NewRGBA(image.Rect(0, 0, ow, oh))
	if c.Shape != ShapeCircle {
		// JPEG без прозрачности: прозрачное — на белом
		xdraw.Draw(out, out.Bounds(), image.NewUniform(color.White), image.Point{}, xdraw.Src)
	}
	src := image.Rect(b.Min.X+c.X, b.Min.Y+c.Y, b.Min.X+c.X+c.W, b.Min.Y+c.Y+c.H)
	xdraw.CatmullRom.Scale(out, out.Bounds(), img, src, xdraw.Over, nil)

	var buf bytes.Buffer
	if c.Shape == ShapeCircle {
		circleMask(out)
		err = png.Encode(&buf, out)
	} else {
		err = jpeg.Encode(&buf, out, &jpeg.Options{Quality: 85})
	}
	if err != nil {
		return "", err
	}
	return dst, writeAtomic(s, dst, buf.Bytes())
}

func (s *Store) decode(sha, mime string) (image.Image, error) {
	f, err := s.Open(sha)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, ErrUnsupported
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, ErrTooLarge
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, ErrUnsupported
	}
	if mime == "image/jpeg" {
		if _, err := f.Seek(0, 0); err != nil {
			return nil, err
		}
		img = orient(img, jpegOrientation(f))
	}
	return img, nil
}

// circleMask делает прозрачным всё вне вписанного круга (со сглаживанием края).
func circleMask(m *image.RGBA) {
	n := m.Bounds().Dx()
	r := float64(n) / 2
	for y := range n {
		for x := range n {
			dx, dy := float64(x)+0.5-r, float64(y)+0.5-r
			d := r - math.Hypot(dx, dy) // > 0 — внутри
			var a float64
			switch {
			case d >= 1:
				continue
			case d > 0:
				a = d
			}
			i := m.PixOffset(x, y)
			for k := range 4 { // RGBA premultiplied: масштабируются все каналы
				m.Pix[i+k] = uint8(float64(m.Pix[i+k]) * a)
			}
		}
	}
}

func writeAtomic(s *Store, dst string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	dir, err := s.tmpDir()
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "preview-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
