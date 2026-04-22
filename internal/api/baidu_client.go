package api

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
)

const (
	BaiduAPIBase = "https://pan.baidu.com/rest/2.0"
)

// BaiduPanClient 百度网盘客户端
type BaiduPanClient struct {
	client      *resty.Client
	accessToken string
	tokenFile   string
}

// TokenInfo 百度网盘令牌信息
type TokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	ExpiresAt    int64  `json:"expires_at"`
}

// FileInfo 百度网盘文件信息
type FileInfo struct {
	FsID           int64  `json:"fs_id"`
	Path           string `json:"path"`
	ServerFilename string `json:"server_filename"` // 百度网盘API返回的是server_filename
	Size           int64  `json:"size"`
	Isdir          int    `json:"isdir"`
	Category       int    `json:"category"`
	ServerMD5      string `json:"server_md5"`
}

// ListResponse 文件列表响应
type ListResponse struct {
	List      []FileInfo `json:"list"`
	RequestID int64      `json:"request_id"`
	Errno     int        `json:"errno"`
	ErrMsg    string     `json:"errmsg,omitempty"`
}

// GetFileList 获取文件列表
func (c *BaiduPanClient) GetFileList(dir string, page, num int) ([]FileInfo, error) {
	resp, err := c.client.R().
		SetQueryParams(map[string]string{
			"method":       "list",
			"access_token": c.accessToken,
			"dir":          dir,
			"page":         fmt.Sprintf("%d", page),
			"num":          fmt.Sprintf("%d", num),
			"order":        "name",
			"desc":         "0",
		}).
		Get(BaiduAPIBase + "/xpan/file")

	if err != nil {
		return nil, err
	}

	var listResp ListResponse
	if err := json.Unmarshal(resp.Body(), &listResp); err != nil {
		return nil, err
	}

	if listResp.Errno != 0 {
		return nil, fmt.Errorf("API error %d: %s", listResp.Errno, listResp.ErrMsg)
	}

	return listResp.List, nil
}

// DownloadInfo 下载信息
type DownloadInfo struct {
	Dlink string `json:"dlink"`
}

// NewBaiduPanClient 创建新的百度网盘客户端
func NewBaiduPanClient(tokenFile string) *BaiduPanClient {
	client := resty.New().
		SetTimeout(30 * time.Second).
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second)

	return &BaiduPanClient{
		client:    client,
		tokenFile: tokenFile,
	}
}

// LoadToken 加载令牌
func (c *BaiduPanClient) LoadToken() error {
	data, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return err
	}

	var token TokenInfo
	if err := json.Unmarshal(data, &token); err != nil {
		return err
	}

	// 检查令牌是否过期
	if time.Now().Unix() > token.ExpiresAt {
		return fmt.Errorf("token expired")
	}

	c.accessToken = token.AccessToken
	return nil
}

// SaveToken 保存令牌
func (c *BaiduPanClient) SaveToken(token *TokenInfo) error {
	// 计算过期时间
	token.ExpiresAt = time.Now().Unix() + int64(token.ExpiresIn)

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(c.tokenFile), 0755); err != nil {
		return err
	}

	c.accessToken = token.AccessToken
	return os.WriteFile(c.tokenFile, data, 0600)
}

// Login 登录（返回授权URL）
func (c *BaiduPanClient) Login(clientID string) (string, error) {
	// 返回授权URL让用户在浏览器中打开
	authURL := fmt.Sprintf(
		"https://openapi.baidu.com/oauth/2.0/authorize?response_type=token&client_id=%s&redirect_uri=oob&scope=basic,netdisk",
		clientID,
	)
	return authURL, nil
}

// SetAccessToken 设置访问令牌
func (c *BaiduPanClient) SetAccessToken(token string) {
	c.accessToken = token
}

// ListAudioFiles 列出音频文件
func (c *BaiduPanClient) ListAudioFiles(ctx context.Context, dir string, page, num int) ([]*models.Track, error) {
	resp, err := c.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"method":       "list",
			"access_token": c.accessToken,
			"dir":          dir,
			"page":         fmt.Sprintf("%d", page),
			"num":          fmt.Sprintf("%d", num),
			"order":        "name",
			"desc":         "0",
		}).
		Get(BaiduAPIBase + "/xpan/file")

	if err != nil {
		return nil, err
	}

	var listResp ListResponse
	if err := json.Unmarshal(resp.Body(), &listResp); err != nil {
		return nil, err
	}

	if listResp.Errno != 0 {
		return nil, fmt.Errorf("API error %d: %s", listResp.Errno, listResp.ErrMsg)
	}

	// 过滤音频文件
	var tracks []*models.Track
	audioFormats := map[string]bool{
		".mp3":  true,
		".flac": true,
		".wav":  true,
		".m4a":  true,
		".aac":  true,
		".ogg":  true,
	}

	for _, file := range listResp.List {
		if file.Isdir == 1 {
			continue // 跳过目录
		}

		ext := filepath.Ext(file.ServerFilename)
		if !audioFormats[ext] {
			continue
		}

		track := &models.Track{
			ID:      fmt.Sprintf("%d", file.FsID),
			Title:   file.ServerFilename,
			Path:    file.Path,
			Size:    file.Size,
			Format:  ext[1:], // 去掉点
			AddedAt: time.Now(),
		}

		// 尝试查找对应的LRC文件
		lrcPath := file.Path[:len(file.Path)-len(ext)] + ".lrc"
		track.LRCPath = lrcPath

		tracks = append(tracks, track)
	}

	return tracks, nil
}

// GetDownloadLink 获取下载链接
func (c *BaiduPanClient) GetDownloadLink(ctx context.Context, fsID int64) (string, error) {
	resp, err := c.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"method":       "filemetas",
			"access_token": c.accessToken,
			"fsids":        fmt.Sprintf("[%d]", fsID),
			"dlink":        "1",
		}).
		Get(BaiduAPIBase + "/xpan/multimedia")

	if err != nil {
		return "", err
	}

	var result struct {
		List  []DownloadInfo `json:"list"`
		Errno int            `json:"errno"`
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return "", err
	}

	if result.Errno != 0 || len(result.List) == 0 {
		return "", fmt.Errorf("failed to get download link")
	}

	return result.List[0].Dlink + "&access_token=" + c.accessToken, nil
}

// DownloadFile 下载文件
func (c *BaiduPanClient) DownloadFile(ctx context.Context, dlink string, writer io.Writer) error {
	resp, err := c.client.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		Get(dlink)

	if err != nil {
		return err
	}
	defer resp.RawBody().Close()

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("download failed: %d", resp.StatusCode())
	}

	_, err = io.Copy(writer, resp.RawBody())
	return err
}

// CheckLRCFileExists 检查同目录下是否存在 LRC 歌词文件
func (c *BaiduPanClient) CheckLRCFileExists(ctx context.Context, audioPath string) (*FileInfo, error) {
	// 提取目录和文件名（使用字符串操作，避免Windows上filepath.Dir的路径转换问题）
	// 百度网盘路径格式: /目录/子目录/文件名.mp3

	// 统一转换为Unix风格路径
	unixPath := strings.ReplaceAll(audioPath, "\\", "/")

	// 移除末尾的斜杠（如果有）
	unixPath = strings.TrimRight(unixPath, "/")

	// 提取目录路径：找到最后一个 / 之前的部分
	lastSlash := strings.LastIndex(unixPath, "/")
	var dir string
	if lastSlash > 0 {
		dir = unixPath[:lastSlash]
	} else {
		dir = "/"
	}

	// 提取文件名
	baseName := ""
	if lastSlash >= 0 && lastSlash < len(unixPath) {
		baseName = unixPath[lastSlash+1:]
	} else {
		baseName = unixPath
	}

	// 移除音频文件扩展名
	ext := filepath.Ext(baseName)
	if ext != "" {
		baseName = strings.TrimSuffix(baseName, ext)
	}

	// 可能的 LRC 文件名（大小写不敏感）
	possibleNames := []string{
		baseName + ".lrc",
		baseName + ".LRC",
		baseName + ".Lrc",
	}

	// 获取目录文件列表
	files, err := c.GetFileList(dir, 1, 1000)
	if err != nil {
		return nil, err
	}

	// 查找 LRC 文件（大小写不敏感）
	for _, file := range files {
		fileName := file.ServerFilename
		for _, possibleName := range possibleNames {
			if strings.EqualFold(fileName, possibleName) {
				return &file, nil
			}
		}
	}

	return nil, nil // 未找到
}

// DownloadLRCContent 下载 LRC 文件内容
func (c *BaiduPanClient) DownloadLRCContent(ctx context.Context, fsID int64) (string, error) {
	// 获取下载链接
	downloadURL, err := c.GetDownloadLink(ctx, fsID)
	if err != nil {
		return "", err
	}

	// 下载文件内容
	resp, err := c.client.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		Get(downloadURL)
	if err != nil {
		return "", err
	}
	defer resp.RawBody().Close()

	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("download failed: %d", resp.StatusCode())
	}

	// 读取内容
	content, err := io.ReadAll(resp.RawBody())
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// SearchAudioFiles 搜索音频文件
func (c *BaiduPanClient) SearchAudioFiles(ctx context.Context, key string, page, num int) ([]*models.Track, error) {
	resp, err := c.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"method":       "search",
			"access_token": c.accessToken,
			"key":          key,
			"page":         fmt.Sprintf("%d", page),
			"num":          fmt.Sprintf("%d", num),
			"recursion":    "1",
		}).
		Get(BaiduAPIBase + "/xpan/file")

	if err != nil {
		return nil, err
	}

	var listResp ListResponse
	if err := json.Unmarshal(resp.Body(), &listResp); err != nil {
		return nil, err
	}

	if listResp.Errno != 0 {
		return nil, fmt.Errorf("API error %d: %s", listResp.Errno, listResp.ErrMsg)
	}

	// 过滤音频文件
	var tracks []*models.Track
	audioFormats := map[string]bool{
		".mp3":  true,
		".flac": true,
		".wav":  true,
		".m4a":  true,
		".aac":  true,
		".ogg":  true,
	}

	for _, file := range listResp.List {
		if file.Isdir == 1 {
			continue
		}

		ext := filepath.Ext(file.ServerFilename)
		if !audioFormats[ext] {
			continue
		}

		track := &models.Track{
			ID:      fmt.Sprintf("%d", file.FsID),
			Title:   file.ServerFilename,
			Path:    file.Path,
			Size:    file.Size,
			Format:  ext[1:],
			AddedAt: time.Now(),
		}

		tracks = append(tracks, track)
	}

	return tracks, nil
}

// GetAudioFilesRecursive 递归获取文件夹中的所有音频文件
func (c *BaiduPanClient) GetAudioFilesRecursive(dir string) ([]FileInfo, error) {
	var allFiles []FileInfo
	audioFormats := map[string]bool{
		".mp3":  true,
		".m4a":  true,
		".flac": true,
		".wav":  true,
		".ogg":  true,
		".aac":  true,
		".wma":  true,
		".ape":  true,
		".alac": true,
	}

	// 递归函数
	var recurse func(currentDir string) error
	recurse = func(currentDir string) error {
		// 获取当前目录的文件列表
		files, err := c.GetFileList(currentDir, 1, 1000)
		if err != nil {
			return err
		}

		// 处理文件
		for _, file := range files {
			if file.Isdir == 1 {
				// 如果是文件夹，递归处理
				if err := recurse(file.Path); err != nil {
					// 忽略错误，继续处理其他文件
					continue
				}
			} else {
				// 如果是文件，检查是否为音频文件
				ext := strings.ToLower(filepath.Ext(file.ServerFilename))
				if audioFormats[ext] {
					allFiles = append(allFiles, file)
				}
			}
		}

		return nil
	}

	// 开始递归
	if err := recurse(dir); err != nil {
		return nil, err
	}

	return allFiles, nil
}

// UploadFile 上传文件到百度网盘（三步上传）

// UploadFile 上传文件到百度网盘（三步上传）
// 参考electron项目的上传逻辑实现
func (c *BaiduPanClient) UploadFile(ctx context.Context, localPath, targetPath string) error {
	// 读取文件内容
	fileData, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("read file failed: %w", err)
	}

	fileSize := len(fileData)

	// 计算MD5
	md5Hash := md5.Sum(fileData)
	md5Str := hex.EncodeToString(md5Hash[:])

	// 步骤1: 预创建
	precreateResp, err := c.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"method":       "precreate",
			"access_token": c.accessToken,
		}).
		SetFormData(map[string]string{
			"path":       targetPath,
			"size":       fmt.Sprintf("%d", fileSize),
			"isdir":      "0",
			"autoinit":   "1",
			"block_list": fmt.Sprintf(`["%s"]`, md5Str),
			"rtype":      "3", // 覆盖模式
		}).
		Post(BaiduAPIBase + "/xpan/file")

	if err != nil {
		return fmt.Errorf("precreate failed: %w", err)
	}

	var precreateData struct {
		Errno      int    `json:"errno"`
		ErrMsg     string `json:"errmsg"`
		UploadID   string `json:"uploadid"`
		ReturnType int    `json:"return_type"`
	}

	if err := json.Unmarshal(precreateResp.Body(), &precreateData); err != nil {
		return fmt.Errorf("parse precreate response failed: %w", err)
	}

	if precreateData.Errno != 0 {
		return fmt.Errorf("precreate failed: %s (%d)", precreateData.ErrMsg, precreateData.Errno)
	}

	// 秒传成功
	if precreateData.ReturnType == 2 {
		return nil
	}

	uploadID := precreateData.UploadID

	// 步骤2: 上传分片
	uploadURL := fmt.Sprintf(
		"https://d.pcs.baidu.com/rest/2.0/pcs/superfile2?method=upload&access_token=%s&type=tmpfile&path=%s&uploadid=%s&partseq=0",
		c.accessToken,
		url.QueryEscape(targetPath),
		uploadID,
	)

	// 使用multipart/form-data上传
	resp, err := c.client.R().
		SetContext(ctx).
		SetFileReader("file", filepath.Base(localPath), bytes.NewReader(fileData)).
		Post(uploadURL)

	if err != nil {
		return fmt.Errorf("upload chunk failed: %w", err)
	}

	var uploadResult struct {
		Errno  int    `json:"errno"`
		ErrMsg string `json:"errmsg"`
	}

	if err := json.Unmarshal(resp.Body(), &uploadResult); err != nil {
		return fmt.Errorf("parse upload response failed: %w", err)
	}

	if uploadResult.Errno != 0 {
		return fmt.Errorf("upload chunk failed: %s (%d)", uploadResult.ErrMsg, uploadResult.Errno)
	}

	// 步骤3: 创建文件
	createResp, err := c.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"method":       "create",
			"access_token": c.accessToken,
		}).
		SetFormData(map[string]string{
			"path":       targetPath,
			"size":       fmt.Sprintf("%d", fileSize),
			"isdir":      "0",
			"uploadid":   uploadID,
			"block_list": fmt.Sprintf(`["%s"]`, md5Str),
			"rtype":      "3",
		}).
		Post(BaiduAPIBase + "/xpan/file")

	if err != nil {
		return fmt.Errorf("create file failed: %w", err)
	}

	var createData struct {
		Errno  int    `json:"errno"`
		ErrMsg string `json:"errmsg"`
	}

	if err := json.Unmarshal(createResp.Body(), &createData); err != nil {
		return fmt.Errorf("parse create response failed: %w", err)
	}

	if createData.Errno != 0 {
		return fmt.Errorf("create file failed: %s (%d)", createData.ErrMsg, createData.Errno)
	}

	return nil
}
