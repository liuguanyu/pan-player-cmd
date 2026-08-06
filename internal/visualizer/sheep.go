package visualizer

import (
	"image/color"
	"math"
)

// DrawMainSheep renders the foreground sheep, ported from Electron draw-sheep.ts.
// It reads animation parameters from SceneState.
func DrawMainSheep(c *Canvas, state *SceneState, analysis AudioAnalysis) {
	w, h := c.W, c.H
	centerX := float64(w) * 0.44
	centerY := float64(h)*0.57 + state.BodyBob

	woolColor := ColorSheepWool
	legColor := ColorSheepHead
	if state.PinkMode {
		woolColor = ColorSheepPink
		legColor = ColorSheepPink
	}

	swaggerStride := 0.0
	if state.Swagger.Active {
		swaggerStride = math.Sin(state.SwaggerPhase) * 6
	}

	legLift := math.Sin(state.LegPhase) * 2
	switch analysis.Intensity {
	case IntensityHigh:
		legLift = math.Sin(state.LegPhase) * 8
	case IntensityMid:
		legLift = math.Sin(state.LegPhase) * 5
	}

	// --- Spotlight / shadow ---
	drawSpotlight(c, centerX+float64(w)*0.03, float64(h)*0.74, float64(w)/640.0, state.Disco.Active || state.Microphone.Active)

	// --- Microphone ---
	if state.Microphone.Active {
		drawMicrophone(c, int(centerX-92), int(float64(h)*0.67), float64(w)/640.0)
	}

	// --- Legs (before body so body renders on top) ---
	lw := int(4 * float64(w) / 640.0)
	drawLeg := func(baseX, baseY float64, lift float64) {
		endY := baseY + 48 - math.Max(0, lift)
		c.StrokeLine(int(baseX), int(baseY), int(baseX), int(endY), lw, legColor)
	}
	drawLeg(centerX-18+swaggerStride, centerY+20, legLift)
	drawLeg(centerX-4-swaggerStride*0.25, centerY+20, -legLift)
	drawLeg(centerX+18+swaggerStride*0.15, centerY+22, -legLift*0.6)
	drawLeg(centerX+34-swaggerStride, centerY+22, legLift*0.6)

	// --- Wool body (16 overlapping circles for fluffy look) ---
	bodyScaleX := 1.0 + state.BodyStretch
	bodyScaleY := 1.0 - state.BodyStretch*0.45
	drawWoolBody(c, int(centerX), int(centerY), woolColor, bodyScaleX, bodyScaleY)

	// --- Head ---
	headX := centerX - 56 + state.HeadSwing*0.8
	headY := centerY - 6 + state.HeadNod*0.8
	headTiltExtra := 0.0
	goofy := state.Goofy.Active
	goofyType := state.GoofyType
	if goofy && goofyType == 1 {
		headTiltExtra = 0.08
	}
	drawSheepHead(c, int(headX), int(headY), state.TongueValue, state.EyeWide, state.Eyelid, state.PinkMode, goofy, goofyType, state.HeadSwing*0.012+headTiltExtra)

	// --- Pink bow ---
	if state.PinkMode {
		c.FillRect(int(centerX-38), int(centerY-12), 8, 4, ColorTongue)
		c.FillRect(int(centerX-34), int(centerY-16), 4, 12, ColorTongue)
	}
}

// drawWoolBody renders the fluffy wool ellipse made of overlapping circles.
func drawWoolBody(c *Canvas, cx, cy int, woolColor color.RGBA, scaleX, scaleY float64) {
	nPuffs := 16
	rx := 48.0 * scaleX
	ry := 30.0 * scaleY
	puffR := 10.0 * (scaleX + scaleY) / 2.0

	for i := 0; i < nPuffs; i++ {
		angle := float64(i) / float64(nPuffs) * math.Pi * 2
		px := int(float64(cx) + math.Cos(angle)*rx)
		py := int(float64(cy) + math.Sin(angle)*ry)
		c.FillCircle(px, py, max(1, int(puffR)), woolColor)
	}

	// Fill the center to avoid gaps
	c.FillEllipse(cx, cy, max(1, int(rx+2)), max(1, int(ry+2)), woolColor)
}

// drawSheepHead renders the sheep head with ears, eyes, and tongue.
func drawSheepHead(c *Canvas, cx, cy int, tongueValue, eyeWide, eyelid float64, pinkMode, goofy bool, goofyType int, headTilt float64) {
	// Head body (black ellipse)
	c.FillEllipse(cx, cy, 24, 18, ColorSheepHead)

	// Ears (triangles)
	c.FillTriangle(cx-18, cy-2, cx-44, cy+4, cx-16, cy+6, ColorSheepHead)
	c.FillTriangle(cx+18, cy-2, cx+38, cy+18, cx+14, cy+12, ColorSheepHead)

	// Pink hair tuft
	if pinkMode {
		c.StrokeLine(cx-2, cy-16, cx-6, cy-30, 3, ColorSheepPink)
		c.StrokeLine(cx+2, cy-15, cx+4, cy-28, 3, ColorSheepPink)
	}

	// Eyes
	isSquint := goofy && goofyType == 1
	isPuff := goofy && goofyType == 2
	eyeW := 8
	eyeH := 8 + int(eyeWide*2)
	if goofy && goofyType == 0 {
		eyeW = 9
		eyeH = 11
	}
	if isPuff {
		eyeW = 9
		eyeH = 10
	}
	if isSquint {
		eyeW = 10
		eyeH = 3
	}

	// Left eye
	c.FillRect(cx-12, cy-8, eyeW, eyeH, ColorSheepEye)
	// Right eye
	c.FillRect(cx+4, cy-8, eyeW, eyeH, ColorSheepEye)

	// Pupils
	pupilY := cy - 3
	if goofy && goofyType == 0 {
		pupilY = cy - 6
	}
	if isSquint {
		pupilY = cy - 7
	}
	c.FillRect(cx-9, pupilY, 2, 2, ColorSheepPupil)
	c.FillRect(cx+7, pupilY, 2, 2, ColorSheepPupil)

	// Eyelid line
	lidOffset := 0.0
	if goofy {
		lidOffset = -4
	}
	if isSquint {
		lidOffset = 1
	} else {
		lidOffset += eyelid * 4
	}
	lidY := cy - 9 + int(lidOffset)
	c.StrokeLine(cx-14, lidY, cx-2, lidY, 1, ColorSheepPupil)
	c.StrokeLine(cx+2, lidY, cx+14, lidY, 1, ColorSheepPupil)

	// Puff cheeks
	if isPuff {
		c.FillEllipse(cx-6, cy+10, 12, 8, ColorSheepHead)
	}

	// Tongue
	if tongueValue > 0.05 {
		tx := cx - 2
		if isSquint {
			tx = cx + 8
		}
		tLen := int(tongueValue * 13)
		c.FillEllipse(tx, cy+12+int(tongueValue*5), 5, 5+tLen, ColorTongue)
	}
}
