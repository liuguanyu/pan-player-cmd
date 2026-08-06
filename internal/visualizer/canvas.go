package visualizer

import (
	"image"
	"image/color"
	"math"
)

// Canvas wraps an *image.RGBA and provides pixel-art-style drawing primitives.
// All coordinates are in logical pixels (0,0 is top-left).
type Canvas struct {
	img *image.RGBA
	W   int
	H   int
}

// NewCanvas creates a canvas backed by an RGBA image of the given size.
func NewCanvas(w, h int) *Canvas {
	return &Canvas{
		img: image.NewRGBA(image.Rect(0, 0, w, h)),
		W:   w,
		H:   h,
	}
}

// Image returns the underlying RGBA image (for encoding to Sixel, etc.).
func (c *Canvas) Image() *image.RGBA {
	return c.img
}

// Clear fills the entire canvas with a single color.
func (c *Canvas) Clear(clr color.RGBA) {
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			c.img.SetRGBA(x, y, clr)
		}
	}
}

// DrawGradientVertical fills the canvas with a vertical gradient from top to bottom.
func (c *Canvas) DrawGradientVertical(top, bottom color.RGBA) {
	for y := 0; y < c.H; y++ {
		t := float64(y) / float64(c.H-1)
		clr := lerpRGBA(top, bottom, t)
		for x := 0; x < c.W; x++ {
			c.img.SetRGBA(x, y, clr)
		}
	}
}

// FillRect draws a filled rectangle.
func (c *Canvas) FillRect(x, y, w, h int, clr color.RGBA) {
	x0 := clampInt(x, 0, c.W)
	y0 := clampInt(y, 0, c.H)
	x1 := clampInt(x+w, 0, c.W)
	y1 := clampInt(y+h, 0, c.H)
	for py := y0; py < y1; py++ {
		for px := x0; px < x1; px++ {
			c.img.SetRGBA(px, py, clr)
		}
	}
}

// FillEllipse draws a filled ellipse centered at (cx, cy) with radii rx, ry.
func (c *Canvas) FillEllipse(cx, cy, rx, ry int, clr color.RGBA) {
	for dy := -ry; dy <= ry; dy++ {
		for dx := -rx; dx <= rx; dx++ {
			if float64(dx*dx)/float64(rx*rx)+float64(dy*dy)/float64(ry*ry) <= 1.0 {
				c.setPixelSafe(cx+dx, cy+dy, clr)
			}
		}
	}
}

// StrokeEllipse draws an outlined ellipse (1px border).
func (c *Canvas) StrokeEllipse(cx, cy, rx, ry int, clr color.RGBA) {
	for dy := -ry; dy <= ry; dy++ {
		for dx := -rx; dx <= rx; dx++ {
			d := float64(dx*dx)/float64(rx*rx) + float64(dy*dy)/float64(ry*ry)
			if d >= 0.85 && d <= 1.15 {
				c.setPixelSafe(cx+dx, cy+dy, clr)
			}
		}
	}
}

// FillCircle draws a filled circle.
func (c *Canvas) FillCircle(cx, cy, r int, clr color.RGBA) {
	c.FillEllipse(cx, cy, r, r, clr)
}

// StrokeLine draws a line of given thickness using Bresenham's algorithm.
func (c *Canvas) StrokeLine(x0, y0, x1, y1, thickness int, clr color.RGBA) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy

	for {
		// Draw a thick pixel
		for ty := -thickness / 2; ty <= thickness/2; ty++ {
			for tx := -thickness / 2; tx <= thickness/2; tx++ {
				c.setPixelSafe(x0+tx, y0+ty, clr)
			}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// FillTriangle draws a filled triangle.
func (c *Canvas) FillTriangle(x0, y0, x1, y1, x2, y2 int, clr color.RGBA) {
	// Bounding box
	minX := min3(x0, x1, x2)
	minY := min3(y0, y1, y2)
	maxX := max3(x0, x1, x2)
	maxY := max3(y0, y1, y2)

	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if pointInTriangle(x, y, x0, y0, x1, y1, x2, y2) {
				c.setPixelSafe(x, y, clr)
			}
		}
	}
}

// setPixelSafe sets a pixel if within bounds.
func (c *Canvas) setPixelSafe(x, y int, clr color.RGBA) {
	if x >= 0 && x < c.W && y >= 0 && y < c.H {
		c.img.SetRGBA(x, y, clr)
	}
}

// -- color helpers --

func lerpRGBA(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: 255,
	}
}

// Named colors (matching Electron version)
var (
	ColorSkyTopDay          = color.RGBA{0x9a, 0xa4, 0xff, 0xff}
	ColorSkyBotDay          = color.RGBA{0x00, 0x1d, 0xff, 0xff}
	ColorSkyTopNight        = color.RGBA{0x1b, 0x21, 0x70, 0xff}
	ColorSkyBotNight        = color.RGBA{0x02, 0x07, 0x2f, 0xff}
	ColorGrass              = color.RGBA{0x20, 0xf0, 0x20, 0xff}
	ColorFence              = color.RGBA{0xff, 0xff, 0xff, 0xff}
	ColorSheepWool          = color.RGBA{0xf3, 0xf3, 0xf3, 0xff}
	ColorSheepPink          = color.RGBA{0xf2, 0x7c, 0xd9, 0xff}
	ColorSheepHead          = color.RGBA{0x00, 0x00, 0x00, 0xff}
	ColorSheepEye           = color.RGBA{0xff, 0xff, 0xff, 0xff}
	ColorSheepPupil         = color.RGBA{0x00, 0x00, 0x00, 0xff}
	ColorTongue             = color.RGBA{0xff, 0x8a, 0xa8, 0xff}
	ColorCloud              = color.RGBA{0xff, 0xff, 0xff, 0xff}
	ColorMoon               = color.RGBA{0xff, 0xf3, 0x6b, 0xff}
	ColorStar               = color.RGBA{0xdf, 0xe7, 0xff, 0xff}
	ColorDiscoBall          = color.RGBA{0xf2, 0xf2, 0xf2, 0xff}
	ColorDiscoWire          = color.RGBA{0x20, 0x20, 0x20, 0xff}
	ColorMicHead            = color.RGBA{0x8c, 0x8c, 0x8c, 0xff}
	ColorMicBody            = color.RGBA{0x1f, 0x1f, 0x1f, 0xff}
	ColorSpectShell         = color.RGBA{0xd8, 0xd8, 0xd8, 0xff}
	ColorShadow             = color.RGBA{0xf0, 0xff, 0xd2, 0xb8} // semi-transparent approximation
	ColorTerminalBackground = color.RGBA{0x10, 0x16, 0x1a, 0xff} // iTerm dark background approximation
)

// -- math helpers --

func pointInTriangle(px, py, x0, y0, x1, y1, x2, y2 int) bool {
	d1 := sign(px, py, x0, y0, x1, y1)
	d2 := sign(px, py, x1, y1, x2, y2)
	d3 := sign(px, py, x2, y2, x0, y0)
	hasNeg := (d1 < 0) || (d2 < 0) || (d3 < 0)
	hasPos := (d1 > 0) || (d2 > 0) || (d3 > 0)
	return !(hasNeg && hasPos)
}

func sign(px, py, x0, y0, x1, y1 int) int {
	return (px-x0)*(y1-y0) - (x1-x0)*(py-y0)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

func max3(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

// Degrees to radians
func deg2rad(d float64) float64 { return d * math.Pi / 180.0 }
