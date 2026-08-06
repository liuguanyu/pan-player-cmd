package visualizer

import (
	"math"
	"math/rand"
	"time"
)

const (
	// Cooldown periods to prevent effects from triggering too often
	goofyCooldown   = 5 * time.Second
	spinCooldown    = 8 * time.Second
	swaggerCooldown = 6 * time.Second
	farHopCooldown  = 4 * time.Second
	birdCooldown    = 3 * time.Second
	lampCooldown    = 10 * time.Second

	// Duration of timed effects
	goofyDuration   = 2 * time.Second
	spinDuration    = 1 * time.Second
	swaggerDuration = 2 * time.Second
	discoDuration   = 4 * time.Second
	microphoneDur   = 3 * time.Second
	spectrumLampDur = 5 * time.Second
)

// NewSceneState creates an initial scene state for the given viewport.
func NewSceneState(vp ViewportSize) *SceneState {
	now := time.Now()
	s := &SceneState{
		StartTime:     now,
		LastFrameTime: now,
		IsNight:       false,
		GoofyType:     0,
	}

	// Initialize far sheep
	s.FarSheep = []FarSheep{
		{X: float64(vp.Width) * 0.15, Y: float64(vp.Height) * 0.68, Scale: 0.4, Phase: 0},
		{X: float64(vp.Width) * 0.25, Y: float64(vp.Height) * 0.7, Scale: 0.3, Phase: 2},
		{X: float64(vp.Width) * 0.85, Y: float64(vp.Height) * 0.67, Scale: 0.35, Phase: 4},
	}

	return s
}

// Update advances the scene state by one frame using the latest audio analysis.
func (s *SceneState) Update(analysis AudioAnalysis) {
	now := time.Now()
	dt := now.Sub(s.LastFrameTime).Seconds()
	if dt <= 0 {
		dt = 1.0 / 60.0
	}
	s.LastFrameTime = now

	s.updateSheepCore(analysis, dt)
	s.updateTimedEffects(analysis, now)
	s.updateDecorations(dt, now)
	s.maybeTriggerEffects(analysis, now)
}

// updateSheepCore computes the primary sheep animation parameters.
func (s *SceneState) updateSheepCore(analysis AudioAnalysis, dt float64) {
	be := analysis.BassEnergy
	te := analysis.TrebleEnergy
	av := analysis.AverageVolume

	// Beat pulse — smooth 0-1 wave driven by bass
	targetPulse := be
	s.BeatPulse += (targetPulse - s.BeatPulse) * math.Min(dt*8, 1)

	// Sheep pulse (smoother)
	s.SheepPulse += (av - s.SheepPulse) * math.Min(dt*5, 1)

	// Leg phase — advances continuously, speed proportional to intensity.
	// Even quiet tracks should visibly dance, so keep a clear idle groove.
	speed := 4.0 + be*2.0 + av*1.5
	if analysis.Intensity == IntensityHigh {
		speed = 9.0
	} else if analysis.Intensity == IntensityMid {
		speed = 6.0
	}
	s.LegPhase += dt * speed * math.Pi * 2

	// Tongue — extends on strong bass
	tongueTarget := 0.0
	if be > 0.4 {
		tongueTarget = be
	}
	s.TongueValue += (tongueTarget - s.TongueValue) * math.Min(dt*12, 1)

	groove := math.Sin(s.LegPhase)
	sideGroove := math.Sin(s.LegPhase * 0.5)

	// Head nod — idle groove + audio-driven lift.
	headNodTarget := groove*3 + av*10 + be*3
	s.HeadNod += (headNodTarget - s.HeadNod) * math.Min(dt*10, 1)

	// Head swing — idle side wobble + treble response.
	swingTarget := sideGroove*5 + te*14
	s.HeadSwing += (swingTarget - s.HeadSwing) * math.Min(dt*8, 1)

	// Body bob — continuous bounce, with bass pushing it higher.
	bobTarget := groove*5 + be*18 + av*4
	s.BodyBob += (bobTarget - s.BodyBob) * math.Min(dt*9, 1)

	// Body stretch — compress on beat and breathe during idle groove.
	stretchTarget := s.BeatPulse*0.12 + math.Abs(groove)*0.04
	s.BodyStretch += (stretchTarget - s.BodyStretch) * math.Min(dt*12, 1)

	// Body tilt — slight lean driven by swagger
	tiltTarget := math.Sin(s.SwaggerPhase) * 0.05
	if s.Swagger.Active {
		tiltTarget = math.Sin(s.SwaggerPhase) * 0.1
	}
	s.BodyTilt += (tiltTarget - s.BodyTilt) * math.Min(dt*6, 1)

	// Eye wideness — opens on treble
	eyeTarget := te * 1.5
	s.EyeWide += (eyeTarget - s.EyeWide) * math.Min(dt*6, 1)

	// Eyelid — closes on beat
	lidTarget := s.BeatPulse * 1.5
	s.Eyelid += (lidTarget - s.Eyelid) * math.Min(dt*8, 1)

	// Spin angle — accumulates during spin effect
	if s.Spin.Active {
		s.SpinAngle += dt * 8
	} else {
		s.SpinAngle *= 0.9 // decay
	}

	// Swagger phase — advances during swagger
	if s.Swagger.Active {
		s.SwaggerPhase += dt * 12
	} else {
		s.SwaggerPhase += dt * 1.5
	}

	// Far sheep kick
	s.FarSheepKick += (be*0.5 - s.FarSheepKick) * math.Min(dt*4, 1)
}

// updateTimedEffects expires effects whose deadline has passed.
func (s *SceneState) updateTimedEffects(analysis AudioAnalysis, now time.Time) {
	s.Goofy.Active = s.Goofy.Active && now.Before(s.Goofy.Until)
	s.Spin.Active = s.Spin.Active && now.Before(s.Spin.Until)
	s.Swagger.Active = s.Swagger.Active && now.Before(s.Swagger.Until)
	s.Disco.Active = s.Disco.Active && now.Before(s.Disco.Until)
	s.Microphone.Active = s.Microphone.Active && now.Before(s.Microphone.Until)
	s.SpectrumLamp.Active = s.SpectrumLamp.Active && now.Before(s.SpectrumLamp.Until)

	// Disco persists while intensity is high
	if analysis.Intensity == IntensityHigh && !s.Disco.Active {
		if now.After(s.Disco.Until.Add(discoDuration)) {
			s.Disco.Active = true
			s.Disco.Until = now.Add(discoDuration)
		}
	}
}

// updateDecorations animates clouds, birds, and far sheep.
func (s *SceneState) updateDecorations(dt float64, now time.Time) {
	// Birds — drift rightwards
	for i := range s.Birds {
		s.Birds[i].X += s.Birds[i].Speed * dt
		s.Birds[i].WingPhase += dt * 8
	}
	// Remove off-screen birds
	alive := s.Birds[:0]
	for _, b := range s.Birds {
		if b.X < 700 {
			alive = append(alive, b)
		}
	}
	s.Birds = alive

	// Far sheep — gentle hop
	for i := range s.FarSheep {
		fs := &s.FarSheep[i]
		if now.After(fs.HopUntil) {
			fs.HopOffset = 0
		} else {
			elapsed := fs.HopUntil.Sub(now).Seconds()
			hopDuration := 0.3
			if elapsed < hopDuration {
				progress := 1.0 - elapsed/hopDuration
				fs.HopOffset = math.Sin(progress*math.Pi) * 8 * s.FarSheepKick
			}
		}
		fs.Phase += dt * 1.5
	}
}

// maybeTriggerEffects randomly activates goofy/spin/swagger etc. based on audio intensity.
func (s *SceneState) maybeTriggerEffects(analysis AudioAnalysis, now time.Time) {
	// Goofy face — triggered by peaks
	if analysis.Peak && now.After(s.LastGoofyTrigger.Add(goofyCooldown)) {
		s.Goofy.Active = true
		s.Goofy.Until = now.Add(goofyDuration)
		s.GoofyType = rand.Intn(3) // 0=wide-eyes, 1=squint, 2=puff-cheeks
		s.LastGoofyTrigger = now
	}

	// Spin — triggered by high-intensity peaks
	if analysis.Peak && analysis.Intensity == IntensityHigh && now.After(s.LastSpinTrigger.Add(spinCooldown)) {
		s.Spin.Active = true
		s.Spin.Until = now.Add(spinDuration)
		s.LastSpinTrigger = now
	}

	// Swagger — triggered by mid+ intensity
	if analysis.Intensity != IntensityLow && now.After(s.LastSwaggerTrigger.Add(swaggerCooldown)) && rand.Float64() < 0.3 {
		s.Swagger.Active = true
		s.Swagger.Until = now.Add(swaggerDuration)
		s.LastSwaggerTrigger = now
	}

	// Microphone — triggered by sustained high volume
	if analysis.AverageVolume > 0.6 && now.After(s.LastSwaggerTrigger.Add(microphoneDur)) && rand.Float64() < 0.2 {
		s.Microphone.Active = true
		s.Microphone.Until = now.Add(microphoneDur)
	}

	// Spectrum lamp — periodic
	if now.After(s.LastLampTrigger.Add(lampCooldown)) && rand.Float64() < 0.15 {
		s.SpectrumLamp.Active = true
		s.SpectrumLamp.Until = now.Add(spectrumLampDur)
		s.LastLampTrigger = now
	}

	// Bird spawn
	if now.After(s.LastBirdTrigger.Add(birdCooldown)) && rand.Float64() < 0.2 {
		s.Birds = append(s.Birds, Bird{
			X:         -20,
			Y:         float64(40 + rand.Intn(80)),
			Speed:     float64(80 + rand.Intn(40)),
			WingPhase: 0,
		})
		s.LastBirdTrigger = now
	}

	// Far sheep hop
	if analysis.BassEnergy > 0.5 && now.After(s.LastFarHopTrigger.Add(farHopCooldown)) {
		for i := range s.FarSheep {
			s.FarSheep[i].HopUntil = now.Add(300 * time.Millisecond)
		}
		s.LastFarHopTrigger = now
	}
}
