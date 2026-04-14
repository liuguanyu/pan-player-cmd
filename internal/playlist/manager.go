package playlist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/liuguanyu/pan-player-cmd/internal/api"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
)

// Manager 播放列表管理器
type Manager struct {
	playlists    []models.Playlist
	playlistsDir string
	mu           sync.RWMutex
}

// NewManager 创建播放列表管理器
func NewManager(dataDir string) *Manager {
	return &Manager{
		playlistsDir: filepath.Join(dataDir, "playlists"),
	}
}

// LoadPlaylists 加载播放列表
func (m *Manager) LoadPlaylists() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 确保目录存在
	if err := os.MkdirAll(m.playlistsDir, 0755); err != nil {
		return err
	}

	// 读取所有播放列表文件
	entries, err := os.ReadDir(m.playlistsDir)
	if err != nil {
		return err
	}

	m.playlists = nil

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(m.playlistsDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var playlist models.Playlist
		if err := json.Unmarshal(data, &playlist); err != nil {
			continue
		}

		m.playlists = append(m.playlists, playlist)
	}

	// 确保有"最近播放"列表
	m.ensureRecentPlaylist()

	// 排序播放列表："最近播放"固定在第一位，其他按创建时间倒排
	m.sortPlaylists()

	return nil
}

// SavePlaylists 保存播放列表
func (m *Manager) SavePlaylists() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.savePlaylistsWithoutLock()
}

// savePlaylistsWithoutLock 保存播放列表（不获取锁，内部使用）
func (m *Manager) savePlaylistsWithoutLock() error {
	// 确保目录存在
	if err := os.MkdirAll(m.playlistsDir, 0755); err != nil {
		return err
	}

	// 保存每个播放列表
	for _, playlist := range m.playlists {
		filePath := filepath.Join(m.playlistsDir, playlist.Name+".json")
		data, err := json.MarshalIndent(playlist, "", "  ")
		if err != nil {
			continue
		}

		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return err
		}
	}

	return nil
}

// ensureRecentPlaylist 确保"最近播放"列表存在
func (m *Manager) ensureRecentPlaylist() {
	for _, pl := range m.playlists {
		if pl.Name == "最近播放" {
			return
		}
	}

	m.playlists = append([]models.Playlist{{
		Name:        "最近播放",
		Description: "最近播放的歌曲",
		Items:       nil,
		CreateTime:  time.Now().Unix(),
		UpdateTime:  time.Now().Unix(),
	}}, m.playlists...)
}

// GetPlaylists 获取所有播放列表
func (m *Manager) GetPlaylists() []models.Playlist {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]models.Playlist, len(m.playlists))
	copy(result, m.playlists)
	return result
}

// GetPlaylist 获取指定播放列表
func (m *Manager) GetPlaylist(name string) *models.Playlist {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i := range m.playlists {
		if m.playlists[i].Name == name {
			return &m.playlists[i]
		}
	}
	return nil
}

// CreatePlaylist 创建播放列表
func (m *Manager) CreatePlaylist(name string, description string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已存在
	for _, pl := range m.playlists {
		if pl.Name == name {
			return nil // 已存在，不重复创建
		}
	}

	now := time.Now().Unix()
	playlist := models.Playlist{
		Name:        name,
		Description: description,
		Items:       nil,
		CreateTime:  now,
		UpdateTime:  now,
	}

	m.playlists = append(m.playlists, playlist)
	return m.savePlaylistsWithoutLock()
}

// RemovePlaylist 删除播放列表
func (m *Manager) RemovePlaylist(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 不能删除"最近播放"
	if name == "最近播放" {
		return nil
	}

	for i, pl := range m.playlists {
		if pl.Name == name {
			m.playlists = append(m.playlists[:i], m.playlists[i+1:]...)

			// 删除文件
			filePath := filepath.Join(m.playlistsDir, name+".json")
			os.Remove(filePath)

			return m.savePlaylistsWithoutLock()
		}
	}

	return nil
}

// RenamePlaylist 重命名播放列表
func (m *Manager) RenamePlaylist(oldName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 不能重命名"最近播放"
	if oldName == "最近播放" {
		return nil
	}

	// 检查新名称是否已存在
	for _, pl := range m.playlists {
		if pl.Name == newName {
			return nil
		}
	}

	// 重命名
	for i := range m.playlists {
		if m.playlists[i].Name == oldName {
			m.playlists[i].Name = newName
			m.playlists[i].UpdateTime = time.Now().Unix()

			// 删除旧文件
			oldFile := filepath.Join(m.playlistsDir, oldName+".json")
			os.Remove(oldFile)

			return m.savePlaylistsWithoutLock()
		}
	}

	return nil
}

// AddToPlaylist 添加歌曲到播放列表
func (m *Manager) AddToPlaylist(playlistName string, items []*models.PlaylistItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.playlists {
		if m.playlists[i].Name == playlistName {
			// 添加新歌曲，避免重复
			existingIDs := make(map[int64]bool)
			for _, item := range m.playlists[i].Items {
				existingIDs[item.FsID] = true
			}

			for _, item := range items {
				if !existingIDs[item.FsID] {
					item.AddTime = time.Now().Unix()
					m.playlists[i].Items = append(m.playlists[i].Items, item)
				}
			}

			m.playlists[i].UpdateTime = time.Now().Unix()
			return m.savePlaylistsWithoutLock()
		}
	}

	return nil
}

// RemoveFromPlaylist 从播放列表删除歌曲
func (m *Manager) RemoveFromPlaylist(playlistName string, fsID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.playlists {
		if m.playlists[i].Name == playlistName {
			items := m.playlists[i].Items
			for j, item := range items {
				if item.FsID == fsID {
					m.playlists[i].Items = append(items[:j], items[j+1:]...)
					m.playlists[i].UpdateTime = time.Now().Unix()
					return m.savePlaylistsWithoutLock()
				}
			}
		}
	}

	return nil
}

// ClearPlaylist 清空播放列表
func (m *Manager) ClearPlaylist(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.playlists {
		if m.playlists[i].Name == name {
			m.playlists[i].Items = nil
			m.playlists[i].UpdateTime = time.Now().Unix()
			return m.savePlaylistsWithoutLock()
		}
	}

	return nil
}

// ReorderPlaylistItems 重排播放列表中的歌曲
func (m *Manager) ReorderPlaylistItems(playlistName string, fromIndex, toIndex int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.playlists {
		if m.playlists[i].Name == playlistName {
			items := m.playlists[i].Items
			if fromIndex < 0 || fromIndex >= len(items) ||
				toIndex < 0 || toIndex >= len(items) {
				return nil
			}

			// 移动项目
			item := items[fromIndex]
			items = append(items[:fromIndex], items[fromIndex+1:]...)
			items = append(items[:toIndex], append([]*models.PlaylistItem{item}, items[toIndex:]...)...)

			m.playlists[i].Items = items
			m.playlists[i].UpdateTime = time.Now().Unix()
			return m.savePlaylistsWithoutLock()
		}
	}

	return nil
}

// UpdateRecentSongs 更新最近播放
func (m *Manager) UpdateRecentSongs(songs []*models.PlaylistItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 找到或创建"最近播放"列表
	for i := range m.playlists {
		if m.playlists[i].Name == "最近播放" {
			m.playlists[i].Items = songs
			m.playlists[i].UpdateTime = time.Now().Unix()
			return m.savePlaylistsWithoutLock()
		}
	}

	// 创建"最近播放"
	m.playlists = append([]models.Playlist{{
		Name:        "最近播放",
		Description: "最近播放的歌曲",
		Items:       songs,
		CreateTime:  time.Now().Unix(),
		UpdateTime:  time.Now().Unix(),
	}}, m.playlists...)

	// 重新排序确保"最近播放"在第一位
	m.sortPlaylists()
	return m.savePlaylistsWithoutLock()
}

// RefreshPlaylist 刷新播放列表
// 算法核心：从现有歌曲路径反推根文件夹，然后重新递归扫描并重建列表
func (m *Manager) RefreshPlaylist(apiClient *api.BaiduPanClient, playlistName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 找到指定的播放列表
	var playlist *models.Playlist
	for i := range m.playlists {
		if m.playlists[i].Name == playlistName {
			playlist = &m.playlists[i]
			break
		}
	}

	if playlist == nil {
		return fmt.Errorf("播放列表不存在: %s", playlistName)
	}

	// 如果是"最近播放"列表，不支持刷新（它会自动更新）
	if playlistName == "最近播放" {
		return fmt.Errorf("最近播放列表不能刷新，它会自动更新")
	}

	// 1. 获取当前列表所有歌曲
	if len(playlist.Items) == 0 {
		return fmt.Errorf("列表为空，无法刷新")
	}

	// 2. 计算需要扫描的根路径集合 (核心算法)
	// 提取所有歌曲的直接父文件夹路径
	parentPaths := make(map[string]bool)
	for _, item := range playlist.Items {
		parent := getParentPath(item.Path)
		if parent != "" {
			parentPaths[parent] = true
		}
	}

	// 转换为slice并排序（短的在前）
	var sortedPaths []string
	for path := range parentPaths {
		sortedPaths = append(sortedPaths, path)
	}
	// 按长度排序，短的在前（父目录通常比子目录短）
	// 这样比较时效率更高
	sort.Strings(sortedPaths)

	// 过滤被包含的路径
	roots := make(map[string]bool)
	for _, path := range sortedPaths {
		isChild := false
		// 检查当前path是否是roots中某个路径的子路径
		for root := range roots {
			// 判断逻辑：root是path的前缀，且path的下一个字符是'/'（或者是完全相等）
			// 例如 root="/music", path="/music/rock" -> true
			// 例如 root="/music", path="/musical" -> false
			if strings.HasPrefix(path, root) {
				if len(path) == len(root) || path[len(root)] == '/' {
					isChild = true
					break
				}
			}
		}

		if !isChild {
			roots[path] = true
		}
	}

	// 3. 递归扫描这些根路径，获取网盘最新文件列表
	var allFiles []api.FileInfo
	scannedPaths := make(map[string]bool)
	for root := range roots {
		err := scanFolderRecursive(apiClient, root, &allFiles, scannedPaths)
		if err != nil {
			return fmt.Errorf("扫描根路径 %s 失败: %w", root, err)
		}
	}

	// 4. 清空当前列表并重建
	// 只保留音频文件
	var newItems []*models.PlaylistItem
	for _, file := range allFiles {
		if file.Isdir == 0 && isAudioFile(file.ServerFilename) {
			timestamp := time.Now().Unix()
			item := &models.PlaylistItem{
				FsID:           file.FsID,
				ServerFileName: file.ServerFilename,
				Path:           file.Path,
				Size:           file.Size,
				Category:       file.Category,
				Isdir:          file.Isdir,
				LocalMtime:     timestamp,
				ServerMtime:    timestamp,
				MD5:            file.ServerMD5,
				AddTime:        time.Now().Unix(),
			}
			newItems = append(newItems, item)
		}
	}

	// 重建播放列表
	playlist.Items = newItems
	playlist.UpdateTime = time.Now().Unix()

	// 保存更改
	return m.savePlaylistsWithoutLock()
}

// calculateSyncRoots 计算最小扫描根集合（与Java实现相同逻辑）
// 这个函数是内部实现，被RefreshPlaylist调用
func calculateSyncRoots(items []*models.PlaylistItem) map[string]bool {
	// 1. 收集所有唯一的父路径
	parentPaths := make(map[string]bool)
	for _, item := range items {
		parent := getParentPath(item.Path)
		if parent != "" {
			parentPaths[parent] = true
		}
	}

	// 2. 转换为slice并排序
	var sortedPaths []string
	for path := range parentPaths {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)

	// 3. 过滤被包含的路径
	roots := make(map[string]bool)
	for _, path := range sortedPaths {
		isChild := false
		for root := range roots {
			if strings.HasPrefix(path, root) {
				if len(path) == len(root) || path[len(root)] == '/' {
					isChild = true
					break
				}
			}
		}

		if !isChild {
			roots[path] = true
		}
	}

	return roots
}

// scanFolderRecursive 递归扫描单个文件夹
func scanFolderRecursive(apiClient *api.BaiduPanClient, path string, result *[]api.FileInfo, scannedPaths map[string]bool) error {
	// 防止重复扫描同一个路径
	if scannedPaths[path] {
		return nil
	}
	scannedPaths[path] = true

	// 分页获取文件列表（每次1000个）
	start := 0
	const limit = 1000
	for {
		files, err := apiClient.GetFileList(path, start/limit+1, limit)
		if err != nil {
			return err
		}

		if len(files) == 0 {
			break
		}

		for _, file := range files {
			if file.Isdir == 1 {
				// 递归扫描子文件夹
				err := scanFolderRecursive(apiClient, file.Path, result, scannedPaths)
				if err != nil {
					// 忽略错误，继续处理其他文件
					continue
				}
			} else {
				*result = append(*result, file)
			}
		}

		// 检查是否还有更多文件
		if len(files) < limit {
			break
		}
		start += limit
	}

	return nil
}

// getParentPath 获取父路径
func getParentPath(path string) string {
	if path == "" || path == "/" {
		return ""
	}
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash == 0 {
		return "/" // 父路径是根
	}
	if lastSlash > 0 {
		return path[:lastSlash]
	}
	return ""
}

// isAudioFile 检查是否为音频文件
func isAudioFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	audioFormats := map[string]bool{
		".mp3":  true,
		".flac": true,
		".wav":  true,
		".m4a":  true,
		".aac":  true,
		".ogg":  true,
		".wma":  true,
		".ape":  true,
		".alac": true,
	}
	return audioFormats[ext]
}

// sortPlaylists 排序播放列表："最近播放"固定在第一位，其他按创建时间倒排
func (m *Manager) sortPlaylists() {
	if len(m.playlists) <= 1 {
		return
	}

	var recentPlaylist *models.Playlist
	var otherPlaylists []models.Playlist

	// 分离"最近播放"和其他列表
	// 使用索引访问，避免循环变量指针问题
	for i := range m.playlists {
		if m.playlists[i].Name == "最近播放" {
			recentPlaylist = &m.playlists[i]
		} else {
			otherPlaylists = append(otherPlaylists, m.playlists[i])
		}
	}

	// 对其他列表按创建时间倒序排序
	sort.Slice(otherPlaylists, func(i, j int) bool {
		return otherPlaylists[i].CreateTime > otherPlaylists[j].CreateTime
	})

	// 重新组合：最近播放在第一位（如果存在），然后是其他列表
	m.playlists = nil
	if recentPlaylist != nil {
		m.playlists = append(m.playlists, *recentPlaylist)
	}
	m.playlists = append(m.playlists, otherPlaylists...)
}
