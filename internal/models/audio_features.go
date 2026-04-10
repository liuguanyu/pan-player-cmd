package models

import "time"

// AudioFeatures 音频特征信息
type AudioFeatures struct {
	FsID           int64     `json:"fs_id"`
	Filename       string    `json:"filename"`
	MD5            string    `json:"md5"`
	Duration       float64   `json:"duration"`
	AnalyzedAt     time.Time `json:"analyzed_at"`

	// 整体特征
	HasVocal        bool    `json:"has_vocal"`        // 是否有人声
	Gender          string  `json:"gender"`           // male/female/neutral
	DominantInstr   string  `json:"dominant_instrument"` // 主要乐器
	HarmonyLevel    float64 `json:"harmony_level"`    // 和声复杂度 0-1
	EnergyLevel     float64 `json:"energy_level"`     // 能量强度 0-1
	BPM             float64 `json:"bpm"`              // 每分钟节拍数

	// 时间轴特征（类似 LRC）
	Sections        []AudioSection `json:"sections,omitempty"`
}

// AudioSection 音频段落
type AudioSection struct {
	Start       float64 `json:"start"`        // 开始时间（秒）
	End         float64 `json:"end"`          // 结束时间（秒）
	Type        string  `json:"type"`         // intro/verse/chorus/bridge/outro
	HasVocal    bool    `json:"has_vocal"`
	Gender      string  `json:"gender,omitempty"`
	Instrument  string  `json:"instrument"`
	Harmony     float64 `json:"harmony"`
	Energy      float64 `json:"energy"`
}

// RealtimeFeatures 实时特征（用于 TUI 显示）
type RealtimeFeatures struct {
	HasVocal       bool
	Gender         string
	DominantInstr  string
	HarmonyLevel   float64
	EnergyLevel    float64
	CurrentSection string // 当前段落类型
	Timestamp      float64
}

// Gender 常量
const (
	GenderMale    = "male"
	GenderFemale  = "female"
	GenderNeutral = "neutral"
)

// SectionType 常量
const (
	SectionIntro   = "intro"
	SectionVerse   = "verse"
	SectionChorus  = "chorus"
	SectionBridge  = "bridge"
	SectionOutro   = "outro"
)
