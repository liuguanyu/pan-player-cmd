package visualizer

import "math"

func drawBlimp(p scenePaint, s *SceneState, x, y float64) {
	p.ellipse(x, y, 52, 22, rgb(0xd8d8d8))
	p.ellipse(x, y+12.5, 48, 8.5, rgb(0xbdbdbd))
	dir := -s.BlimpDir
	p.polygon(rgb(0xc4c4c4), x+dir*40, y-6, x+dir*66, y-26, x+dir*66, y-4)
	p.polygon(rgb(0xc4c4c4), x+dir*40, y+6, x+dir*66, y+26, x+dir*66, y+4)
	p.rect(x-12, y+21, 22, 7, rgb(0x9a9a9a))
	left, top, bottom := x-36, y-14, y+13
	p.rect(left-2, top-2, 70, 31, rgb(0x101010))
	for i := 0; i < 10; i++ {
		bx := left + float64(i)*6.6
		lit := int(math.Round(s.Lamp[i] * 7))
		for seg := 0; seg < 7; seg++ {
			clr := rgb(0x38f93e)
			if seg >= 6 {
				clr = rgb(0xff3131)
			} else if seg >= 4 {
				clr = rgb(0xffe14a)
			}
			if seg >= lit {
				clr.A = 38
			}
			sy := bottom - float64(seg+1)*27/7
			p.rect(bx+.8, sy+.8, 5, 27.0/7-1.6, clr)
		}
		peak := min(6, int(s.LampPeak[i]*7))
		if s.LampPeak[i] > .02 && peak >= lit {
			py := bottom - float64(peak+1)*27/7
			p.rect(bx+.8, py+27.0/7-2.2, 5, 1.4, white)
		}
	}
}
func drawDisco(p scenePaint, s *SceneState, w, h, horizon float64) {
	drop := 1.0
	if s.DiscoT < 3 {
		drop = s.DiscoT / 3
	} else if s.DiscoT > 23 {
		drop = math.Max(0, (26-s.DiscoT)/3)
	}
	x, y := w*.42, -30+drop*80
	p.line(x, 0, x, y-18, 1.5, rgb(0x9a9a9a))
	p.ellipse(x, y, 22, 22, rgb(0xdcdcdc))
	for i := 0; i < 25; i++ {
		gx, gy := x-17+float64(i%5)*7, y-17+float64(i/5)*7
		if math.Hypot(gx+2.5-x, gy+2.5-y) > 19 {
			continue
		}
		p.rect(gx, gy, 5, 5, lerpRGBA(rgb(0x9a9a9a), white, .5+.5*math.Sin(s.Clock*6+float64(i)*1.7)))
	}
	if s.SpotGlow > .01 {
		clr := white
		clr.A = uint8(255 * math.Min(1, s.SpotGlow))
		for _, spot := range s.Spots {
			p.ellipse(spot[0]*w, horizon+20+spot[1]*(h-horizon-30), 10, 5, clr)
		}
	}
}
func drawUfo(p scenePaint, s *SceneState, x, ground float64) {
	t := s.UfoT
	hover := 34 + math.Sin(s.Clock*3)*3
	y := hover
	if t < 2.5 {
		y = -30 + (hover+30)*smooth(t/2.5)
	} else if t > UfoSeconds-2 {
		y = hover - (hover+30)*smooth((t-(UfoSeconds-2))/2)
	}
	if s.ufoBeaming() {
		a := math.Min(1, math.Min(t-2.5, UfoSeconds-2-t)*2)
		clr := rgb(0x3ce628)
		clr.A = uint8(170 * a)
		p.polygon(clr, x-7, y+6, x+7, y+6, x+34, ground-70, x-34, ground-70)
	}
	p.ellipse(x, y+1, 22, 6, rgb(0xb8b8c0))
	p.ellipse(x, y-4, 10, 8, rgb(0xe8e8f0))
	p.ellipse(x, y+1, 22, 4, rgb(0xb8b8c0))
	for i := 0; i < 3; i++ {
		clr := rgb(0x3a8a2a)
		if (int(s.Clock*4)+i)%3 == 0 {
			clr = rgb(0x7cff5a)
		}
		p.ellipse(x-12+float64(i)*12, y+2, 2, 2, clr)
	}
}
func drawCat(p scenePaint, s *SceneState, x, y float64) {
	// Use the full 40-second visit (the Java renderer accidentally faded at 15s).
	enter := clamp(math.Min(s.CatT/.6, (CatSeconds-s.CatT)/.6), 0, 1)
	orange := rgb(0xf08a24)
	orange.A = uint8(255 * enter)
	p.ellipse(x, y-10, 14, 16, orange)
	p.ellipse(x, y-32, 11, 11, orange)
	p.polygon(orange, x-10, y-38, x-8, y-48, x-2, y-41)
	p.polygon(orange, x+10, y-38, x+8, y-48, x+2, y-41)
	feet := rgb(0xb85a10)
	feet.A = orange.A
	p.rect(x-8, y-4, 5, 10, feet)
	p.rect(x+3, y-4, 5, 10, feet)
	eyes := black
	eyes.A = orange.A
	p.ellipse(x-4, y-33, 1.6, 1.6, eyes)
	p.ellipse(x+4, y-33, 1.6, 1.6, eyes)
}
