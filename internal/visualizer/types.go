package visualizer

import "time"

// --- Audio Analysis ---

// IntensityLevel classifies the overall audio loudness.
type IntensityLevel int

const (
	IntensityLow IntensityLevel = iota
	IntensityMid
	IntensityHigh
)

// AudioAnalysis holds the per-frame frequency-domain features extracted by the
// FFT analyzer. All values are in [0, 1] unless noted.
type AudioAnalysis struct {
	AverageVolume float64 // overall loudness (0–1)
	BassEnergy    float64 // low-frequency energy (0–1)
	TrebleEnergy  float64 // high-frequency energy (0–1)
	Beat          float64 // beat intensity (0–1, >0 when a beat is detected)
	Peak          bool    // true when a beat peak is detected this frame
	Intensity     IntensityLevel
}

// --- Timed Effect Helpers ---

// TimedBool is an effect flag that auto-expires after its Until time.
type TimedBool struct {
	Active bool
	Until  time.Time
}

// --- Scene State ---

// SceneState is the full animation state updated every frame.
type SceneState struct {
	StartTime     time.Time
	LastFrameTime time.Time

	// Visual mode
	IsNight  bool
	PinkMode bool

	// Sheep body
	BodyBob     float64
	BodyStretch float64
	BodyTilt    float64

	// Audio-driven pulses
	BeatPulse  float64
	SheepPulse float64

	// Leg
	LegPhase float64

	// Head
	HeadNod     float64
	HeadSwing   float64
	TongueValue float64

	// Eyes
	EyeWide float64
	Eyelid  float64

	// Spin effect
	Spin      TimedBool
	SpinAngle float64

	// Swagger effect
	Swagger      TimedBool
	SwaggerPhase float64

	// Goofy face
	Goofy     TimedBool
	GoofyType int

	// Performance effects
	Disco        TimedBool
	Microphone   TimedBool
	SpectrumLamp TimedBool

	// Decorations
	Birds        []Bird
	FarSheep     []FarSheep
	FarSheepKick float64

	// Cooldown timers
	LastGoofyTrigger   time.Time
	LastSpinTrigger    time.Time
	LastSwaggerTrigger time.Time
	LastLampTrigger    time.Time
	LastBirdTrigger    time.Time
	LastFarHopTrigger  time.Time
}

// ViewportSize is the logical pixel size of the rendering canvas.
type ViewportSize struct {
	Width  int
	Height int
}

const (
	// Logical pixel viewport. Sixel pixels are not terminal character rows;
	// keep the image moderately wide while reserving only the space it needs
	// in the text layout.
	DefaultViewportWidth  = 560
	DefaultViewportHeight = 252

	// Approximate terminal rows occupied by the image in iTerm2. This is a
	// layout reservation, not a conversion from sixel strips (6-pixel bands).
	DefaultVisualizerRows = 10

	// FFT window size (must be power of 2)
	FFTWindow = 1024

	// How often the visualizer produces a new analysis frame (~43 fps)
	AnalysisIntervalMs = 23
)

// Bird is a small bird decoration flying across the sky.
type Bird struct {
	X         float64
	Y         float64
	Speed     float64
	WingPhase float64
}

// FarSheep is a small sheep in the background.
type FarSheep struct {
	X         float64
	Y         float64
	Scale     float64
	Phase     float64
	HopOffset float64
	HopUntil  time.Time
}
