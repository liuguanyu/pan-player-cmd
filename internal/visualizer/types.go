package visualizer

import (
	"math/rand"
	"time"
)

type IntensityLevel int

const (
	IntensityLow IntensityLevel = iota
	IntensityMid
	IntensityHigh
)

// AudioAnalysis uses normalized PCM magnitudes, not Android FFT byte scaling.
type AudioAnalysis struct {
	AverageVolume, BassEnergy, TrebleEnergy, Beat float64
	Peak                                          bool
	Intensity                                     IntensityLevel
	Spectrum                                      []float64
}

type ViewportSize struct{ Width, Height int }

const (
	DefaultViewportWidth  = 560
	DefaultViewportHeight = 252
	DefaultVisualizerRows = 10
	FFTWindow             = 1024
	AnalysisIntervalMs    = 23
)

type Outfit int

const (
	OutfitNone Outfit = iota
	OutfitCowboy
	OutfitTopHat
	OutfitRedCap
	OutfitSunglasses
	OutfitPunk
	OutfitBow
)

type Face int

const (
	FaceNormal Face = iota
	FaceBigEye
	FaceWink
	FaceCrossEyed
	FaceGrin
	FaceTongue
)

type Move int

const (
	MoveBob Move = iota
	MoveHop
	MoveLunge
	MoveCrouch
	MoveTap
)

type jumpPhase int

const (
	jumpGround jumpPhase = iota
	jumpPrep
	jumpAir
	jumpLand
)

// SceneState is owned exclusively by the render loop. Clock and all event timers
// are playback time; wall-clock time is only used to measure frame deltas.
// Ported from pan-music-car-player's DancingSheepVisualizer.
type SceneState struct {
	LastFrameTime                                                                time.Time
	Clock, Activity, Bass, Treble, Volume, Breathe                               float64
	BlinkTimer, Blink                                                            float64
	BeatPos, BeatConfidence, SecondsPerBeat                                      float64
	BodyLift, BodyShift, BodyTilt, Crouch, HeadAngle, HeadDrop, FeetLift, LegTap float64
	JumpLift, JumpSquash                                                         float64
	jumpPhase                                                                    jumpPhase
	jumpStart, jumpAirSeconds, jumpT                                             float64
	jumpAirBeats, barsUntilJump                                                  int
	Move                                                                         Move
	moveBeats                                                                    int
	lastBeat                                                                     int64
	Face                                                                         Face
	faceT, faceTimer                                                             float64
	Outfit                                                                       Outfit
	Peace                                                                        bool
	outfitTimer                                                                  float64
	Stars                                                                        [60][3]float64
	Clouds                                                                       [3][3]float64
	BlimpX, BlimpDir, blimpTimer                                                 float64
	Lamp, LampPeak                                                               [10]float64
	DiscoT, discoTimer, SpotGlow                                                 float64
	Spots                                                                        [14][2]float64
	CatT, catTimer, BirdsX, birdsTimer, BirdsY, UfoT, ufoTimer                   float64
	rng                                                                          *rand.Rand
	tracker                                                                      *BeatTracker
}
