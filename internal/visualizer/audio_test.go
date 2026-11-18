package visualizer

import (
	"math"
	"testing"
)

func TestRingBufferWrapAndLatestSamples(t *testing.T) {
	rb := NewRingBuffer(4)
	rb.Write([][2]float64{{1, 1}, {2, 2}, {3, 3}, {4, 4}})
	if rb.Available() != 4 {
		t.Fatal("full buffer reported empty")
	}
	rb.Write([][2]float64{{5, 5}, {6, 6}})
	got := rb.ReadAvailable(4)
	for i, v := range got {
		if v[0] != float64(i+3) {
			t.Fatalf("wrong wrap order: %v", got)
		}
	}
	rb.Write([][2]float64{{7, 7}, {8, 8}, {9, 9}, {10, 10}, {11, 11}})
	got = rb.ReadAvailable(4)
	for i, v := range got {
		if v[0] != float64(i+8) {
			t.Fatalf("oversized write kept oldest rather than newest: %v", got)
		}
	}
}
func TestAnalyzerPCMNormalization(t *testing.T) {
	rb := NewRingBuffer(FFTWindow * 4)
	samples := make([][2]float64, FFTWindow)
	for i := range samples {
		v := .5 * math.Sin(2*math.Pi*4*float64(i)/FFTWindow)
		samples[i] = [2]float64{v, v}
	}
	rb.Write(samples)
	a := NewAudioAnalyzer()
	got := a.Analyze(rb)
	if got.AverageVolume < .6 || got.BassEnergy < .5 || len(got.Spectrum) != 513 {
		t.Fatalf("PCM was under-scaled: %+v", got)
	}
	if got.TrebleEnergy > .01 {
		t.Fatal("bass leaked into treble")
	}
	// A short FFT frame following a loud frame must not reuse old tail samples.
	short := NewRingBuffer(FFTWindow)
	short.Write(make([][2]float64, FFTWindow/2))
	got = a.Analyze(short)
	if got.BassEnergy > 0 || got.AverageVolume > 0 {
		t.Fatal("FFT retained stale tail")
	}
}
func TestBeatTrackerTempoAndLoss(t *testing.T) {
	for _, period := range []float64{.43, .5, .7} {
		b := NewBeatTracker()
		for i := 0; i < 30; i++ {
			b.registerOnset(float64(i) * period)
		}
		if math.Abs(b.period-period) > .015 {
			t.Fatalf("period %v estimated as %v", period, b.period)
		}
		now := 29 * period
		if b.Confidence(now) < .5 {
			t.Fatalf("failed to lock: %v", b.Confidence(now))
		}
		if b.Confidence(now+7) != 0 {
			t.Fatal("confidence retained without onsets")
		}
		if math.Abs((b.position(now+1)-b.position(now))-1/b.period) > 1e-8 {
			t.Fatal("beat position not continuous")
		}
	}
}
func TestBeatTrackerSpectralFlux(t *testing.T) {
	b := NewBeatTracker()
	onsets := 0
	for i := 0; i < 800; i++ {
		spectrum := make([]float64, 32)
		if i%25 == 0 {
			for j := 1; j < 8; j++ {
				spectrum[j] = .4
			}
		}
		if b.Feed(spectrum, float64(i)*.02) {
			onsets++
		}
	}
	if onsets < 20 || b.Confidence(15.98) < .5 || math.Abs(b.period-.5) > .02 {
		t.Fatalf("onsets=%d period=%v confidence=%v", onsets, b.period, b.Confidence(15.98))
	}
}
