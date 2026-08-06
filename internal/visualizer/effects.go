package visualizer

import (
	"image/color"
	"math"
)

// --- Spotlight ---

func drawSpotlight(c *Canvas, cx, cy, scale float64, active bool) {
	alpha := 0.55
	if active {
		alpha = 0.92
	}
	rx := int(90 * scale)
	ry := int(32 * scale)
	// Render as a soft ellipse - approximate by drawing thin ellipses
	for i := 0; i < 3; i++ {
		offset := i
		clr := ColorShadow
		clr.A = uint8(float64(clr.A) * alpha * (1.0 - float64(i)*0.3))
		c.FillEllipse(int(cx), int(cy), rx-offset, ry-offset, clr)
	}
}

// --- Disco Ball ---

func drawDiscoBall(c *Canvas, w, h int, beat float64) {
	x := int(float64(w) * 0.52)
	y := int(float64(h) * 0.14)
	radius := int(float64(h) * 0.07)

	// Wire
	c.StrokeLine(x, 0, x, y-radius, 3, ColorDiscoWire)

	// Ball
	c.FillCircle(x, y, radius, ColorDiscoBall)

	// Grid texture
	cellSize := radius / 4
	if cellSize < 2 {
		cellSize = 2
	}
	for row := -3; row <= 3; row++ {
		for col := -3; col <= 3; col++ {
			cx := x + col*cellSize
			cy := y + row*cellSize
			dist := math.Sqrt(float64(col*col + row*row))
			if dist < 3.6 {
				clr := ColorDiscoBall
				if row == col || (row+col)%3 == 0 {
					clr.A = 220
				}
				c.FillRect(cx, cy, cellSize-1, cellSize-1, clr)
			}
		}
	}

	// Ground spots
	for i := 0; i < 9; i++ {
		px := int(float64(w) * (0.1 + float64(i)*0.1))
		py := int(float64(h) * (0.7 + float64((i%3)-1)*0.04))
		spotR := 12 + (i%2)*4 + int(beat*6)
		c.FillEllipse(px, py, spotR, spotR/2, ColorCloud)
	}
}

// --- Microphone ---

func drawMicrophone(c *Canvas, x, y int, scale float64) {
	headW := int(10 * scale)
	headH := int(12 * scale)
	stickH := int(36 * scale)

	// Head
	c.FillCircle(x+headW/2, y+headH/2, headW/2, ColorMicHead)

	// Stick
	c.StrokeLine(x+headW/2, y+headH, x+headW/2, y+headH+stickH, int(3*scale), ColorMicBody)

	// Base
	c.FillRect(x-int(2*scale), y+headH+stickH, headW+int(4*scale), int(4*scale), ColorMicBody)
}

// --- Spectrum Lamp (airship with frequency bars) ---

func drawSpectrumLamp(c *Canvas, x, y, scale float64, state *SceneState) {
	shellW := int(36 * scale)
	shellH := int(22 * scale)
	ix, iy := int(x), int(y)

	// Shell
	c.FillEllipse(ix+shellW/2, iy+shellH/2, shellW/2, shellH/2, ColorSpectShell)

	// Window
	c.FillRect(ix+int(8*scale), iy+int(5*scale), int(18*scale), int(12*scale), ColorSheepHead)

	// Tail fin
	c.FillRect(ix-int(5*scale), iy+int(6*scale), int(6*scale), int(8*scale), ColorSpectShell)

	// Gondola
	c.FillRect(ix+int(11*scale), iy+shellH, int(8*scale), int(4*scale), ColorSpectShell)

	// Frequency bars (use current state pulse values)
	bars := []float64{state.BeatPulse, state.SheepPulse, state.BeatPulse * 0.7, state.SheepPulse * 0.8}
	barColors := []color.RGBA{
		{0x38, 0xf9, 0x3e, 0xff},
		{0xc8, 0xff, 0x33, 0xff},
		{0xff, 0x9b, 0x1e, 0xff},
		{0xff, 0x31, 0x31, 0xff},
	}
	for i, val := range bars {
		bh := int(math.Max(3, val*10))
		c.FillRect(
			ix+int(10*scale)+i*int(4*scale),
			iy+int(float64(16-bh)*scale),
			int(3*scale),
			bh,
			barColors[i],
		)
	}
}
