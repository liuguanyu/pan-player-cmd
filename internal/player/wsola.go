package player

import (
	"io"
	"math"

	"github.com/gopxl/beep"
)

// 本文件实现一个流式、可动态变速、且【保持音高不变】的时间拉伸器（WSOLA 算法）。
//
// 为什么不用 beep.ResampleRatio：
//   ResampleRatio 通过改变采样率变速，速度×1.5 的同时音高也升高 1.5 倍。
//   人耳对音高变化极其敏感，因此 1.5x 会被感知为“快得离谱”。真正的“倍速”
//   （如网盘/QQ音乐）只压缩时间、保持音高不变，WSOLA 正是为此设计的算法。
//
// 原理（WSOLA = Waveform Similarity Overlap-Add）：
//   1. 输出按固定步长 hop（窗长的 1/2，Hann 窗 50% 重叠满足 COLA 恒等式
//      w(n)+w(n+L/2)=1，任意倍速下幅度保持不变、不削波）推进；
//   2. 输入主游标每跳推进 hop × speed；
//   3. 每跳在输入主游标 ±search 范围内，用归一化互相关挑选与“已定格的输出
//      历史尾部”最相似的窗起点，保证相邻窗衔接自然、无金属声；
//   4. speed>1 时输入消费速率高于输出产生速率 → 时间压缩；speed<1 反之 →
//      时间拉伸。窗内容取自输入原始波形，基频不变 → 音高保持不变。
//
// 流式支持：输入只要求是普通 beep.Streamer（顺序 Stream），本结构内部维护
// 输入历史缓冲（供基音估计与搜索回看）+ 输出历史缓冲，无需 Seek。
//
// 结束条件：输入耗尽后继续叠加窗直到缓冲排空（drained），最后一个窗的尾部
// 淡出后返回 (n, false)。级联时每一级都遵循同样语义，因此最末一级会在所有
// 级排空后才结束，不会截断或拖长尾部。
type wsolaStreamer struct {
	inner  beep.Streamer
	speed  float64
	winLen int
	hop    int
	hann   []float64

	// 输入历史缓冲（每声道一条）；index 0 对应绝对采样位置 oldest。
	oldest int
	bufL   []float64
	bufR   []float64
	next   int  // 已从 inner 读取到的下一个绝对位置
	total  int  // 输入总采样数；<=0 表示未知，以实际耗尽为准
	done   bool // inner 是否已耗尽
	drained bool // 输入历史缓冲是否已排空（drain 模式）

	inputPos int // 输入主游标（本跳理想起点，绝对坐标）
	outPos   int // 下一个窗的输出起始位置（绝对坐标）
	drainPos int // 下一个可发出的输出采样（绝对坐标）

	outBase  int
	outL     []float64
	outR     []float64
	finished bool // 已向外宣告结束

	period float64 // 估计的基音周期（采样）；0 表示尚未估计
}

const (
	wsolaWinLen     = 1056 // 窗长 ≈ 24ms @44.1kHz（对 ≤ ~120Hz 基频的人声足够长）
	wsolaCorrN      = 528  // 相关长度 ≈ 12ms
	wsolaPeriodLo   = 32   // 周期下限 ≈ 1.4kHz 基音
	wsolaPeriodHi   = 640  // 周期上限 ≈ 69Hz 基音
	wsolaInHistory  = 2048 // 输入历史保留量
	wsolaOutHistory = 4096 // 输出历史保留量
)

func newWSOLASpeed(inner beep.Streamer, total int, speed float64) *wsolaStreamer {
	hann := make([]float64, wsolaWinLen)
	for i := range hann {
		hann[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(wsolaWinLen)))
	}
	return &wsolaStreamer{
		inner:  inner,
		speed:  speed,
		winLen: wsolaWinLen,
		hop:    wsolaWinLen / 2,
		hann:   hann,
		total:  total,
		bufL:   make([]float64, 0, 1<<18),
		bufR:   make([]float64, 0, 1<<18),
		outL:   make([]float64, 0, 1<<16),
		outR:   make([]float64, 0, 1<<16),
	}
}

// SetSpeed 动态变速（不重建流器，对齐 beep.Resampler.SetRatio 的语义）。
func (s *wsolaStreamer) SetSpeed(speed float64) {
	if speed <= 0 {
		return
	}
	s.speed = speed
}

func (s *wsolaStreamer) Stream(samples [][2]float64) (int, bool) {
	written := 0
	for written < len(samples) {
		if s.finished {
			return written, false
		}
		if !s.drained && s.inputExhausted() {
			s.enterDrain()
		}

		// 放置下一个窗：
		// - 非 drain：正常放置；
		// - drain：主游标还在剩余数据范围内就继续放置（用已缓存的数据），
		//   越过数据末尾后停止——尾部由最后一个零填充窗自然淡出。
		dataEnd := s.next
		if s.total > 0 && s.total < dataEnd {
			dataEnd = s.total
		}
		if !s.drained || s.inputPos < dataEnd {
			s.placeWindow()
		}

		// 发出所有已“完整”的输出采样。
		// 正常模式下只能发到 outPos-hop：后面 [outPos-hop, outPos) 是刚写的窗，
		// 还没被下一个窗 OLA 叠加，不能发出。
		// drain/结束模式下，最后写的窗尾部不会再被覆盖，可全部发出。
		maxEmit := s.outPos - s.hop
		if s.drained {
			if s.outPos > 0 {
				maxEmit = s.outPos - s.hop + s.winLen - 1
			} else {
				maxEmit = s.drainPos - 1
			}
		}
		if maxEmit < s.drainPos {
			// 等待下一个窗（此时必然 !drained）
			continue
		}
		space := len(samples) - written
		limit := maxEmit
		if limit > s.drainPos+space-1 {
			limit = s.drainPos + space - 1
		}
		for o := s.drainPos; o <= limit; o++ {
			samples[written][0] = s.outL[o-s.outBase]
			samples[written][1] = s.outR[o-s.outBase]
			s.outL[o-s.outBase] = 0
			s.outR[o-s.outBase] = 0
			written++
		}
		s.drainPos = limit + 1

		if s.drained && s.drainPos > maxEmit {
			// 缓冲排空：宣告结束
			s.finished = true
			return written, false
		}
	}
	return written, true
}

func (s *wsolaStreamer) inputExhausted() bool {
	if s.done && s.inputPos >= s.next {
		// inner 已实际耗尽（total 可能是估算值，以实际读取为准）
		return true
	}
	if s.total > 0 {
		return s.inputPos >= s.total
	}
	return false
}

// enterDrain 进入 drain 模式：不再从 inner 读取，用剩余历史缓冲继续叠窗。
func (s *wsolaStreamer) enterDrain() {
	s.drained = true
	// 收紧 total 为实际读取量，避免尾部超出数据的窗读到垃圾
	if s.total < 0 || s.total > s.next {
		s.total = s.next
	}
}

// placeWindow 在 outPos 处叠加一个窗，并推进游标。
func (s *wsolaStreamer) placeWindow() {
	// 确保窗数据已读入缓冲（drain 模式下缓冲已固定，readUpTo 立即返回）
	s.readUpTo(s.inputPos + s.winLen + 16)

	dataEnd := s.next
	if s.total > 0 && s.total < dataEnd {
		dataEnd = s.total
	}

	// 定期用输入历史自相关估计基音周期
	if s.outPos > 0 && s.outPos%(8*s.hop) == 0 {
		if p, ok := s.estimatePeriod(); ok {
			s.period = p
		}
	}

	// 搜索半径：至少覆盖一个 hop 的位移 + 3 个基音周期，
	// 保证相位锁定候选在搜索范围内（对高音高同样成立）
	search := 3 * int(s.period)
	if s.period == 0 {
		search = s.hop
	}
	if search < s.hop {
		search = s.hop
	}

	// 用归一化互相关，在 [inputPos-search, inputPos+search] 内挑选与输出
	// 历史尾部最相似的窗起点
	lo := s.inputPos - search
	if lo < s.oldest {
		lo = s.oldest
	}
	hi := s.inputPos + search
	maxP := dataEnd - s.winLen
	if maxP < lo {
		maxP = lo
	}
	if hi > maxP {
		hi = maxP
	}
	bestP := s.inputPos
	if bestP > maxP {
		bestP = maxP
	}
	if hi >= lo {
		bestScore := math.Inf(-1)
		for p := lo; p <= hi; p += 16 {
			sc := s.correlate(p)
			if sc > bestScore {
				bestScore = sc
				bestP = p
			}
		}
	}

	// 重叠相加：输入 [bestP, bestP+winLen) × Hann → 输出 [outPos, outPos+winLen)
	for i := 0; i < s.winLen; i++ {
		inIdx := bestP + i
		var vL, vR float64
		if inIdx < dataEnd && inIdx-s.oldest >= 0 {
			vL = s.bufL[inIdx-s.oldest]
			vR = s.bufR[inIdx-s.oldest]
		}
		w := s.hann[i]
		o := s.outPos + i
		s.ensureOut(o + 1)
		s.outL[o-s.outBase] += vL * w
		s.outR[o-s.outBase] += vR * w
	}

	// 推进游标
	s.outPos += s.hop
	s.inputPos += int(float64(s.hop)*s.speed + 0.5)
	if s.inputPos < s.hop {
		s.inputPos = s.hop
	}

	// 输出缓冲定期压缩，避免无限增长
	if s.outPos-s.outBase > wsolaOutHistory+s.hop {
		s.compactOut()
	}
}

// correlate 计算输入 [p, p+corrN) 与放置区域 [outPos, outPos+corrN)
// 的归一化互相关（左声道）。放置区域当前保存的是上一个窗的尾部，
// 与之匹配可保证新窗头部与已叠加波形同相位 → OLA 平滑、音高稳定。
func (s *wsolaStreamer) correlate(p int) float64 {
	n := wsolaCorrN
	if n > s.hop {
		n = s.hop
	}
	if p < s.oldest || p+n > s.next {
		return math.Inf(-1) // 输入数据不足
	}
	// 参考段：放置位置 [outPos, outPos+n)（上一个窗的尾部）
	start := s.outPos
	if start < s.outBase {
		start = s.outBase
	}
	if start-s.outBase+n > len(s.outL) {
		return math.Inf(-1) // 缓冲不足（首个窗）
	}
	var num, denA, denB float64
	for i := 0; i < n; i++ {
		a := s.outL[start-s.outBase+i]
		b := s.bufL[p-s.oldest+i]
		num += a * b
		denA += a * a
		denB += b * b
	}
	if denA == 0 || denB == 0 {
		return 0 // 参考段全零（首个窗）或输入段全零
	}
	return num / math.Sqrt(denA*denB)
}

// estimatePeriod 用输入历史自相关估计基音周期（左声道）。
func (s *wsolaStreamer) estimatePeriod() (float64, bool) {
	end := s.inputPos
	start := end - wsolaInHistory
	if start < s.oldest {
		start = s.oldest
	}
	span := end - start
	if span < wsolaPeriodHi*3 {
		return 0, false
	}
	var bestLag int
	var bestScore float64
	for lag := wsolaPeriodHi; lag >= wsolaPeriodLo; lag-- {
		var num, den float64
		for i := 0; i+lag <= span; i++ {
			a := s.bufL[start-s.oldest+i]
			b := s.bufL[start-s.oldest+i+lag]
			num += a * b
			den += a*a + b*b
		}
		if den == 0 {
			continue
		}
		sc := num / (den * 0.5)
		if sc > bestScore {
			bestScore = sc
			bestLag = lag
		}
	}
	if bestLag == 0 || bestScore < 0.3 {
		return 0, false
	}
	return float64(bestLag), true
}

// readUpTo 从 inner 顺序读取，直到缓冲覆盖绝对位置 upto（或 inner 耗尽）。
func (s *wsolaStreamer) readUpTo(upto int) bool {
	if s.drained || s.done {
		return s.next >= upto
	}
	if s.next >= upto {
		return true
	}
	chunk := make([][2]float64, 8192)
	for s.next < upto {
		remain := upto - s.next
		want := len(chunk)
		if want > remain {
			want = remain
		}
		n, ok := s.inner.Stream(chunk[:want])
		if n <= 0 {
			s.done = true
			return false
		}
		for i := 0; i < n; i++ {
			s.bufL = append(s.bufL, chunk[i][0])
			s.bufR = append(s.bufR, chunk[i][1])
		}
		s.next += n
		if !ok {
			s.done = true
			return false
		}
	}
	// 压缩输入缓冲：只保留主游标前方一个窗 + 历史 wsolaInHistory
	keepOldest := s.inputPos - wsolaInHistory
	if keepOldest < s.oldest {
		keepOldest = s.oldest
	}
	if s.next-s.oldest > wsolaInHistory+s.winLen+2048 {
		if keepOldest > s.oldest {
			cut := keepOldest - s.oldest
			s.bufL = s.bufL[cut:]
			s.bufR = s.bufR[cut:]
			s.oldest = keepOldest
		}
	}
	return true
}

func (s *wsolaStreamer) ensureOut(absPos int) {
	if absPos-s.outBase >= len(s.outL) {
		g := absPos - s.outBase
		if g < 1<<16 {
			g = 1 << 16
		}
		nl := make([]float64, g+wsolaWinLen)
		copy(nl, s.outL)
		s.outL = nl
		nr := make([]float64, g+wsolaWinLen)
		copy(nr, s.outR)
		s.outR = nr
	}
}

func (s *wsolaStreamer) compactOut() {
	keep := s.drainPos
	if keep < s.outBase {
		keep = s.outBase
	}
	if keep < 0 {
		keep = 0
	}
	cut := keep - s.outBase
	if cut <= 0 {
		return
	}
	restL := make([]float64, len(s.outL)-cut)
	copy(restL, s.outL[cut:])
	restR := make([]float64, len(s.outR)-cut)
	copy(restR, s.outR[cut:])
	s.outL = restL
	s.outR = restR
	s.outBase = keep
}

func (s *wsolaStreamer) Err() error { return s.inner.Err() }
func (s *wsolaStreamer) Close() error {
	if c, ok := s.inner.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// --- 级联封装：支持任意倍速（单级 WSOLA 只能做到 2x，更高倍速拆成多级） ---

type cascadeStreamer struct {
	inner  beep.Streamer
	total  int
	speed  float64
	stages []*wsolaStreamer
	head   beep.Streamer // 末级（Stream 的入口）
	tail   beep.Streamer // 首级（直接包裹原始输入），用于 Close
}

// stageCount 返回将 speed 拆分为每级 ≤ 2x 所需的最少级数
func stageCount(speed float64) int {
	if speed <= 2 {
		return 1
	}
	n := 1
	f := speed
	for f > 2+1e-9 {
		f = f / 2
		n++
	}
	return n
}

func newCascadeSpeedStreamer(inner beep.Streamer, total int, speed float64) *cascadeStreamer {
	c := &cascadeStreamer{inner: inner, total: total, speed: speed}
	c.rebuild()
	return c
}

func (c *cascadeStreamer) rebuild() {
	n := stageCount(c.speed)
	f := math.Pow(c.speed, 1.0/float64(n))
	cur := c.inner
	tot := c.total
	c.stages = c.stages[:0]
	for i := 0; i < n; i++ {
		st := newWSOLASpeed(cur, tot, f)
		c.stages = append(c.stages, st)
		cur = beep.Streamer(st)
		tot = -1 // 中间级输出总长未知
	}
	c.head = beep.Streamer(c.stages[len(c.stages)-1])
	c.tail = beep.Streamer(c.stages[0])
}

// SetSpeed 动态变速。级数变化时重建链路（仅在跨 2/4/8 倍速边界时发生）。
func (c *cascadeStreamer) SetSpeed(speed float64) {
	if speed <= 0 {
		return
	}
	c.speed = speed
	if stageCount(speed) == len(c.stages) {
		f := math.Pow(speed, 1.0/float64(len(c.stages)))
		for _, st := range c.stages {
			st.SetSpeed(f)
		}
	} else {
		c.rebuild()
	}
}

func (c *cascadeStreamer) Stream(samples [][2]float64) (int, bool) { return c.head.Stream(samples) }
func (c *cascadeStreamer) Err() error                              { return c.head.Err() }
func (c *cascadeStreamer) Close() error {
	if c.tail == nil {
		return nil
	}
	if cl, ok := c.tail.(io.Closer); ok {
		return cl.Close()
	}
	return nil
}
