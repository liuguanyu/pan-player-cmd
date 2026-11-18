package visualizer

import (
	"math"
	"math/rand"
	"time"
)

const (
	DaySeconds        = 150.0
	NightSeconds      = 150.0
	TransitionSeconds = 15.0
	CycleSeconds      = DaySeconds + NightSeconds + 2*TransitionSeconds
	Gravity           = 540.0
	MaxJump           = 90.0
	CatSeconds        = 40.0
	UfoSeconds        = 9.0
)

func smooth(x float64) float64 { x = clamp(x, 0, 1); return x * x * (3 - 2*x) }
func NightFactor(clock float64) float64 {
	t := math.Mod(clock, CycleSeconds)
	if t < 0 {
		t += CycleSeconds
	}
	if t < DaySeconds {
		return 0
	}
	t -= DaySeconds
	if t < TransitionSeconds {
		return smooth(t / TransitionSeconds)
	}
	t -= TransitionSeconds
	if t < NightSeconds {
		return 1
	}
	return 1 - smooth((t-NightSeconds)/TransitionSeconds)
}
func beatPulse(pos float64) float64 { c := math.Cos(math.Pi * pos); return c * c }
func NewSceneState(_ ViewportSize) *SceneState {
	s := &SceneState{LastFrameTime: time.Now(), rng: rand.New(rand.NewSource(7)), tracker: NewBeatTracker(),
		BlinkTimer: 3, SecondsPerBeat: .5, barsUntilJump: 3, moveBeats: 8, lastBeat: -1, faceTimer: 6, outfitTimer: 20,
		BlimpX: -1, BlimpDir: -1, blimpTimer: 25, DiscoT: -1, discoTimer: 30, CatT: -1, catTimer: 40,
		BirdsX: -1, birdsTimer: 15, BirdsY: 60, UfoT: -1, ufoTimer: 70,
		Clouds: [3][3]float64{{.12, 40, 1}, {.45, 70, .8}, {.78, 30, 1.1}}}
	for i := range s.Stars {
		s.Stars[i] = [3]float64{s.rng.Float64(), 8 + s.rng.Float64()*150, s.rng.Float64() * 2 * math.Pi}
	}
	s.shuffleSpots()
	return s
}
func (s *SceneState) shuffleSpots() {
	for i := range s.Spots {
		s.Spots[i] = [2]float64{s.rng.Float64(), s.rng.Float64()}
	}
}
func (s *SceneState) Update(a AudioAnalysis) {
	now := time.Now()
	dt := now.Sub(s.LastFrameTime).Seconds()
	s.LastFrameTime = now
	s.Advance(a, dt, true)
}

// Advance uses bounded real frame time; hidden windows must reset LastFrameTime
// rather than catch up hours of animation on the next frame.
func (s *SceneState) Advance(a AudioAnalysis, dt float64, playing bool) {
	dt = clamp(dt, 0, .05)
	target, rate := 0.0, 3.0
	if playing {
		target = 1
		rate = 6
	}
	s.Activity += (target - s.Activity) * math.Min(1, dt*rate)
	if !playing && s.Activity < .01 {
		s.Activity = 0
	}
	k := math.Min(1, dt*6)
	if playing {
		k = 1 - math.Exp(-dt*42)
	}
	if !playing {
		a = AudioAnalysis{}
	}
	s.Bass += (a.BassEnergy - s.Bass) * k
	s.Treble += (a.TrebleEnergy - s.Treble) * k
	s.Volume += (a.AverageVolume - s.Volume) * k
	step := dt * s.Activity
	s.Clock += step
	if playing {
		s.tracker.Feed(a.Spectrum, s.Clock)
	}
	s.BeatPos = s.tracker.position(s.Clock + .07)
	s.BeatConfidence = s.tracker.Confidence(s.Clock)
	s.SecondsPerBeat = s.tracker.period
	s.advanceDance(dt)
	s.Breathe += (s.Volume*s.Activity - s.Breathe) * math.Min(1, dt*4)
	if s.faceT > 0 {
		s.faceT -= step
		if s.faceT <= 0 {
			s.faceT = 0
			s.Face = FaceNormal
		}
	} else {
		s.faceTimer -= step
	}
	s.BlinkTimer -= step
	if s.BlinkTimer <= 0 {
		s.Blink = 1
		s.BlinkTimer = 2.5 + s.rng.Float64()*4
	}
	s.Blink = math.Max(0, s.Blink-dt*6)
	s.outfitTimer -= step
	if s.outfitTimer <= 0 {
		next := OutfitNone
		if s.Outfit == OutfitNone || s.rng.Float64() >= .55 {
			for {
				next = Outfit(1 + s.rng.Intn(6))
				if next != s.Outfit {
					break
				}
			}
		}
		s.Outfit = next
		s.Peace = next == OutfitSunglasses || (next == OutfitNone && s.rng.Float64() < .25)
		s.outfitTimer = 18 + s.rng.Float64()*18
	}
	for i := range s.Clouds {
		s.Clouds[i][0] += step * .006
		if s.Clouds[i][0] > 1.15 {
			s.Clouds[i][0] = -.15
		}
	}
	s.updateEqualizer(a.Spectrum, dt)
	night := NightFactor(s.Clock)
	if s.BlimpX < 0 {
		s.blimpTimer -= step
		if s.blimpTimer <= 0 && night < .2 {
			s.BlimpX = 0
			s.BlimpDir = -1
			if s.rng.Intn(2) == 0 {
				s.BlimpDir = 1
			}
		}
	} else {
		s.BlimpX += step * .025
		if s.BlimpX > 1 {
			s.BlimpX = -1
			s.blimpTimer = 40 + s.rng.Float64()*30
		}
	}
	if s.DiscoT < 0 {
		s.discoTimer -= step
		if s.discoTimer <= 0 && night > .9 && s.UfoT < 0 {
			s.DiscoT = 0
		}
	} else {
		s.DiscoT += step
		if s.DiscoT > 26 {
			s.DiscoT = -1
			s.discoTimer = 45 + s.rng.Float64()*30
		}
	}
	discoOn := s.DiscoT > 3 && s.DiscoT < 23
	if discoOn && a.Peak && s.Activity > .5 {
		s.shuffleSpots()
		s.SpotGlow = 1
	}
	glow := 0.0
	if discoOn {
		glow = (.8 + s.Bass*.2) * s.Activity
	}
	s.SpotGlow += (glow - s.SpotGlow) * math.Min(1, dt*4)
	if s.CatT < 0 {
		s.catTimer -= step
		if s.catTimer <= 0 {
			s.CatT = 0
		}
	} else {
		s.CatT += step
		if s.CatT > CatSeconds {
			s.CatT = -1
			s.catTimer = 60 + s.rng.Float64()*60
		}
	}
	if s.BirdsX < 0 {
		s.birdsTimer -= step
		if s.birdsTimer <= 0 {
			s.BirdsX = 0
			s.BirdsY = 40 + s.rng.Float64()*50
		}
	} else {
		s.BirdsX += step * .06
		if s.BirdsX > 1 {
			s.BirdsX = -1
			s.birdsTimer = 35 + s.rng.Float64()*40
		}
	}
	if s.UfoT < 0 {
		s.ufoTimer -= step
		if s.ufoTimer <= 0 && night > .9 && s.DiscoT < 0 {
			s.UfoT = 0
		}
	} else {
		s.UfoT += step
		if s.UfoT > UfoSeconds {
			s.UfoT = -1
			s.ufoTimer = 80 + s.rng.Float64()*60
		}
	}
}
func (s *SceneState) advanceDance(dt float64) {
	beat := int64(math.Floor(s.BeatPos))
	if beat != s.lastBeat {
		first := s.lastBeat < 0
		s.lastBeat = beat
		if !first && s.Activity > .5 {
			s.onBeat()
		}
	}
	s.updateJump(dt)
	amp := s.Activity * (.35 + .65*s.BeatConfidence) * math.Min(1.3, .75+s.Bass*.9)
	pulse, alt, sway := beatPulse(s.BeatPos), math.Cos(math.Pi*s.BeatPos), math.Sin(math.Pi*s.BeatPos/2)
	lift, shift, tilt, crouch, angle, drop, tap := 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0
	switch s.Move {
	case MoveBob:
		angle = pulse*34 - 6
		drop = pulse * 18
		lift = (1 - pulse) * 8
		crouch = pulse * .15
	case MoveHop:
		angle = pulse*14 - 4
		drop = pulse * 6
		tilt = -alt * 3
	case MoveLunge:
		shift = alt * 26
		tilt = -alt * 6
		angle = alt*14 + 6
		drop = pulse * 6
		crouch = .12
	case MoveCrouch:
		crouch = .35 + pulse*.35
		angle = pulse * 30
		drop = pulse * 14
	case MoveTap:
		shift = sway * 14
		tilt = sway * 6
		tap = alt * 18
		angle = sway*22 + pulse*8
		lift = pulse * 6
	}
	k := math.Min(1, dt*20)
	approach := func(v *float64, t float64) { *v += (t*amp - *v) * k }
	approach(&s.BodyLift, lift)
	approach(&s.BodyShift, shift)
	approach(&s.BodyTilt, tilt)
	approach(&s.Crouch, crouch)
	approach(&s.HeadAngle, angle)
	approach(&s.HeadDrop, drop)
	approach(&s.LegTap, tap)
}
func (s *SceneState) onBeat() {
	if s.jumpPhase == jumpGround && s.BeatConfidence > .5 {
		if s.lastBeat%4 == 3 {
			s.barsUntilJump--
			if s.barsUntilJump <= 0 {
				s.scheduleJump(true)
				s.barsUntilJump = 2 + s.rng.Intn(4)
			}
		}
		if s.jumpPhase == jumpGround && s.Move == MoveHop && s.lastBeat%2 == 1 {
			s.scheduleJump(false)
		}
	}
	s.moveBeats--
	if s.moveBeats <= 0 && s.lastBeat%4 == 0 {
		old := s.Move
		for s.Move == old {
			s.Move = Move(s.rng.Intn(5))
		}
		s.moveBeats = 4 * (1 + s.rng.Intn(2))
	}
	if s.faceT <= 0 && s.faceTimer <= 0 {
		s.Face = Face(1 + s.rng.Intn(5))
		s.faceT = 1.6 + s.rng.Float64()*1.2
		s.faceTimer = 7 + s.rng.Float64()*9
	}
}
func (s *SceneState) scheduleJump(big bool) {
	n := 1
	if big {
		n = 2
	}
	apex := func(n int) float64 { t := float64(n) * s.SecondsPerBeat; return Gravity * t * t / 8 }
	if big && apex(2) > MaxJump {
		n = 1
	}
	if !big && apex(1) < 10 {
		n = 2
	}
	s.jumpStart = float64(s.lastBeat + 1)
	s.jumpAirBeats = n
	s.jumpPhase = jumpPrep
}
func (s *SceneState) updateJump(dt float64) {
	s.JumpLift = 0
	squash := 0.0
	if s.jumpPhase == jumpPrep {
		left := s.jumpStart - s.BeatPos
		if s.Activity < .3 {
			s.jumpPhase = jumpGround
		} else if left <= 0 {
			s.jumpPhase = jumpAir
			s.jumpAirSeconds = float64(s.jumpAirBeats) * s.SecondsPerBeat
			s.jumpT = math.Min(-left*s.SecondsPerBeat, dt)
		} else if left < .45 {
			squash = math.Sin((1 - left/.45) * math.Pi / 2)
		}
	}
	switch s.jumpPhase {
	case jumpAir:
		if s.jumpT >= s.jumpAirSeconds {
			s.jumpPhase = jumpLand
			s.jumpT = 0
		} else {
			t := s.jumpT
			s.JumpLift = Gravity * t * (s.jumpAirSeconds - t) / 2
			squash = math.Max(0, 1-t/.06)
			s.jumpT += dt
		}
	case jumpLand:
		v := s.jumpT / .16
		if v >= 1 {
			s.jumpPhase = jumpGround
		} else {
			squash = math.Sin(math.Pi*math.Min(1, v*1.6+.25)) * (1 - v) * 1.1
			s.jumpT += dt
		}
	}
	scale := .6
	if s.jumpAirBeats >= 2 {
		scale = 1
	}
	s.JumpSquash = math.Max(0, squash) * scale
}
func (s *SceneState) updateEqualizer(spectrum []float64, dt float64) {
	n := len(spectrum)
	for i := range s.Lamp {
		target := 0.0
		if n > 1 {
			from := max(1, int(math.Pow(float64(n-1), float64(i)/10)))
			to := min(n, max(from+1, int(math.Pow(float64(n-1), float64(i+1)/10))))
			for j := from; j < to; j++ {
				target = math.Max(target, spectrum[j])
			}
			target = math.Min(1, math.Sqrt(math.Min(1, target*(1+float64(i)*.22)*1.6)))
		}
		target *= s.Activity
		rate := 12.0
		if target > s.Lamp[i] {
			rate = 55
		}
		s.Lamp[i] += (target - s.Lamp[i]) * (1 - math.Exp(-dt*rate))
		s.LampPeak[i] = math.Max(s.Lamp[i], s.LampPeak[i]-dt*.6)
	}
}
func (s *SceneState) ufoBeaming() bool { return s.UfoT > 2.5 && s.UfoT < UfoSeconds-2 }
func (s *SceneState) AtRest() bool {
	if s.Activity > 0 || s.jumpPhase != jumpGround {
		return false
	}
	motion := math.Abs(s.BodyLift) + math.Abs(s.BodyShift) + math.Abs(s.BodyTilt) + s.Crouch + math.Abs(s.HeadAngle) + s.HeadDrop + math.Abs(s.LegTap) + s.Breathe + s.SpotGlow + s.JumpLift + s.JumpSquash
	for i := range s.Lamp {
		motion += s.Lamp[i] + s.LampPeak[i]
	}
	return motion < .01
}
