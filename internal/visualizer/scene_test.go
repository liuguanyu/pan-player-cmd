package visualizer

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestNightCycle(t *testing.T) {
	for _, v := range []struct{ clock, want float64 }{{0, 0}, {149, 0}, {157.5, .5}, {165, 1}, {240, 1}, {315, 1}, {322.5, .5}, {330, 0}, {660, 0}} {
		if got := NightFactor(v.clock); math.Abs(got-v.want) > 1e-9 {
			t.Fatalf("night(%v)=%v", v.clock, got)
		}
	}
	prev := NightFactor(0)
	for clock := 1.0 / 60; clock < CycleSeconds*2; clock += 1.0 / 60 {
		n := NightFactor(clock)
		if math.Abs(n-prev) > .005 {
			t.Fatal("abrupt day/night transition")
		}
		prev = n
	}
}
func TestPauseSettlesAndPreservesClock(t *testing.T) {
	s := NewSceneState(ViewportSize{})
	a := AudioAnalysis{AverageVolume: .7, BassEnergy: .8, Spectrum: make([]float64, 128)}
	for i := range a.Spectrum {
		a.Spectrum[i] = .3
	}
	for i := 0; i < 600; i++ {
		s.Advance(a, 1.0/60, true)
	}
	s.jumpPhase = jumpAir
	s.jumpAirSeconds = 1
	s.jumpT = .2
	for i := 0; i < 600; i++ {
		s.Advance(a, 1.0/60, false)
	}
	if !s.AtRest() || s.JumpLift != 0 {
		t.Fatal("paused sheep did not settle")
	}
	clock := s.Clock
	for i := 0; i < 600; i++ {
		s.Advance(a, 1.0/60, false)
	}
	if s.Clock != clock {
		t.Fatal("resting clock advanced")
	}
	for _, v := range s.Lamp {
		if v > .01 {
			t.Fatal("equalizer did not settle")
		}
	}
	s.Advance(a, 1.0/60, true)
	if s.Clock <= clock {
		t.Fatal("resume did not advance")
	}
}
func TestMovesAndBarBoundaries(t *testing.T) {
	for move := MoveBob; move <= MoveTap; move++ {
		s := NewSceneState(ViewportSize{})
		s.Move = move
		s.moveBeats = 100000
		s.Activity = 1
		s.Bass = .8
		s.BeatConfidence = 1
		maxPose := 0.0
		for i := 0; i < 240; i++ {
			s.BeatPos = float64(i) / 30
			s.advanceDance(1.0 / 60)
			maxPose = math.Max(maxPose, math.Max(math.Abs(s.HeadAngle), math.Max(math.Abs(s.BodyShift), s.Crouch*30)))
		}
		if move != MoveHop && maxPose < 15 {
			t.Fatalf("move %v too subtle: %v", move, maxPose)
		}
	}
	s := NewSceneState(ViewportSize{})
	s.Activity = 1
	changes := 0
	for i := 0; i < 3600; i++ {
		s.BeatPos = float64(i) / 30
		old := s.Move
		s.advanceDance(1.0 / 60)
		if old != s.Move {
			changes++
			if s.lastBeat%4 != 0 {
				t.Fatal("move changed outside bar boundary")
			}
		}
	}
	if changes < 3 {
		t.Fatal("moves did not rotate")
	}
}
func TestJumpProjectileAndSquash(t *testing.T) {
	for _, period := range []float64{.46, .5, .7, 1} {
		s := NewSceneState(ViewportSize{})
		s.Activity = 1
		s.SecondsPerBeat = period
		s.lastBeat = 3
		s.scheduleJump(true)
		var flight []float64
		before, after, apex := 0.0, 0.0, 0.0
		for i := 0; i < 420; i++ {
			s.BeatPos = 3 + float64(i)/120/period
			phase := s.jumpPhase
			s.updateJump(1.0 / 120)
			if phase == jumpPrep {
				before = math.Max(before, s.JumpSquash)
			}
			if s.jumpPhase == jumpLand {
				after = math.Max(after, s.JumpSquash)
			}
			if s.JumpLift > 0 {
				flight = append(flight, s.JumpLift)
				apex = math.Max(apex, s.JumpLift)
			}
		}
		if len(flight) < 3 || before < .4 || after < .4 || apex > MaxJump+1 {
			t.Fatalf("period %v: flight=%d prep=%v land=%v apex=%v", period, len(flight), before, after, apex)
		}
		for i := 1; i+1 < len(flight); i++ {
			acc := (flight[i+1] - 2*flight[i] + flight[i-1]) * 120 * 120
			if math.Abs(acc+Gravity) > 1e-5 {
				t.Fatalf("not constant gravity: %v", acc)
			}
		}
	}
}
func TestEqualizerHasIndependentBands(t *testing.T) {
	s := NewSceneState(ViewportSize{})
	s.Activity = 1
	spectrum := make([]float64, 513)
	spectrum[2] = .6
	for i := 0; i < 120; i++ {
		s.updateEqualizer(spectrum, 1.0/60)
	}
	if s.Lamp[1] < .5 || s.Lamp[9] != 0 {
		t.Fatalf("bands do not represent spectrum: %v", s.Lamp)
	}
}
func TestSceneEvents(t *testing.T) {
	s := NewSceneState(ViewportSize{})
	s.Activity = 1
	a := AudioAnalysis{}
	cat, blimp := false, false
	for i := 0; i < 1600; i++ {
		s.Advance(a, .05, true)
		cat = cat || s.CatT >= 0
		blimp = blimp || s.BlimpX >= 0
	}
	if !cat || !blimp {
		t.Fatal("day events did not appear")
	}
	s.Clock = 200
	s.discoTimer = 0
	s.ufoTimer = 0
	s.DiscoT = -1
	s.UfoT = -1
	s.Advance(a, .02, true)
	if s.DiscoT < 0 || s.UfoT >= 0 {
		t.Fatal("night event gating failed")
	}
	s.DiscoT = -1
	s.discoTimer = 100
	s.Advance(a, .02, true)
	if s.UfoT < 0 {
		t.Fatal("UFO never appeared")
	}
}

func TestRenderScenes(t *testing.T) {
	// Set SHEEP_PREVIEW_DIR to retain PNGs for visual inspection; normal tests
	// also render every outfit/face/event to catch clipping and invalid geometry.
	dir := os.Getenv("SHEEP_PREVIEW_DIR")
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	cases := []string{"day", "night", "disco", "ufo", "cat", "blimp-left", "blimp-right"}
	for o := OutfitNone; o <= OutfitBow; o++ {
		cases = append(cases, fmt.Sprintf("outfit-%d", o))
	}
	for f := FaceNormal; f <= FaceTongue; f++ {
		cases = append(cases, fmt.Sprintf("face-%d", f))
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			s := NewSceneState(ViewportSize{})
			s.Activity = 1
			s.Clock = 30
			switch name {
			case "night":
				s.Clock = 220
			case "disco":
				s.Clock = 220
				s.DiscoT = 10
				s.SpotGlow = .9
			case "ufo":
				s.Clock = 220
				s.UfoT = 4
			case "cat":
				s.CatT = 25
			case "blimp-left", "blimp-right":
				s.BlimpX = .3
				if name == "blimp-right" {
					s.BlimpDir = 1
				}
				for i := range s.Lamp {
					s.Lamp[i] = float64(i+1) / 10
					s.LampPeak[i] = 1
				}
			}
			for o := OutfitNone; o <= OutfitBow; o++ {
				if name == fmt.Sprintf("outfit-%d", o) {
					s.Outfit = o
					s.Peace = o == OutfitSunglasses
				}
			}
			for f := FaceNormal; f <= FaceTongue; f++ {
				if name == fmt.Sprintf("face-%d", f) {
					s.Face = f
					s.faceT = 2
				}
			}
			detail, dst := NewCanvas(1120, 504), NewCanvas(560, 252)
			DrawBackground(detail, s)
			DrawMainSheep(detail, s, AudioAnalysis{})
			downsample2(detail, dst)
			if dir != "" {
				file, err := os.Create(filepath.Join(dir, name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, dst.Image())
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
func BenchmarkSceneRender(b *testing.B) {
	s := NewSceneState(ViewportSize{})
	s.Clock = 220
	s.DiscoT = 10
	s.SpotGlow = .9
	detail, dst := NewCanvas(1120, 504), NewCanvas(560, 252)
	for i := 0; i < b.N; i++ {
		DrawBackground(detail, s)
		DrawMainSheep(detail, s, AudioAnalysis{})
		downsample2(detail, dst)
		SixelEncode(dst.Image())
	}
}
