package analyzer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
	"github.com/liuguanyu/pan-player-cmd/internal/utils"
)

// AudioFeatureCache 音频特征缓存系统
type AudioFeatureCache struct {
	cacheDir string
}

// NewAudioFeatureCache 创建缓存系统
func NewAudioFeatureCache() *AudioFeatureCache {
	return &AudioFeatureCache{
		cacheDir: utils.AudioFeaturesDir(),
	}
}

// Load 从缓存加载音频特征
func (c *AudioFeatureCache) Load(fsID int64, md5 string) (*models.AudioFeatures, error) {
	// 使用 MD5 作为缓存文件名，更可靠
	filename := fmt.Sprintf("%s.audio.json", md5)
	path := filepath.Join(c.cacheDir, filename)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("cache not found")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var features models.AudioFeatures
	if err := json.Unmarshal(data, &features); err != nil {
		return nil, err
	}

	return &features, nil
}

// Save 保存音频特征到缓存
func (c *AudioFeatureCache) Save(features *models.AudioFeatures) error {
	if err := utils.EnsureAudioFeaturesDir(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(features, "", "  ")
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("%s.audio.json", features.MD5)
	path := filepath.Join(c.cacheDir, filename)
	return os.WriteFile(path, data, 0644)
}

// HasCache 检查是否有缓存
func (c *AudioFeatureCache) HasCache(md5 string) bool {
	filename := fmt.Sprintf("%s.audio.json", md5)
	path := filepath.Join(c.cacheDir, filename)
	_, err := os.Stat(path)
	return err == nil
}

// ListCache 列出所有缓存文件
func (c *AudioFeatureCache) ListCache() ([]string, error) {
	files, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return nil, err
	}

	var cacheFiles []string
	for _, file := range files {
		if file.Name() != "" && filepath.Ext(file.Name()) == ".audio.json" {
			cacheFiles = append(cacheFiles, file.Name())
		}
	}
	return cacheFiles, nil
}

// ClearCache 清空所有缓存
func (c *AudioFeatureCache) ClearCache() error {
	files, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".audio.json" {
			path := filepath.Join(c.cacheDir, file.Name())
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetCacheSize 获取缓存大小
func (c *AudioFeatureCache) GetCacheSize() (int64, error) {
	files, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return 0, err
	}

	var size int64
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".audio.json" {
			path := filepath.Join(c.cacheDir, file.Name())
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			size += info.Size()
		}
	}
	return size, nil
}
