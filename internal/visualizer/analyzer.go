package visualizer

import (
	"math"
	"math/cmplx"
)

// realFFT computes the FFT of a real-valued slice (length must be power of 2).
// Returns only the first half of the spectrum (bins 0..n/2) since the rest
// is conjugate-symmetric for real inputs.
func realFFT(x []float64) []complex128 {
	n := len(x)
	c := make([]complex128, n)
	for i := 0; i < n; i++ {
		c[i] = complex(x[i], 0)
	}
	fft(c)
	return c[:n/2+1]
}

// fft performs an in-place Cooley-Tukey FFT on complex128 data.
// Length must be a power of 2.
func fft(a []complex128) {
	n := len(a)

	// Bit-reversal permutation
	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for j&bit != 0 {
			j ^= bit
			bit >>= 1
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}

	// Cooley-Tukey
	for length := 2; length <= n; length <<= 1 {
		half := length >> 1
		angle := -2.0 * math.Pi / float64(length)
		wlen := complex(math.Cos(angle), math.Sin(angle))
		for i := 0; i < n; i += length {
			w := complex(1.0, 0)
			for k := 0; k < half; k++ {
				u := a[i+k]
				v := a[i+k+half] * w
				a[i+k] = u + v
				a[i+k+half] = u - v
				w *= wlen
			}
		}
	}
}

// AudioAnalyzer reads samples from a ring buffer, performs FFT, and produces
// AudioAnalysis structs consumed by the animation state machine.
// The ring buffer is passed to Analyze() each call so the analyzer always
// uses the latest buffer (the buffer may be replaced when the audio source
// changes, e.g. on track change).
type AudioAnalyzer struct {
	window   []float64 // re-used FFT input buffer
	lastPeak float64
}

// NewAudioAnalyzer creates an analyzer.
func NewAudioAnalyzer() *AudioAnalyzer {
	return &AudioAnalyzer{
		window: make([]float64, FFTWindow),
	}
}

// Analyze reads the latest samples from the given ring buffer, computes
// frequency-domain features, and returns an AudioAnalysis result.
func (a *AudioAnalyzer) Analyze(buf *RingBuffer) AudioAnalysis {
	if buf == nil {
		return AudioAnalysis{Intensity: IntensityLow}
	}
	raw := buf.ReadAvailable(FFTWindow)
	if len(raw) < FFTWindow/2 {
		// Not enough data yet — return silent analysis
		return AudioAnalysis{Intensity: IntensityLow}
	}

	// Zero padding must not retain the preceding FFT window.
	clear(a.window)
	var power float64
	for i := 0; i < FFTWindow && i < len(raw); i++ {
		a.window[i] = (raw[i][0] + raw[i][1]) / 2.0
		power += (raw[i][0]*raw[i][0] + raw[i][1]*raw[i][1]) / 2
	}

	// Apply Hann window to reduce spectral leakage
	hannWindow(a.window[:min(FFTWindow, len(raw))])

	// FFT
	spectrum := realFFT(a.window)
	nBins := len(spectrum)

	// PCM is already in [-1,1]. Normalize Hann-window magnitudes before
	// compressing; the old Android-byte divisors made real PCM nearly silent.
	magnitudes := make([]float64, nBins)
	for i := range spectrum {
		magnitudes[i] = cmplx.Abs(spectrum[i]) * 4 / float64(FFTWindow)
	}
	avgVolume := clamp(math.Sqrt(power/float64(len(raw)))*2, 0, 1)

	// Bass energy: first 10% of bins
	bassBins := max(1, nBins/10)
	var bassSum float64
	for i := 0; i < bassBins; i++ {
		bassSum += magnitudes[i] * magnitudes[i]
	}
	bassEnergy := clamp(math.Sqrt(bassSum)*1.5, 0, 1)

	// Treble energy: bins from 70% to end
	trebleStart := nBins * 7 / 10
	var trebleSum float64
	trebleCount := 0
	for i := trebleStart; i < nBins; i++ {
		trebleSum += magnitudes[i] * magnitudes[i]
		trebleCount++
	}
	trebleEnergy := 0.0
	if trebleCount > 0 {
		trebleEnergy = clamp(math.Sqrt(trebleSum)*2, 0, 1)
	}

	// Beat detection: compare current bass to recent average
	beat := 0.0
	peak := false
	if bassEnergy > a.lastPeak*1.4 && bassEnergy > 0.3 {
		beat = clamp(bassEnergy, 0, 1)
		peak = true
	}
	a.lastPeak = a.lastPeak*0.9 + bassEnergy*0.1
	if a.lastPeak > bassEnergy {
		a.lastPeak = bassEnergy
	}

	// Intensity classification
	intensity := IntensityLow
	if avgVolume > 0.4 {
		intensity = IntensityMid
	}
	if avgVolume > 0.7 {
		intensity = IntensityHigh
	}

	return AudioAnalysis{
		AverageVolume: avgVolume,
		BassEnergy:    bassEnergy,
		TrebleEnergy:  trebleEnergy,
		Beat:          beat,
		Peak:          peak,
		Intensity:     intensity,
		Spectrum:      magnitudes,
	}
}

// hannWindow applies a Hann window to the slice in-place.
func hannWindow(x []float64) {
	n := len(x)
	for i := 0; i < n; i++ {
		multiplier := 0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(n-1)))
		x[i] *= multiplier
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
