package visualizer

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"testing"
)

// Decode the emitted subset so tests verify image data, not just DCS markers.
func decodeSixel(t *testing.T, s string, w, h int) [][]int {
	t.Helper()
	pixels := make([][]int, h)
	for y := range pixels {
		pixels[y] = make([]int, w)
		for x := range pixels[y] {
			pixels[y][x] = -1
		}
	}
	if !strings.HasPrefix(s, "\x1bP0;0;0q\"") || !strings.HasSuffix(s, "\x1b\\") {
		t.Fatal("invalid DCS framing")
	}
	data := strings.TrimSuffix(strings.TrimPrefix(s, "\x1bP0;0;0q\""), "\x1b\\")
	i := 0
	number := func() int {
		start := i
		for i < len(data) && data[i] >= '0' && data[i] <= '9' {
			i++
		}
		v, _ := strconv.Atoi(data[start:i])
		return v
	}
	for i < len(data) && (data[i] >= '0' && data[i] <= '9' || data[i] == ';') {
		i++
	}
	x, y, clr := 0, 0, 0
	for i < len(data) {
		ch := data[i]
		i++
		switch ch {
		case '#':
			clr = number()
			if i < len(data) && data[i] == ';' {
				for i < len(data) && (data[i] == ';' || data[i] >= '0' && data[i] <= '9') {
					i++
				}
			}
		case '$':
			x = 0
		case '-':
			y += 6
			x = 0
		default:
			run := 1
			if ch == '!' {
				run = number()
				if i >= len(data) {
					t.Fatal("truncated RLE")
				}
				ch = data[i]
				i++
			}
			if ch < '?' || ch > '~' {
				t.Fatalf("invalid sixel byte %q", ch)
			}
			for n := 0; n < run; n++ {
				for bit := 0; bit < 6; bit++ {
					if (ch-'?')&(1<<uint(bit)) != 0 && y+bit < h && x < w {
						pixels[y+bit][x] = clr
					}
				}
				x++
			}
		}
	}
	return pixels
}
func TestSixelPlanesAndPartialStrip(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 13, 7))
	for y := 0; y < 7; y++ {
		for x := 0; x < 13; x++ {
			c := black
			if (x+y)%3 == 0 {
				c = white
			}
			img.SetRGBA(x, y, c)
		}
	}
	palette, index := buildPalette(img)
	_ = palette
	encoded := SixelEncode(img)
	if encoded != SixelEncode(img) {
		t.Fatal("palette is nondeterministic")
	}
	decoded := decodeSixel(t, encoded, 13, 7)
	for y := 0; y < 7; y++ {
		for x := 0; x < 13; x++ {
			if decoded[y][x] != index[img.RGBAAt(x, y)] {
				t.Fatalf("pixel %d,%d was lost or misplaced", x, y)
			}
		}
	}
	solid := decodeSixel(t, SixelEncodeSolid(13, 7, black), 13, 7)
	for _, row := range solid {
		for _, v := range row {
			if v != 0 {
				t.Fatal("clear frame incomplete")
			}
		}
	}
}
func TestPaletteRetainsForegroundAfterGradient(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 560, 252))
	for y := 0; y < 252; y++ {
		for x := 0; x < 560; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x % 256), uint8(y), 200, 255})
		}
	}
	// High-contrast eyes/wool are deliberately last in scan order.
	for y := 200; y < 252; y++ {
		for x := 200; x < 350; x++ {
			c := white
			if x < 230 {
				c = black
			}
			img.SetRGBA(x, y, c)
		}
	}
	palette, _ := buildPalette(img)
	for _, want := range []color.RGBA{black, white} {
		distance := math.Inf(1)
		for _, c := range palette {
			d := math.Abs(float64(c.R)-float64(want.R)) + math.Abs(float64(c.G)-float64(want.G)) + math.Abs(float64(c.B)-float64(want.B))
			distance = math.Min(distance, d)
		}
		if distance > 10 {
			t.Fatalf("foreground color lost: %v distance %v", want, distance)
		}
	}
	if len(palette) > 256 {
		t.Fatal("palette exceeds Sixel limit")
	}
}
