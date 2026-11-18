package visualizer

import "math"

// BeatTracker ports the car player's low-frequency spectral-flux detector and
// tempo/phase lock. Times are playback seconds, so pause never advances a beat.
// It is used only on the render goroutine.
type BeatTracker struct {
	previous                           []float64
	history                            [24]float64
	count, index                       int
	lastFlux, lastOnset                float64
	onsets                             []float64
	period, candidate                  float64
	votes                              int
	anchorTime, anchorBeat, confidence float64
	started                            bool
}

func NewBeatTracker() *BeatTracker {
	return &BeatTracker{period: .5, lastOnset: -1e9}
}
func (b *BeatTracker) position(t float64) float64 {
	return b.anchorBeat + (t-b.anchorTime)/b.period
}
func (b *BeatTracker) Confidence(t float64) float64 {
	return clamp(b.confidence-math.Max(0, t-b.lastOnset-4)/2, 0, 1)
}
func (b *BeatTracker) Feed(spectrum []float64, t float64) bool {
	if !b.started {
		b.started = true
		b.anchorTime = t
	}
	n := min(24, len(spectrum))
	flux := 0.0
	if len(b.previous) == n {
		for i := 1; i < n; i++ {
			w := 1.0
			if i < 8 {
				w = 1.5
			}
			flux += math.Max(0, math.Log1p(spectrum[i]*60)-math.Log1p(b.previous[i]*60)) * w
		}
	}
	b.previous = append(b.previous[:0], spectrum[:n]...)
	count := min(b.count, len(b.history))
	mean, variance := 0.0, 0.0
	for i := 0; i < count; i++ {
		mean += b.history[i]
	}
	if count > 0 {
		mean /= float64(count)
	}
	for i := 0; i < count; i++ {
		d := b.history[i] - mean
		variance += d * d
	}
	std := 0.0
	if count > 1 {
		std = math.Sqrt(variance / float64(count-1))
	}
	b.history[b.index] = flux
	b.index = (b.index + 1) % len(b.history)
	b.count++
	onset := count >= 6 && flux > mean+std*1.2 && flux > .6 && flux >= b.lastFlux && t-b.lastOnset >= .18
	b.lastFlux = flux
	if onset {
		b.registerOnset(t)
	}
	return onset
}
func (b *BeatTracker) score(p float64) float64 {
	score := 0.0
	for i, a := range b.onsets {
		for _, v := range b.onsets[i+1:] {
			d := v - a
			k := math.Round(d / p)
			if k < 1 || k > 4 {
				continue
			}
			err := (d - k*p) / (p * .08)
			score += math.Exp(-err*err) / k
		}
	}
	octave := math.Log2(.5 / p)
	return score * math.Exp(-octave*octave/.5)
}
func (b *BeatTracker) registerOnset(t float64) {
	b.lastOnset = t
	b.onsets = append(b.onsets, t)
	for len(b.onsets) > 64 || (len(b.onsets) > 0 && t-b.onsets[0] > 12) {
		b.onsets = b.onsets[1:]
	}
	if len(b.onsets) >= 4 {
		best, bestScore := b.period, 0.0
		for i := 0; i <= 140; i++ {
			p := .3 + float64(i)*.005
			score := b.score(p)
			if score > bestScore {
				bestScore = score
				best = p
			}
		}
		next := b.period
		if math.Abs(best-b.period)/b.period < .08 {
			next = b.period*.85 + best*.15
			b.votes = 0
		} else if b.confidence <= .25 || bestScore >= b.score(b.period)*1.6 {
			if b.candidate > 0 && math.Abs(best-b.candidate)/b.candidate < .08 {
				b.votes++
			} else {
				b.candidate = best
				b.votes = 1
			}
			if b.votes >= 6 || b.confidence <= .25 {
				next = best
				b.votes = 0
			}
		} else {
			b.votes = 0
		}
		b.anchorBeat = b.position(t)
		b.anchorTime = t
		b.period = next
	}
	pos := b.position(t)
	err := pos - math.RoundToEven(pos)
	if math.Abs(err) < .3 {
		gain := .25
		if b.confidence < .3 {
			gain = .6
		}
		b.anchorBeat = pos - err*gain
		b.anchorTime = t
		b.confidence = math.Min(1, b.confidence+.12*(1-math.Abs(err)*2))
	} else {
		b.confidence = math.Max(0, b.confidence-.06)
		if b.confidence < .15 {
			b.anchorBeat = math.RoundToEven(pos)
			b.anchorTime = t
		}
	}
}
