package files

import (
	"bufio"
	"encoding/binary"
	"image"
	"image/draw"
	"io"
)

// jpegOrientation — тег Orientation (0x0112) из EXIF JPEG; 1, если его нет
// или он не читается. Читаются только маркеры до начала данных кадра.
func jpegOrientation(r io.Reader) int {
	br := bufio.NewReader(io.LimitReader(r, 1<<20))
	var soi [2]byte
	if _, err := io.ReadFull(br, soi[:]); err != nil || soi != [2]byte{0xFF, 0xD8} {
		return 1
	}
	for {
		var m [4]byte
		if _, err := io.ReadFull(br, m[:]); err != nil || m[0] != 0xFF {
			return 1
		}
		marker := m[1]
		n := int(binary.BigEndian.Uint16(m[2:])) - 2
		if marker == 0xDA || marker == 0xD9 || n < 0 {
			return 1 // начались данные кадра — EXIF не было
		}
		seg := make([]byte, n)
		if _, err := io.ReadFull(br, seg); err != nil {
			return 1
		}
		if marker == 0xE1 && len(seg) > 6 && string(seg[:6]) == "Exif\x00\x00" {
			return exifOrientation(seg[6:])
		}
	}
}

// exifOrientation читает Orientation из TIFF-заголовка и IFD0.
func exifOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:8]))
	if off < 8 || off+2 > len(t) {
		return 1
	}
	count := int(bo.Uint16(t[off:]))
	for i := range count {
		e := off + 2 + i*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 && bo.Uint16(t[e+2:]) == 3 { // SHORT
			if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// swapsAxes — ориентация поворачивает изображение на 90°.
func swapsAxes(o int) bool { return o >= 5 && o <= 8 }

// orient приводит изображение к виду, в котором его показывают (EXIF 1…8).
func orient(src image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if swapsAxes(o) {
		dw, dh = h, w
	}
	s := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(s, s.Bounds(), src, b.Min, draw.Src)
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range dh {
		for x := range dw {
			var sx, sy int
			switch o {
			case 2: // отражение по горизонтали
				sx, sy = w-1-x, y
			case 3: // поворот на 180°
				sx, sy = w-1-x, h-1-y
			case 4: // отражение по вертикали
				sx, sy = x, h-1-y
			case 5: // транспонирование
				sx, sy = y, x
			case 6: // поворот на 90° по часовой
				sx, sy = y, h-1-x
			case 7: // поперечное транспонирование
				sx, sy = w-1-y, h-1-x
			case 8: // поворот на 90° против часовой
				sx, sy = w-1-y, x
			}
			i, j := s.PixOffset(sx, sy), dst.PixOffset(x, y)
			copy(dst.Pix[j:j+4], s.Pix[i:i+4])
		}
	}
	return dst
}
