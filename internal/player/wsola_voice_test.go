package player

import (
	"math"
	"testing"

	"github.com/gopxl/beep"
)

// voiceSource 生成类人声信号：f0 及其泛音 + 慢幅度调制（模拟音节），
// 这是真实音频的典型形态，也给了互相关足够的“抓手”来锁定相位。
type voiceSource struct {
	sr       beep.SampleRate
	f0       float64
	harms    []float64 // 各次谐波幅度
	modHz    float64
	total    int
	consumed int
}

func (v *voiceSource) sample(n int) float64 {
	mod := 0.6 + 0.4*math.Sin(2*math.Pi*v.modHz*float64(n)/float64(v.sr)) // 0.2..1.0
	var s float64
	for k, a := range v.harms {
		s += a * math.Sin(2*math.Pi*v.f0*float64(k+1)*float64(n)/float64(v.sr))
	}
	return 0.4 * mod * s
}

func (v *voiceSource) Stream(samples [][2]float64) (int, bool) {
	for i := range samples {
		if v.consumed >= v.total {
			return i, false
		}
		s := v.sample(v.consumed)
		samples[i][0] = s
		samples[i][1] = s
		v.consumed++
	}
	return len(samples), true
}
func (v *voiceSource) Err() error     { return nil }
func (v *voiceSource) Close() error   { return nil }
func (v *voiceSource) Len() int       { return v.total }
func (v *voiceSource) Position() int  { return v.consumed }
func (v *voiceSource) Seek(int) error { return nil }

// TestWSOLAVoicePitch 用类人声信号验证音高保持：f0=200Hz 经 1.5x/2x 后基频应仍≈200Hz。
func TestWSOLAVoicePitch(t *testing.T) {
	const sr = 44100
	f0 := 200.0
	for _, speed := range []float64{1.5, 2.0} {
		src := &voiceSource{
			sr:      beep.SampleRate(sr),
			f0:      f0,
			harms:   []float64{1.0, 0.5, 0.33, 0.25, 0.2},
			modHz:   5,
			total:   4 * sr,
		}
		cascade := newCascadeSpeedStreamer(beep.Streamer(src), src.total, speed)
		out := pump(t, cascade, 1<<20)
		l := flat(out)
		// 基频用自相关（对谐波丰富的信号可靠）
		peak, clarity := autocorrFreq(l, sr, f0*0.8, f0*1.4)
		t.Logf("speed=%.2f 输出 %.3fs, 基频 %.1fHz (clarity %.2f)，期望 ≈%.0fHz",
			speed, float64(len(l))/sr, peak, clarity, f0)
		if peak < f0*0.9 || peak > f0*1.1 {
			t.Errorf("音高未保持: 实测基频 %.1fHz (期望 %f±10%%)", peak, f0)
		}
	}
}
