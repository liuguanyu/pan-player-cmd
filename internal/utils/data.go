package utils

import (
	"os"
	"path/filepath"
)

// DataDir 返回应用数据目录
func DataDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "/tmp"
	}
	return filepath.Join(homeDir, ".pan-player")
}

// CacheDir 返回缓存目录
func CacheDir() string {
	return filepath.Join(DataDir(), "cache")
}

// AudioFeaturesDir 返回音频特征缓存目录
func AudioFeaturesDir() string {
	return filepath.Join(DataDir(), "audio_features")
}

// EnsureDataDir 确保数据目录存在
func EnsureDataDir() error {
	dataDir := DataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}
	return nil
}

// EnsureAudioFeaturesDir 确保音频特征缓存目录存在
func EnsureAudioFeaturesDir() error {
	audioFeaturesDir := AudioFeaturesDir()
	if err := os.MkdirAll(audioFeaturesDir, 0755); err != nil {
		return err
	}
	return nil
}