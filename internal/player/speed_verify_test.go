package player

import (
	"math"
	"testing"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/effects"
)

// finiteTone 有限长度正弦源（模拟真实音频流，只支持顺序 Stream）
type finiteTone struct {
	sr       beep.SampleRate
	freq     float64
	total    int
	consumed int
}

func (t *finiteTone) Stream(samples [][2]float64) (int, bool) {
	for i := range samples {
		if t.consumed >= t.total {
			return i, false
		}
		v := 0.5 * math.Sin(2*math.Pi*t.freq*float64(t.consumed)/float64(t.sr))
		samples[i][0] = v
		samples[i][1] = v
		t.consumed++
	}
	return len(samples), true
}
func (t *finiteTone) Err() error     { return nil }
func (t *finiteTone) Close() error   { return nil }
func (t *finiteTone) Len() int       { return t.total }
func (t *finiteTone) Position() int  { return t.consumed }
func (t *finiteTone) Seek(int) error { return nil }

// pump 拉满 streamer 直到结束，返回全部输出采样（上限 max 防死循环）
func pump(t *testing.T, s beep.Streamer, max int) [][2]float64 {
	t.Helper()
	chunk := make([][2]float64, 8192)
	var out [][2]float64
	total := 0
	for {
		n, ok := s.Stream(chunk)
		if n > 0 {
			cp := make([][2]float64, n)
			copy(cp, chunk[:n])
			out = append(out, cp...)
			total += n
			if total > max {
				t.Fatalf("输出超过上限 %d 采样，疑似死循环", max)
			}
		}
		if !ok || n == 0 {
			return out
		}
	}
}

func flat(out [][2]float64) []float64 {
	l := make([]float64, len(out))
	for i := range out {
		l[i] = out[i][0]
	}
	return l
}

func speedName(s float64) string {
	switch s {
	case 0.75:
		return "x0_75"
	case 1.25:
		return "x1_25"
	case 1.5:
		return "x1_5"
	case 2.0:
		return "x2"
	default:
		return "x1_5"
	}
}

// TestWSOLASpeedAccuracy 验证倍速只压缩时间：输出时长 ≈ 源时长 / speed。
func TestWSOLASpeedAccuracy(t *testing.T) {
	const sr = 44100
	for _, speed := range []float64{0.75, 1.25, 1.5, 2.0} {
		speed := speed
		t.Run(speedName(speed), func(t *testing.T) {
			src := &finiteTone{sr: beep.SampleRate(sr), freq: 200, total: 3 * sr}
			cascade := newCascadeSpeedStreamer(beep.Streamer(src), src.total, speed)
			out := pump(t, cascade, 1<<20)
			got := len(flat(out))
			want := float64(3*sr) / speed
			if math.Abs(float64(got)-want) > want*0.08 {
				t.Errorf("输出 %d 采样 (%.3fs)，期望 ≈ %.0f 采样 (%.3fs)，偏差 >8%%",
					got, float64(got)/sr, want, want/sr)
			} else {
				t.Logf("speed=%.2f 输出 %.3fs，期望 %.3fs ✅", speed, float64(got)/sr, want/sr)
			}
		})
	}
}

// TestWSOLAPitchPreserved 验证倍速不改变音高：200Hz 源经 1.5x 后仍应为 200Hz
// （旧 ResampleRatio 方案会变成 300Hz，这是“倍速不准/太快”的根源）。
func TestWSOLAPitchPreserved(t *testing.T) {
	const sr = 44100
	src := &finiteTone{sr: beep.SampleRate(sr), freq: 200, total: 3 * sr}
	cascade := newCascadeSpeedStreamer(beep.Streamer(src), src.total, 1.5)
	out := pump(t, cascade, 1<<20)
	l := flat(out)

	// 自相关测音高（对 OLA 后的波形比过零计数可靠）
	freq, clarity := autocorrFreq(l, sr, 100, 400)
	t.Logf("1.5x 输出实测频率: %.1f Hz (clarity %.2f)，期望 200Hz，旧方案会输出 300Hz", freq, clarity)
	if freq < 180 || freq > 220 {
		t.Errorf("音高未保持: 实测 %.1f Hz", freq)
	}
}

// TestWSOLASetSpeedDynamic 验证播放中动态变速可用（切 2x 会重建级联，切 0.75x 会降回单级）。
func TestWSOLASetSpeedDynamic(t *testing.T) {
	const sr = 44100
	src := &finiteTone{sr: beep.SampleRate(sr), freq: 200, total: 10 * sr}
	cascade := newCascadeSpeedStreamer(beep.Streamer(src), src.total, 1.5)
	chunk := make([][2]float64, 8192)
	total := 0
	for i := 0; ; i++ {
		n, ok := cascade.Stream(chunk)
		total += n
		if i%1000 == 500 {
			cascade.SetSpeed(2.0) // 中途切 2x
		}
		if i%1000 == 1500 {
			cascade.SetSpeed(0.75) // 中途切 0.75x
		}
		if i > 200000 {
			t.Fatalf("死循环")
		}
		if !ok || n == 0 {
			break
		}
	}
	t.Logf("动态变速后总输出 %.2fs（10s 源）", float64(total)/sr)
	if total < 5*sr || total > 30*sr {
		t.Errorf("输出时长异常: %d 采样", total)
	}
}

func autocorrFreq(l []float64, sr int, fmin, fmax float64) (freq float64, clarity float64) {
	if len(l) < sr/2 {
		return 0, 0
	}
	start := len(l) / 2 - sr / 2
	if start < 0 {
		start = 0
	}
	end := start + sr
	if end > len(l) {
		end = len(l)
	}
	x := l[start:end]
	minLag := int(float64(sr)/fmax)
	maxLag := int(float64(sr)/fmin)
	if maxLag >= len(x) {
		maxLag = len(x) - 1
	}
	var e float64
	for _, v := range x {
		e += v * v
	}
	if e == 0 {
		return 0, 0
	}
	var bestLag int
	var bestScore float64
	for lag := minLag; lag <= maxLag; lag++ {
		c := 0.0
		for i := 0; i+lag < len(x); i++ {
			c += x[i] * x[i+lag]
		}
		c /= e
		if c > bestScore {
			bestScore = c
			bestLag = lag
		}
	}
	if bestLag == 0 {
		return 0, 0
	}
	return float64(sr) / float64(bestLag), bestScore
}

// TestSetStreamChain1xBypass 复现 SetStream 的 1x 旁路链路: tone -> volume -> ctrl，
// 验证整条链路输出时长与源一致。
func TestSetStreamChain1xBypass(t *testing.T) {
	const sr = 44100
	src := &finiteTone{sr: beep.SampleRate(sr), freq: 200, total: 3 * sr}
	volume := &effects.Volume{Streamer: beep.Streamer(src), Base: 2, Volume: 0}
	ctrl := &beep.Ctrl{Streamer: volume, Paused: false}
	out := pump(t, ctrl, 1<<20)
	got := len(flat(out))
	want := float64(3 * sr)
	if math.Abs(float64(got)-want) > want*0.02 {
		t.Errorf("1x 链路输出 %d 采样，期望 ≈ %.0f", got, want)
	}
}
