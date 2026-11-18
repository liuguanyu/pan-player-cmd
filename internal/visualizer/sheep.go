package visualizer

import "math"

// DrawMainSheep ports the car player's side-facing Annabelle: planted feet,
// two-segment knees, scalloped outlined wool and independently rotated head.
func DrawMainSheep(c *Canvas, s *SceneState, _ AudioAnalysis) {
	p := newScenePaint(c)
	scale := p.a
	w, h := float64(c.W)/scale, float64(c.H)/scale
	ground := h * .86
	rx, ry := 62*(1+s.Breathe*.04), 38.0
	baseX := w*.5 + 24
	bx := baseX + s.BodyShift
	by := ground - 74 - s.BodyLift - s.JumpLift + (s.Crouch+s.JumpSquash*.5)*30
	legX := []float64{-36, -20, 30, 46}
	tilt := deg2rad(s.BodyTilt)
	for i, x := range legX {
		hx, hy := bx+x*math.Cos(tilt), by+ry*.55-x*math.Sin(tilt)
		tap := math.Max(0, -s.LegTap)
		if i == 0 || i == 3 {
			tap = math.Max(0, s.LegTap)
		}
		fx, fy := baseX+x-tap*.4, ground-s.FeetLift-tap-s.JumpLift*.82
		drawJointedLeg(p, hx, hy, fx, fy, i < 2)
		if s.Outfit == OutfitCowboy || s.Outfit == OutfitPunk {
			clr := rgb(0x8b4a1c)
			if s.Outfit == OutfitPunk {
				clr = rgb(0xe0308a)
			}
			p.rect(fx-4, fy-9, 10, 10, clr)
		}
	}
	p = p.rotate(s.BodyTilt, bx, by)
	wool := white
	if s.Outfit == OutfitPunk {
		wool = rgb(0xff8cf0)
	}
	for i := 0; i < 22; i++ {
		a := float64(i) * math.Pi * 2 / 22
		p.ellipse(bx+math.Cos(a)*rx, by+math.Sin(a)*ry, 12.5, 12.5, black)
	}
	for i := 0; i < 22; i++ {
		a := float64(i) * math.Pi * 2 / 22
		p.ellipse(bx+math.Cos(a)*rx, by+math.Sin(a)*ry, 11, 11, wool)
	}
	p.ellipse(bx, by, rx, ry, wool)
	if s.Peace {
		x, y := bx+26, by-4
		clr := rgb(0x333333)
		p.ring(x, y, 10, 2, clr)
		p.line(x, y-10, x, y+10, 2, clr)
		p.line(x, y, x-7, y+7, 2, clr)
		p.line(x, y, x+7, y+7, 2, clr)
	}
	neckX, neckY := bx-rx+24, by-20
	hx, hy := bx-rx+4, by-16+s.HeadDrop
	p = p.rotate(4-s.HeadAngle, neckX, neckY)
	// Drooping right ear and long muzzle.
	p.polygon(black, hx+10, hy-18, hx+34, hy-15, hx+42, hy-4, hx+40, hy+18, hx+28, hy+8, hx+8, hy-6)
	p.ellipse(hx-3, hy-1, 27, 23, black)
	p.ellipse(hx-23, hy+10, 17, 16, black)
	drawMouth(p, s, hx-24, hy+14)
	drawEyes(p, s, hx-10, hy-6)
	drawHat(p, s.Outfit, hx-4, hy-22)
}
func drawJointedLeg(p scenePaint, hx, hy, fx, fy float64, front bool) {
	dx, dy := fx-hx, fy-hy
	d := math.Hypot(dx, dy)
	if d >= 53.5 || d < 1 {
		p.line(hx, hy, fx, fy, 4, black)
		return
	}
	bend := math.Sqrt(27*27 - d*d/4)
	nx, ny := -dy/d, dx/d
	dir := 1.0
	if front {
		dir = -1
	}
	if nx*dir < 0 {
		nx = -nx
		ny = -ny
	}
	kx, ky := hx+dx/2+nx*bend, hy+dy/2+ny*bend
	p.line(hx, hy, kx, ky, 4, black)
	p.line(kx, ky, fx, fy, 4, black)
}
func drawMouth(p scenePaint, s *SceneState, x, y float64) {
	if s.Face == FaceGrin && !s.ufoBeaming() {
		p.rect(x-6, y-4, 22, 7, white)
		for tx := x - 1; tx < x+16; tx += 5 {
			p.line(tx, y-4, tx, y+3, 1, black)
		}
		p.line(x-6, y, x+16, y, 1, black)
	}
	if s.Face == FaceTongue || s.ufoBeaming() {
		out := math.Min(1, s.faceT*4)
		if s.ufoBeaming() {
			out = 1
		}
		p.rect(x-2, y-2, 11, 8+out*10, rgb(0xff7aa0))
		p.ellipse(x+3.5, y+6+out*10, 5.5, 4, rgb(0xff7aa0))
	}
}
func drawEyes(p scenePaint, s *SceneState, x, y float64) {
	if s.ufoBeaming() {
		for _, dx := range []float64{-9, 11} {
			p.ellipse(x+dx, y, 9, 9, white)
			p.ellipse(x+dx, y, 3, 3, black)
		}
		return
	}
	if s.Outfit == OutfitSunglasses || s.Outfit == OutfitPunk {
		p.rect(x-18, y-6, 16, 13, rgb(0x8a8a8a))
		p.rect(x+2, y-6, 16, 13, rgb(0x8a8a8a))
		p.line(x-2, y-2, x+2, y-2, 2, rgb(0x8a8a8a))
		p.rect(x-15, y-4, 4, 3, rgb(0xd0d0d0))
		p.rect(x+5, y-4, 4, 3, rgb(0xd0d0d0))
		return
	}
	switch s.Face {
	case FaceBigEye:
		p.ellipse(x-9, y+1, 13, 13, black)
		p.ellipse(x-9, y+1, 12, 12, white)
		p.ellipse(x-6, y+2, 2.6, 2.6, black)
		p.ellipse(x+11, y-.5, 6, 5.5, white)
		p.rect(x+4, y-7, 14, 6, black)
		p.ellipse(x+11, y+2, 2.2, 2.2, black)
		return
	case FaceWink:
		p.ellipse(x-9, y-.5, 8, 7.5, white)
		p.ellipse(x-9, y, 3, 3, black)
		p.line(x+3, y, x+10, y+3, 2, white)
		p.line(x+10, y+3, x+17, y, 2, white)
		return
	case FaceCrossEyed:
		p.ellipse(x-9, y, 8, 8, white)
		p.ellipse(x+10, y, 8, 8, white)
		p.ellipse(x-4, y+2, 3, 3, black)
		p.ellipse(x+5, y+2, 3, 3, black)
		return
	}
	for _, dx := range []float64{-10, 10} {
		p.ellipse(x+dx, y-.5, 7, 6.5, white)
		p.ellipse(x+dx+1, y+2, 2.4, 2.4, black)
		lid := .45 + s.Blink*.55
		p.rect(x+dx-8, y-8, 16, 14*lid, black)
		p.line(x+dx-7, y-8+14*lid, x+dx+7, y-8+14*lid, 1.5, white)
	}
}
func drawHat(p scenePaint, o Outfit, x, y float64) {
	switch o {
	case OutfitCowboy:
		p.rect(x-16, y-22, 34, 24, rgb(0x8b4a1c))
		p.ellipse(x+1, y+1, 37, 5, rgb(0x8b4a1c))
		p.rect(x-16, y-6, 34, 4, rgb(0x5e3010))
	case OutfitTopHat:
		p.rect(x-13, y-34, 28, 34, black)
		p.ellipse(x+1, y, 27, 4, black)
		p.rect(x-13, y-8, 28, 4, rgb(0x444444))
	case OutfitRedCap:
		p.ellipse(x, y, 20, 14, rgb(0xe0222a))
		p.rect(x-21, y+1, 42, 15, black)
		p.ellipse(x-22, y+.5, 16, 3.5, rgb(0xa8141a))
	case OutfitPunk:
		p.polygon(rgb(0xff3aa8), x-10, y+4, x-6, y-16, x-1, y+2, x+3, y-20, x+7, y+2, x+12, y-14, x+14, y+6)
		p.ring(x-30, y+20, 4, 2, rgb(0xffd23a))
	case OutfitBow:
		p.polygon(rgb(0xff8ac8), x, y+2, x-14, y-8, x-14, y+12)
		p.polygon(rgb(0xff8ac8), x, y+2, x+14, y-8, x+14, y+12)
		p.ellipse(x, y+2, 4, 4, rgb(0xe04a9a))
	}
}
