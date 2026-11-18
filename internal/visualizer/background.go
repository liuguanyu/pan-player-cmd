package visualizer

import "math"

func DrawBackground(c *Canvas, s *SceneState) {
	p := newScenePaint(c)
	w, h := float64(c.W)/p.a, float64(c.H)/p.a
	night := NightFactor(s.Clock)
	horizon, ground := h*.5, h*.86
	top := lerpRGBA(rgb(0x8c96ff), rgb(0x0b0f3a), night)
	bottom := lerpRGBA(rgb(0x0a1ef0), rgb(0x1a2a86), night)
	// The gradient ends at the horizon, not the bottom of the grass.
	for y := 0; y < int(horizon*p.a); y++ {
		c.FillRect(0, y, c.W, 1, lerpRGBA(top, bottom, float64(y)/(horizon*p.a)))
	}
	if night > 0 {
		for _, star := range s.Stars {
			tw := .55 + .45*math.Sin(s.Clock*2.4+star[2])
			clr := white
			clr.A = uint8(255 * night * tw)
			p.rect(star[0]*w-1, star[1]-1, 2, 2, clr)
		}
		moon := rgb(0xfff04a)
		moon.A = uint8(255 * night)
		p.ellipse(w*.86, 46, 22, 22, moon)
		cut := lerpRGBA(bottom, rgb(0x0b0f3a), .7)
		cut.A = uint8(255 * night)
		p.ellipse(w*.86-9, 42, 22*.92, 22*.92, cut)
	}
	cloud := lerpRGBA(white, rgb(0x6e6e78), night)
	for _, cl := range s.Clouds {
		x, y, z := cl[0]*w, cl[1], cl[2]
		p.ellipse(x, y, 34*z, 7*z, cloud)
		p.ellipse(x-2*z, y-5*z, 18*z, 8*z, cloud)
		p.ellipse(x+11*z, y-3.5*z, 15*z, 7.5*z, cloud)
	}
	if s.BlimpX >= 0 {
		x := -70 + s.BlimpX*(w+140)
		if s.BlimpDir < 0 {
			x = w + 70 - s.BlimpX*(w+140)
		}
		drawBlimp(p, s, x, 50)
	}
	p.rect(0, horizon, w, h-horizon, lerpRGBA(rgb(0x22e022), rgb(0x119a11), night))
	if s.BirdsX >= 0 {
		clr := lerpRGBA(white, rgb(0x9a9aa8), night)
		flap := math.Sin(s.Clock*9) * 3
		for _, b := range [][2]float64{{0, 0}, {26, -10}, {44, 6}, {70, -4}} {
			x := w*(1.05-s.BirdsX*1.2) + b[0]
			y := s.BirdsY + b[1] + math.Sin(s.Clock*1.3+b[0])*2
			p.line(x-6, y-flap, x, y, 1.6, clr)
			p.line(x, y, x+6, y-flap, 1.6, clr)
		}
	}
	fence := lerpRGBA(white, rgb(0xc8c8d8), night)
	x0, y0, x1, y1 := 14.0, horizon+80, 74.0, horizon-20
	for i := 0; i < 3; i++ {
		t := float64(i) / 2
		x, y := x0+(x1-x0)*t, y0+(y1-y0)*t
		p.line(x, y-6, x, y+52-18*t, 5, fence)
	}
	p.line(x0-6, y0+4, x1+6, y1-2, 4, fence)
	p.line(x0-6, y0+24, x1+6, y1+12, 4, fence)
	if s.DiscoT >= 0 {
		drawDisco(p, s, w, h, horizon)
	}
	if s.CatT >= 0 {
		catX := w*.5 - 190
		left := catX + 18
		grow, shrink := math.Min(1, s.CatT/2), math.Min(1, (CatSeconds-s.CatT)/2)
		from := w - (w-left)*grow + (w-left)*(1-shrink)
		grass := lerpRGBA(rgb(0x14b414), rgb(0x0a700a), night)
		for x := left; x < w; x += 16 {
			if x < from {
				continue
			}
			p.line(x, h*.96, x+8, h*.96-8, 2, grass)
			p.line(x+8, h*.96-8, x+16, h*.96, 2, grass)
		}
		drawCat(p, s, catX, h*.96)
	}
	if s.UfoT >= 0 {
		drawUfo(p, s, w*.5+30, ground)
	}
}
