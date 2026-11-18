package visualizer

import (
	"image"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"
)

// SixelEncode uses a whole-image median-cut palette. Taking the first 256 colors
// loses wool/eyes/accessories when the gradient and antialiased sky fill the
// palette before the foreground is visited. Quantized indices are reused for
// all planes instead of scanning every pixel for every palette entry.
func SixelEncode(img *image.RGBA) string {
	if img.Bounds().Empty() {
		return ""
	}
	palette, _ := buildPalette(img)
	if len(palette) == 0 {
		return ""
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	indices := make([]uint8, w*h)
	cache := make(map[color.RGBA]uint8)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			clr := img.RGBAAt(x+img.Rect.Min.X, y+img.Rect.Min.Y)
			idx, ok := cache[clr]
			if !ok {
				best := int(^uint(0) >> 1)
				for i, p := range palette {
					r, g, b := int(clr.R)-int(p.R), int(clr.G)-int(p.G), int(clr.B)-int(p.B)
					d := r*r + g*g + b*b
					if d < best {
						best = d
						idx = uint8(i)
					}
					if d == 0 {
						break
					}
				}
				cache[clr] = idx
			}
			indices[y*w+x] = idx
		}
	}
	return encodeIndices(w, h, palette, indices)
}
func SixelEncodeSolid(w, h int, clr color.RGBA) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	return encodeIndices(w, h, []color.RGBA{clr}, make([]uint8, w*h))
}
func encodeIndices(w, h int, palette []color.RGBA, indices []uint8) string {
	var out strings.Builder
	out.WriteString("\x1bP0;0;0q\"1;1;")
	out.WriteString(strconv.Itoa(w))
	out.WriteByte(';')
	out.WriteString(strconv.Itoa(h))
	for i, clr := range palette {
		out.WriteByte('#')
		out.WriteString(strconv.Itoa(i))
		out.WriteString(";2;")
		for j, v := range []uint8{clr.R, clr.G, clr.B} {
			if j > 0 {
				out.WriteByte(';')
			}
			out.WriteString(strconv.Itoa(int(math.Round(float64(v) * 100 / 255))))
		}
	}
	// One strip's planes, filled in one pass over its pixels.
	planes := make([]byte, len(palette)*w)
	used := make([]bool, len(palette))
	for y := 0; y < h; y += 6 {
		clear(planes)
		clear(used)
		for dy := 0; dy < 6 && y+dy < h; dy++ {
			for x := 0; x < w; x++ {
				idx := int(indices[(y+dy)*w+x])
				planes[idx*w+x] |= 1 << uint(dy)
				used[idx] = true
			}
		}
		for i := range palette {
			if !used[i] {
				continue
			}
			out.WriteByte('#')
			out.WriteString(strconv.Itoa(i))
			plane := planes[i*w : (i+1)*w]
			end := w
			for end > 0 && plane[end-1] == 0 {
				end--
			}
			for x := 0; x < end; {
				v := plane[x]
				run := 1
				for x+run < end && plane[x+run] == v {
					run++
				}
				if run >= 4 {
					out.WriteByte('!')
					out.WriteString(strconv.Itoa(run))
					out.WriteByte('?' + v)
				} else {
					for j := 0; j < run; j++ {
						out.WriteByte('?' + v)
					}
				}
				x += run
			}
			out.WriteByte('$')
		}
		if y+6 < h {
			out.WriteByte('-')
		}
	}
	out.WriteString("\x1b\\")
	return out.String()
}

type paletteSample struct {
	clr   color.RGBA
	count int
}
type colorBox struct {
	samples      []paletteSample
	channel      int
	span, weight int
}

func makeColorBox(samples []paletteSample) colorBox {
	box := colorBox{samples: samples}
	lo, hi := [3]int{255, 255, 255}, [3]int{}
	for _, s := range samples {
		box.weight += s.count
		for i, v := range []uint8{s.clr.R, s.clr.G, s.clr.B} {
			lo[i] = min(lo[i], int(v))
			hi[i] = max(hi[i], int(v))
		}
	}
	for i := 0; i < 3; i++ {
		if hi[i]-lo[i] > box.span {
			box.span = hi[i] - lo[i]
			box.channel = i
		}
	}
	return box
}
func component(c color.RGBA, i int) uint8 {
	if i == 0 {
		return c.R
	}
	if i == 1 {
		return c.G
	}
	return c.B
}
func colorKey(c color.RGBA) uint32 { return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B) }
func buildPalette(img *image.RGBA) ([]color.RGBA, map[color.RGBA]int) {
	histogram := make(map[color.RGBA]int)
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.A > 0 {
				c.A = 255
				histogram[c]++
			}
		}
	}
	samples := make([]paletteSample, 0, len(histogram))
	for c, n := range histogram {
		samples = append(samples, paletteSample{c, n})
	}
	sort.Slice(samples, func(i, j int) bool { return colorKey(samples[i].clr) < colorKey(samples[j].clr) })
	if len(samples) == 0 {
		return nil, nil
	}
	boxes := []colorBox{makeColorBox(samples)}
	for len(boxes) < 256 {
		chosen, score := -1, -1.0
		for i, b := range boxes {
			if len(b.samples) < 2 {
				continue
			}
			v := float64(b.span) * math.Sqrt(float64(b.weight))
			if v > score {
				chosen = i
				score = v
			}
		}
		if chosen < 0 {
			break
		}
		box := boxes[chosen]
		sort.Slice(box.samples, func(i, j int) bool {
			a, b := box.samples[i].clr, box.samples[j].clr
			av, bv := component(a, box.channel), component(b, box.channel)
			if av == bv {
				return colorKey(a) < colorKey(b)
			}
			return av < bv
		})
		total, split := 0, 1
		for i, s := range box.samples[:len(box.samples)-1] {
			total += s.count
			split = i + 1
			if total >= box.weight/2 {
				break
			}
		}
		boxes[chosen] = makeColorBox(box.samples[:split])
		boxes = append(boxes, makeColorBox(box.samples[split:]))
	}
	palette := make([]color.RGBA, 0, len(boxes))
	index := make(map[color.RGBA]int)
	for _, box := range boxes {
		r, g, b := 0, 0, 0
		for _, s := range box.samples {
			r += int(s.clr.R) * s.count
			g += int(s.clr.G) * s.count
			b += int(s.clr.B) * s.count
		}
		c := color.RGBA{uint8(r / box.weight), uint8(g / box.weight), uint8(b / box.weight), 255}
		if _, ok := index[c]; !ok {
			index[c] = len(palette)
			palette = append(palette, c)
		}
	}
	return palette, index
}
