// Command genicon draws the app emblem (an original design: a gold ring around a four-point star on
// dark slate) and writes the icon files Tauri's bundler reads. Run from overlay/:
//
//	go run ./tools/genicon src-tauri/icons
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// draw renders the emblem at n×n with 4× supersampling.
func draw(n int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	const ss = 4
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					u := (float64(x)+(float64(sx)+.5)/ss)/float64(n)*2 - 1
					v := (float64(y)+(float64(sy)+.5)/ss)/float64(n)*2 - 1
					c := shade(u, v)
					r += float64(c.R) * float64(c.A)
					g += float64(c.G) * float64(c.A)
					b += float64(c.B) * float64(c.A)
					a += float64(c.A)
				}
			}
			if a > 0 {
				img.SetNRGBA(x, y, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(a / (ss * ss))})
			}
		}
	}
	return img
}

// shade is the emblem's colour at (u, v) in [-1, 1].
func shade(u, v float64) color.NRGBA {
	d := math.Hypot(u, v)
	gold := color.NRGBA{0xd9, 0xb0, 0x5a, 0xff}
	switch {
	case d > 0.94:
		return color.NRGBA{}
	case d > 0.84:
		return gold
	case d > 0.80:
		return color.NRGBA{0x3a, 0x2c, 0x14, 0xff}
	}
	// A four-point star: |u|^p + |v|^p small, with a pinched waist.
	s := math.Pow(math.Abs(u), 0.55) + math.Pow(math.Abs(v), 0.55)
	if s < 0.95 {
		return gold
	}
	t := d / 0.8
	return color.NRGBA{uint8(0x22 - 8*t), uint8(0x26 - 8*t), uint8(0x30 - 8*t), 0xff}
}

func main() {
	dir := "src-tauri/icons"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	must(os.MkdirAll(dir, 0o755))
	enc := func(n int) []byte {
		var b bytes.Buffer
		must(png.Encode(&b, draw(n)))
		return b.Bytes()
	}
	for name, n := range map[string]int{"icon.png": 512, "32x32.png": 32, "128x128.png": 128, "128x128@2x.png": 256} {
		must(os.WriteFile(filepath.Join(dir, name), enc(n), 0o644))
	}
	must(os.WriteFile(filepath.Join(dir, "icon.ico"), ico(enc(16), enc(32), enc(48), enc(64), enc(128), enc(256)), 0o644))
}

// ico packs PNG images (16, 32, 48, 64, 128, 256 px, in that order) into one .ico file.
func ico(pngs ...[]byte) []byte {
	sizes := []int{16, 32, 48, 64, 128, 256}
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, [3]uint16{0, 1, uint16(len(pngs))})
	off := 6 + 16*len(pngs)
	for i, p := range pngs {
		w := byte(sizes[i])
		if sizes[i] == 256 {
			w = 0
		}
		b.Write([]byte{w, w, 0, 0})
		binary.Write(&b, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&b, binary.LittleEndian, [2]uint32{uint32(len(p)), uint32(off)})
		off += len(p)
	}
	for _, p := range pngs {
		b.Write(p)
	}
	return b.Bytes()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
