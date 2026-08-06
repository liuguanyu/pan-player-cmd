package visualizer

import "math"

// DrawBackground renders sky, grass, fence, moon/stars, and clouds.
func DrawBackground(c *Canvas, state *SceneState) {
	w, h := c.W, c.H

	// --- Sky gradient ---
	if state.IsNight {
		c.DrawGradientVertical(ColorSkyTopNight, ColorSkyBotNight)
	} else {
		c.DrawGradientVertical(ColorSkyTopDay, ColorSkyBotDay)
	}

	// --- Stars (only at night) ---
	if state.IsNight {
		drawStars(c, state)
	}

	// --- Moon (only at night) ---
	if state.IsNight {
		mx := int(float64(w) * 0.85)
		my := int(float64(h) * 0.12)
		c.FillCircle(mx, my, 18, ColorMoon)
	}

	// --- Clouds ---
	drawClouds(c, state)

	// --- Birds ---
	for _, bird := range state.Birds {
		drawBird(c, bird)
	}

	// --- Grass ---
	grassTop := int(float64(h) * 0.65)
	c.FillRect(0, grassTop, w, h-grassTop, ColorGrass)

	// --- Fence ---
	drawFence(c, w, grassTop)

	// --- Far sheep ---
	for _, fs := range state.FarSheep {
		drawFarSheep(c, fs)
	}

	// --- Spectrum Lamp ---
	if state.SpectrumLamp.Active {
		drawSpectrumLamp(c, float64(w)*0.75, float64(h)*0.15, float64(w)/640.0, state)
	}
}

func drawClouds(c *Canvas, state *SceneState) {
	w := float64(c.W)
	h := float64(c.H)
	// Static cloud positions
	clouds := []struct{ x, y, rw, rh float64 }{
		{w * 0.2, h * 0.15, 60, 20},
		{w * 0.6, h * 0.1, 80, 24},
		{w * 0.9, h * 0.2, 50, 16},
	}
	for _, cl := range clouds {
		cx := int(cl.x)
		cy := int(cl.y)
		rx := int(cl.rw)
		ry := int(cl.rh)
		c.FillEllipse(cx, cy, rx, ry, ColorCloud)
		c.FillEllipse(cx+int(cl.rw*0.3), cy-5, int(cl.rw*0.5), int(cl.rh*0.6), ColorCloud)
		c.FillEllipse(cx-int(cl.rw*0.2), cy+2, int(cl.rw*0.4), int(cl.rh*0.5), ColorCloud)
	}
}

func drawStars(c *Canvas, state *SceneState) {
	// Simple fixed star positions (pixel-style dots)
	stars := [][2]int{
		{100, 30}, {200, 50}, {350, 20}, {500, 60}, {580, 35},
		{150, 70}, {420, 45}, {550, 25}, {250, 15}, {480, 55},
	}
	for _, s := range stars {
		c.FillRect(s[0], s[1], 3, 3, ColorStar)
	}
}

func drawFence(c *Canvas, w, grassTop int) {
	fenceY := grassTop + 2
	postH := 20
	postW := 3
	railH := 3

	for x := 0; x < w; x += 30 {
		c.FillRect(x, fenceY, postW, postH, ColorFence)
	}
	// Top rail
	c.FillRect(0, fenceY+4, w, railH, ColorFence)
	// Bottom rail
	c.FillRect(0, fenceY+14, w, railH, ColorFence)
}

func drawBird(c *Canvas, bird Bird) {
	x := int(bird.X)
	y := int(bird.Y)
	wingFlap := int(math.Sin(bird.WingPhase) * 4)

	// Simple V-shaped bird
	c.StrokeLine(x-4, y-wingFlap, x, y, 1, ColorSheepHead)
	c.StrokeLine(x, y, x+4, y-wingFlap, 1, ColorSheepHead)
}

func drawFarSheep(c *Canvas, fs FarSheep) {
	x := int(fs.X)
	y := int(fs.Y) + int(fs.HopOffset)
	scale := int(fs.Scale * 50)

	// Body
	c.FillEllipse(x, y, scale, scale/2, ColorSheepWool)
	// Head
	c.FillEllipse(x-scale, y-scale/4, scale/2, scale/3, ColorSheepHead)
	// Legs
	legH := scale / 2
	c.StrokeLine(x-scale/4, y+scale/2, x-scale/4, y+scale/2+legH, 2, ColorSheepHead)
	c.StrokeLine(x+scale/4, y+scale/2, x+scale/4, y+scale/2+legH, 2, ColorSheepHead)
}
