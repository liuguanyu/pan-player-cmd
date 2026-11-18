package visualizer

import (
	"image/color"
	"math"
)

// scenePaint supplies Android-like affine transforms in 360-unit scene space.
// Rendering at 2x and downsampling keeps eyes, wool outlines and hats legible in
// the terminal without increasing the Sixel payload dimensions.
type scenePaint struct {
	c                  *Canvas
	a, b, d, e, tx, ty float64
}

func newScenePaint(c *Canvas) scenePaint {
	scale := math.Min(float64(c.H)/360, float64(c.W)/360)
	return scenePaint{c: c, a: scale, e: scale}
}
func (p scenePaint) point(x, y float64) (int, int) {
	return int(math.Round(p.a*x + p.d*y + p.tx)), int(math.Round(p.b*x + p.e*y + p.ty))
}
func (p scenePaint) rotate(deg, x, y float64) scenePaint {
	r := deg2rad(deg)
	co, si := math.Cos(r), math.Sin(r)
	tx, ty := x-co*x+si*y, y-si*x-co*y
	p.tx += p.a*tx + p.d*ty
	p.ty += p.b*tx + p.e*ty
	a, b, d, e := p.a, p.b, p.d, p.e
	p.a = a*co + d*si
	p.b = b*co + e*si
	p.d = -a*si + d*co
	p.e = -b*si + e*co
	return p
}
func (p scenePaint) line(x, y, x1, y1, width float64, clr color.RGBA) {
	a, b := p.point(x, y)
	d, e := p.point(x1, y1)
	p.c.StrokeLine(a, b, d, e, max(1, int(math.Round(width*math.Hypot(p.a, p.b)))), clr)
}
func (p scenePaint) polygon(clr color.RGBA, pts ...float64) {
	if len(pts) < 6 {
		return
	}
	x0, y0 := p.point(pts[0], pts[1])
	for i := 2; i+3 < len(pts); i += 2 {
		x, y := p.point(pts[i], pts[i+1])
		x1, y1 := p.point(pts[i+2], pts[i+3])
		p.c.FillTriangle(x0, y0, x, y, x1, y1, clr)
	}
}
func (p scenePaint) rect(x, y, w, h float64, clr color.RGBA) {
	p.polygon(clr, x, y, x+w, y, x+w, y+h, x, y+h)
}
func (p scenePaint) ellipse(x, y, rx, ry float64, clr color.RGBA) {
	if rx <= 0 || ry <= 0 {
		return
	}
	// Inverse-transform only the bounding box, avoiding polygon seams and keeping
	// ellipses smooth under arbitrary head/body rotation.
	cx, cy := p.a*x+p.d*y+p.tx, p.b*x+p.e*y+p.ty
	bx := math.Hypot(p.a*rx, p.d*ry)
	by := math.Hypot(p.b*rx, p.e*ry)
	det := p.a*p.e - p.b*p.d
	if det == 0 {
		return
	}
	for py := max(0, int(math.Floor(cy-by))); py <= min(p.c.H-1, int(math.Ceil(cy+by))); py++ {
		for px := max(0, int(math.Floor(cx-bx))); px <= min(p.c.W-1, int(math.Ceil(cx+bx))); px++ {
			dx, dy := float64(px)-cx, float64(py)-cy
			u, v := (p.e*dx-p.d*dy)/det/rx, (-p.b*dx+p.a*dy)/det/ry
			if u*u+v*v <= 1 {
				p.c.setPixelSafe(px, py, clr)
			}
		}
	}
}
func (p scenePaint) ring(x, y, r, width float64, clr color.RGBA) {
	for i := 0; i < 48; i++ {
		a, b := float64(i)*math.Pi/24, float64(i+1)*math.Pi/24
		p.line(x+math.Cos(a)*r, y+math.Sin(a)*r, x+math.Cos(b)*r, y+math.Sin(b)*r, width, clr)
	}
}
func rgb(v uint32) color.RGBA { return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} }

var black, white = rgb(0), rgb(0xffffff)

func downsample2(src *Canvas, dst *Canvas) {
	for y := 0; y < dst.H; y++ {
		for x := 0; x < dst.W; x++ {
			var r, g, b int
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					v := src.img.RGBAAt(x*2+dx, y*2+dy)
					r += int(v.R)
					g += int(v.G)
					b += int(v.B)
				}
			}
			dst.img.SetRGBA(x, y, color.RGBA{uint8(r / 4), uint8(g / 4), uint8(b / 4), 255})
		}
	}
}
