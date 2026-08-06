package visualizer

import (
	"image"
	"image/color"
	"math"
	"strings"
)

// SixelEncode converts an RGBA image to a Sixel escape sequence string.
// The resulting string can be written directly to a Sixel-capable terminal.
//
// Sixel format reference:
//
//	ESC P q             — start
//	#0;2;0;0;0          — define palette entry 0 as RGB (0,0,0)
//	"1;1;W;H            — raster attributes (optional)
//	<sixels per strip>
//	ESC \               — end
func SixelEncode(img *image.RGBA) string {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w == 0 || h == 0 {
		return ""
	}

	// Build palette from the image (up to 256 colors)
	palette, _ := buildPalette(img)

	// Floyd-Steinberg dither: smooth the sky gradient by diffusing
	// quantization error to neighboring pixels.  Creates a new image
	// where every pixel is one of the palette colors.
	img = ditherFloydSteinberg(img, palette)

	return sixelEncodeWithPalette(img, palette)
}

// SixelEncodeSolid returns a solid-color Sixel image used to erase previous
// Sixel graphics. It intentionally avoids dithering and palette extraction.
func SixelEncodeSolid(w, h int, clr color.RGBA) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, clr)
		}
	}
	return sixelEncodeWithPalette(img, []color.RGBA{clr})
}

func sixelEncodeWithPalette(img *image.RGBA, palette []color.RGBA) string {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	var sb strings.Builder

	// Sixel start (explicit parameters for max compatibility)
	sb.WriteString("\x1bP0;0;0q")
	// Raster attributes: pixel aspect 1:1 and explicit image bounds.
	// Without this, some terminals may use a non-square default aspect ratio.
	sb.WriteByte('"')
	sb.WriteString("1;1;")
	sb.WriteString(itoa(w))
	sb.WriteByte(';')
	sb.WriteString(itoa(h))

	// Define palette
	for i, clr := range palette {
		sb.WriteString(colorDefine(i, clr))
	}

	// Render in 6-pixel-high strips
	strips := (h + 5) / 6
	for strip := 0; strip < strips; strip++ {
		y0 := strip * 6
		y1 := y0 + 6
		if y1 > h {
			y1 = h
		}
		stripH := y1 - y0

		// For each color in the palette, output a color plane for this strip.
		// Important: after each color plane, emit '$' to carriage-return to the
		// start of the same strip. Without '$', color planes are appended
		// horizontally, stretching the image into a long band.
		wrotePlane := false
		for colorIdx, clr := range palette {
			// Skip if this color doesn't appear in this strip
			if !colorInStrip(img, y0, y1, clr) {
				continue
			}

			sb.WriteString(colorSelect(colorIdx))

			// Encode sixels for this color in this strip.
			// Each sixel character represents one column; we must emit
			// enough characters to cover the full image width so the
			// graphics cursor stays aligned.  Use standard Sixel RLE
			// (!count char) to compress long runs.
			x := 0
			for x < w {
				// Build the 6-bit sixel value for this column.
				byteVal := byte(0)
				for dy := 0; dy < stripH; dy++ {
					px := img.RGBAAt(x, y0+dy)
					if px.R == clr.R && px.G == clr.G && px.B == clr.B && px.A > 0 {
						byteVal |= (1 << uint(dy))
					}
				}
				char := byteToSixel(byteVal)

				// Count how many times this exact value repeats.
				run := 1
				for x+run < w {
					nextVal := byte(0)
					for dy := 0; dy < stripH; dy++ {
						px := img.RGBAAt(x+run, y0+dy)
						if px.R == clr.R && px.G == clr.G && px.B == clr.B && px.A > 0 {
							nextVal |= (1 << uint(dy))
						}
					}
					if byteToSixel(nextVal) != char {
						break
					}
					run++
				}

				if run > 1 {
					sb.WriteString(fmtSixel("!%d", run))
				}
				sb.WriteByte(char)
				x += run
			}
			sb.WriteByte('$')
			wrotePlane = true
		}

		// Move to next strip
		if wrotePlane {
			sb.WriteByte('-')
		}
	}

	// Sixel end
	sb.WriteString("\x1b\\")
	return sb.String()
}

// Color quantization: extract a palette from the image.
// We use a simple hash-based approach that works for pixel art with few colors.
func buildPalette(img *image.RGBA) ([]color.RGBA, map[color.RGBA]int) {
	bounds := img.Bounds()
	colorIndex := make(map[color.RGBA]int)
	var palette []color.RGBA

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			clr := img.RGBAAt(x, y)
			if clr.A == 0 {
				continue
			}
			if _, ok := colorIndex[clr]; !ok {
				if len(palette) >= 256 {
					continue
				}
				colorIndex[clr] = len(palette)
				palette = append(palette, clr)
			}
		}
	}

	return palette, colorIndex
}

func colorDefine(index int, clr color.RGBA) string {
	// Multiply by 100/255 ≈ 0.392 for the Sixel 0-100 RGB scale
	r := int(math.Round(float64(clr.R) * 100.0 / 255.0))
	g := int(math.Round(float64(clr.G) * 100.0 / 255.0))
	b := int(math.Round(float64(clr.B) * 100.0 / 255.0))
	if r > 100 {
		r = 100
	}
	if g > 100 {
		g = 100
	}
	if b > 100 {
		b = 100
	}
	return fmtSixel("#%d;2;%d;%d;%d", index, r, g, b)
}

func colorSelect(index int) string {
	return fmtSixel("#%d", index)
}

func fmtSixel(format string, args ...interface{}) string {
	var sb strings.Builder
	switch format {
	case "#%d;2;%d;%d;%d":
		i := args[0].(int)
		r := args[1].(int)
		g := args[2].(int)
		b := args[3].(int)
		sb.WriteByte('#')
		sb.WriteString(itoa(i))
		sb.WriteString(";2;")
		sb.WriteString(itoa(r))
		sb.WriteByte(';')
		sb.WriteString(itoa(g))
		sb.WriteByte(';')
		sb.WriteString(itoa(b))
	case "#%d":
		sb.WriteByte('#')
		sb.WriteString(itoa(args[0].(int)))
	case "!%d":
		sb.WriteByte('!')
		sb.WriteString(itoa(args[0].(int)))
	}
	return sb.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [10]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}

// byteToSixel converts a 6-bit value (0-63) to its sixel character.
// Sixel characters: ? = 0, @ = 1, A = 2, ... ~ = 62, DEL = 63
func byteToSixel(b byte) byte {
	if b < 63 {
		return '?' + b
	}
	return '~' // value 63 (all bits set)
}

func colorInStrip(img *image.RGBA, y0, y1 int, clr color.RGBA) bool {
	bounds := img.Bounds()
	for y := y0; y < y1 && y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			px := img.RGBAAt(x, y)
			if px.R == clr.R && px.G == clr.G && px.B == clr.B && px.A > 0 {
				return true
			}
		}
	}
	return false
}

// ditherFloydSteinberg creates a new RGBA image where every pixel is one of
// the palette colors.  Quantization error is diffused to the right and down
// using the classic Floyd-Steinberg kernel, smoothing gradients like the sky.
//
// Kernel:       X   7/16
//
//	3/16  5/16  1/16
func ditherFloydSteinberg(img *image.RGBA, palette []color.RGBA) *image.RGBA {
	if len(palette) == 0 {
		return img
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	dithered := image.NewRGBA(bounds)

	// Copy pixel data (float64 for error accumulation)
	type fpx struct{ r, g, b float64 }
	pixels := make([][]fpx, h)
	for y := 0; y < h; y++ {
		pixels[y] = make([]fpx, w)
		for x := 0; x < w; x++ {
			c := img.RGBAAt(x, y)
			pixels[y][x] = fpx{float64(c.R), float64(c.G), float64(c.B)}
		}
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			old := pixels[y][x]
			// Find closest palette color
			best := palette[0]
			bestDist := float64(1 << 50)
			for _, p := range palette {
				dr := old.r - float64(p.R)
				dg := old.g - float64(p.G)
				db := old.b - float64(p.B)
				d := dr*dr + dg*dg + db*db
				if d < bestDist {
					bestDist = d
					best = p
				}
				if d == 0 {
					break
				}
			}

			// Set the dithered pixel
			dithered.SetRGBA(x, y, best)

			// Compute and distribute error
			errR := old.r - float64(best.R)
			errG := old.g - float64(best.G)
			errB := old.b - float64(best.B)

			if x+1 < w {
				pixels[y][x+1].r += errR * 7 / 16
				pixels[y][x+1].g += errG * 7 / 16
				pixels[y][x+1].b += errB * 7 / 16
			}
			if y+1 < h {
				if x > 0 {
					pixels[y+1][x-1].r += errR * 3 / 16
					pixels[y+1][x-1].g += errG * 3 / 16
					pixels[y+1][x-1].b += errB * 3 / 16
				}
				pixels[y+1][x].r += errR * 5 / 16
				pixels[y+1][x].g += errG * 5 / 16
				pixels[y+1][x].b += errB * 5 / 16
				if x+1 < w {
					pixels[y+1][x+1].r += errR * 1 / 16
					pixels[y+1][x+1].g += errG * 1 / 16
					pixels[y+1][x+1].b += errB * 1 / 16
				}
			}
		}
	}

	return dithered
}
