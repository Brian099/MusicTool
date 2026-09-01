package feiniu

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"music-toolkit/internal/database"
)

// Client 飞牛音乐 NAS HTTP 客户端
type Client struct {
	db         *database.DB
	httpClient *http.Client

	mu           sync.RWMutex
	serverURL    string
	username     string
	passwordHash string
	deviceID     string
	accessCode   string
	userToken    string
	lastLoginAt  int64
}

// NewClient 创建飞牛客户端实例
func NewClient(db *database.DB) *Client {
	c := &Client{
		db: db,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	_ = c.InitFromDB(context.Background())
	return c
}

// InitFromDB 从本地 SQLite 读取已持久化的配置
func (c *Client) InitFromDB(ctx context.Context) error {
	if c.db == nil {
		return nil
	}
	cfg, err := c.db.GetFeiNiuConfig(ctx)
	if err != nil || cfg == nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.serverURL = normalizeURL(cfg.ServerURL)
	c.username = cfg.Username
	c.passwordHash = cfg.PasswordHash
	c.deviceID = cfg.DeviceID
	c.accessCode = cfg.AccessCode
	c.userToken = cfg.UserToken
	c.lastLoginAt = cfg.UpdatedAt
	return nil
}

// SHA256 计算输入文本的 SHA-256 十六进制字符串
func SHA256(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

// GenerateDeviceID 生成 32 位随机十六进制设备 ID
func GenerateDeviceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "0123456789abcdef0123456789abcdef"
	}
	return hex.EncodeToString(b)
}

func normalizeURL(u string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(u), "/")
	trimmed = strings.TrimSuffix(trimmed, "/music/api/v1")
	return strings.TrimRight(trimmed, "/")
}

// SetAuth 配置认证信息并执行密码登录
func (c *Client) SetAuth(ctx context.Context, serverURL, username, rawPassword, accessCode string) (*LoginData, error) {
	normURL := normalizeURL(serverURL)
	if normURL == "" {
		return nil, errors.New("服务器地址不能为空")
	}
	if username == "" {
		return nil, errors.New("用户名不能为空")
	}

	devID := c.GetDeviceID()
	if devID == "" {
		devID = GenerateDeviceID()
	}

	passHash := SHA256(rawPassword)

	loginResp, err := c.doLoginRaw(ctx, normURL, username, passHash, devID, accessCode)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()

	c.mu.Lock()
	c.serverURL = normURL
	c.username = username
	c.passwordHash = passHash
	c.deviceID = devID
	c.accessCode = accessCode
	c.userToken = loginResp.UserToken
	c.lastLoginAt = now
	c.mu.Unlock()

	if c.db != nil {
		_ = c.db.SaveFeiNiuConfig(ctx, &database.FeiNiuConfigRecord{
			ID:           1,
			ServerURL:    normURL,
			Username:     username,
			PasswordHash: passHash,
			DeviceID:     devID,
			AccessCode:   accessCode,
			UserToken:    loginResp.UserToken,
			UpdatedAt:    now,
		})
	}

	return loginResp, nil
}

// Disconnect 断开连接并清除配置
func (c *Client) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	c.serverURL = ""
	c.username = ""
	c.passwordHash = ""
	c.deviceID = ""
	c.accessCode = ""
	c.userToken = ""
	c.lastLoginAt = 0
	c.mu.Unlock()

	if c.db != nil {
		return c.db.ClearFeiNiuConfig(ctx)
	}
	return nil
}

// GetStatus 获取当前飞牛客户端连接状态
func (c *Client) GetStatus(ctx context.Context) StatusResponse {
	c.mu.RLock()
	serverURL := c.serverURL
	username := c.username
	token := c.userToken
	updatedAt := c.lastLoginAt
	c.mu.RUnlock()

	if serverURL == "" || token == "" {
		return StatusResponse{
			Connected: false,
			ServerURL: serverURL,
			Username:  username,
			UpdatedAt: updatedAt,
		}
	}

	// 快速探测获取歌单列表第一页验证有效性
	_, err := c.GetPlaylists(ctx, 1, 1)
	if err != nil {
		return StatusResponse{
			Connected: false,
			ServerURL: serverURL,
			Username:  username,
			UpdatedAt: updatedAt,
			Error:     err.Error(),
		}
	}

	return StatusResponse{
		Connected: true,
		ServerURL: serverURL,
		Username:  username,
		UpdatedAt: updatedAt,
	}
}

// GetDeviceID 获取或返回现有设备 ID
func (c *Client) GetDeviceID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.deviceID
}

func (c *Client) doLoginRaw(ctx context.Context, serverURL, username, passwordHash, devID, accessCode string) (*LoginData, error) {
	apiURL := fmt.Sprintf("%s/music/api/v1/user/password-login", serverURL)

	reqBody := LoginRequest{
		Username: username,
		Password: passwordHash,
		DeviceId: devID,
	}
	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal login req: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("create login req: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if accessCode != "" {
		httpReq.Header.Set("x-access-code", base64.StdEncoding.EncodeToString([]byte(accessCode)))
		httpReq.Header.Set("x-access-source", "app")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("connect to feiniu nas failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read login resp: %w", err)
	}

	var fnResp FeiNiuResponse[LoginData]
	if err := json.Unmarshal(bodyBytes, &fnResp); err != nil {
		return nil, fmt.Errorf("unmarshal login resp: %w (status %d)", err, resp.StatusCode)
	}

	if fnResp.Code != 0 {
		if fnResp.Code == 120001 {
			return nil, errors.New("用户名或密码错误，请核对后重试")
		}
		if fnResp.Msg != "" {
			return nil, errors.New(fnResp.Msg)
		}
		return nil, fmt.Errorf("飞牛登录失败 (错误码: %d)", fnResp.Code)
	}

	if fnResp.Data.UserToken == "" {
		return nil, errors.New("飞牛服务端未返回有效 userToken")
	}

	return &fnResp.Data, nil
}

// ensureLogin 在 Token 失效时加锁自动重新登录换取新 Token
func (c *Client) ensureLogin(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.serverURL == "" || c.username == "" || c.passwordHash == "" {
		return errors.New("飞牛 NAS 未配置登录账号或密码")
	}

	devID := c.deviceID
	if devID == "" {
		devID = GenerateDeviceID()
		c.deviceID = devID
	}

	loginResp, err := c.doLoginRaw(ctx, c.serverURL, c.username, c.passwordHash, devID, c.accessCode)
	if err != nil {
		return fmt.Errorf("自动刷新飞牛 Token 失败: %w", err)
	}

	now := time.Now().Unix()
	c.userToken = loginResp.UserToken
	c.lastLoginAt = now

	if c.db != nil {
		_ = c.db.SaveFeiNiuConfig(ctx, &database.FeiNiuConfigRecord{
			ID:           1,
			ServerURL:    c.serverURL,
			Username:     c.username,
			PasswordHash: c.passwordHash,
			DeviceID:     c.deviceID,
			AccessCode:   c.accessCode,
			UserToken:    c.userToken,
			UpdatedAt:    now,
		})
	}
	return nil
}

// doRequest 通用 API 请求封装，支持自动附带 Cookie 及 401 自动重连重试
func (c *Client) doRequest(ctx context.Context, method, path string, body any, respTarget any) error {
	return c.doRequestInternal(ctx, method, path, body, respTarget, 0)
}

func (c *Client) doRequestInternal(ctx context.Context, method, path string, body any, respTarget any, retryCount int) error {
	c.mu.RLock()
	serverURL := c.serverURL
	token := c.userToken
	accessCode := c.accessCode
	c.mu.RUnlock()

	if serverURL == "" {
		return errors.New("未配置飞牛 NAS 服务端地址")
	}

	// 拼接完整 URL
	relPath := strings.TrimPrefix(path, "/")
	fullURL := fmt.Sprintf("%s/music/api/v1/%s", serverURL, relPath)

	var reqReader io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqReader = bytes.NewReader(jsonBytes)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, fullURL, reqReader)
	if err != nil {
		return fmt.Errorf("create http request: %w", err)
	}

	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		httpReq.Header.Set("Cookie", fmt.Sprintf("music-token=%s", token))
	}
	if accessCode != "" {
		httpReq.Header.Set("x-access-code", base64.StdEncoding.EncodeToString([]byte(accessCode)))
		httpReq.Header.Set("x-access-source", "app")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request feiniu api (%s %s) failed: %w", method, path, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read feiniu response: %w", err)
	}

	// 检查是否 401 或 Token 失效
	isUnauthorized := resp.StatusCode == http.StatusUnauthorized
	if !isUnauthorized && len(bodyBytes) > 0 {
		var genericResp FeiNiuResponse[any]
		if err := json.Unmarshal(bodyBytes, &genericResp); err == nil {
			if genericResp.Code == 401 || strings.Contains(strings.ToLower(genericResp.Msg), "invalid token") {
				isUnauthorized = true
			}
		}
	}

	if isUnauthorized {
		if retryCount >= 1 {
			return errors.New("飞牛 NAS 登录态已失效且自动重连失败，请重新配置密码")
		}
		// 触发自动重新登录
		if err := c.ensureLogin(ctx); err != nil {
			return fmt.Errorf("登录失效且重连失败: %w", err)
		}
		// 重新发起请求
		return c.doRequestInternal(ctx, method, path, body, respTarget, retryCount+1)
	}

	if respTarget != nil {
		if err := json.Unmarshal(bodyBytes, respTarget); err != nil {
			return fmt.Errorf("parse feiniu response: %w (body: %s)", err, string(bodyBytes))
		}
	}

	return nil
}

// GetPlaylists 获取歌单列表（支持分页）
func (c *Client) GetPlaylists(ctx context.Context, page, size int) (*FeiNiuPageData[FeiNiuPlaylist], error) {
	if page <= 0 {
		page = 1
	}
	if size == 0 {
		size = 50
	}
	path := fmt.Sprintf("playlist/list?page=%d&size=%d", page, size)

	var resp FeiNiuResponse[FeiNiuPageData[FeiNiuPlaylist]]
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("获取歌单失败: %s (code %d)", resp.Msg, resp.Code)
	}
	return &resp.Data, nil
}

// GetPlaylistTracks 获取歌单内曲目列表
func (c *Client) GetPlaylistTracks(ctx context.Context, playlistGUID string, page, size int) (*FeiNiuPageData[FeiNiuTrack], error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 300
	}
	path := fmt.Sprintf("track/playlist-detail/list?playlistGUID=%s&page=%d&size=%d", url.QueryEscape(playlistGUID), page, size)

	var resp FeiNiuResponse[FeiNiuPageData[FeiNiuTrack]]
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("获取歌单歌曲失败: %s (code %d)", resp.Msg, resp.Code)
	}
	return &resp.Data, nil
}

// CreatePlaylist 创建新歌单
func (c *Client) CreatePlaylist(ctx context.Context, name string, coverID string) (*FeiNiuPlaylist, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("歌单名称不能为空")
	}
	req := PlaylistCreateReq{
		Name:    name,
		CoverID: coverID,
	}

	var resp FeiNiuResponse[FeiNiuPlaylist]
	if err := c.doRequest(ctx, "POST", "playlist/create", req, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("创建歌单失败: %s (code %d)", resp.Msg, resp.Code)
	}
	return &resp.Data, nil
}

// EditPlaylist 编辑/重命名歌单
func (c *Client) EditPlaylist(ctx context.Context, guid, name, coverID string) error {
	if guid == "" {
		return errors.New("歌单 GUID 不能为空")
	}
	req := PlaylistEditReq{
		GUID:    guid,
		Name:    name,
		CoverID: coverID,
	}

	var resp FeiNiuResponse[any]
	if err := c.doRequest(ctx, "POST", "playlist/edit", req, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("编辑歌单失败: %s (code %d)", resp.Msg, resp.Code)
	}
	return nil
}

// DeletePlaylist 删除歌单
func (c *Client) DeletePlaylist(ctx context.Context, guid string) error {
	if guid == "" {
		return errors.New("歌单 GUID 不能为空")
	}
	req := PlaylistDeleteReq{
		GUID: guid,
	}

	var resp FeiNiuResponse[any]
	if err := c.doRequest(ctx, "POST", "playlist/delete", req, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("删除歌单失败: %s (code %d)", resp.Msg, resp.Code)
	}
	return nil
}

// AddTracksToPlaylist 向指定歌单批量添加曲目
func (c *Client) AddTracksToPlaylist(ctx context.Context, guid string, trackGUIDs []string) error {
	if guid == "" {
		return errors.New("歌单 GUID 不能为空")
	}
	if len(trackGUIDs) == 0 {
		return nil
	}

	req := PlaylistTracksActionReq{
		GUID:       guid,
		TrackGUIDs: trackGUIDs,
	}

	var resp FeiNiuResponse[any]
	if err := c.doRequest(ctx, "POST", "playlist/add-track", req, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("向歌单添加歌曲失败: %s (code %d)", resp.Msg, resp.Code)
	}
	return nil
}

// RemoveTracksFromPlaylist 从指定歌单移除曲目
func (c *Client) RemoveTracksFromPlaylist(ctx context.Context, guid string, trackGUIDs []string) error {
	if guid == "" {
		return errors.New("歌单 GUID 不能为空")
	}
	if len(trackGUIDs) == 0 {
		return nil
	}

	req := PlaylistTracksActionReq{
		GUID:       guid,
		TrackGUIDs: trackGUIDs,
	}

	var resp FeiNiuResponse[any]
	if err := c.doRequest(ctx, "POST", "playlist/remove-track", req, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("从歌单移除歌曲失败: %s (code %d)", resp.Msg, resp.Code)
	}
	return nil
}

// PurgeInvalidTracks 清除歌单内的无效歌曲
func (c *Client) PurgeInvalidTracks(ctx context.Context, guid string) error {
	if guid == "" {
		return errors.New("歌单 GUID 不能为空")
	}

	req := PlaylistDeleteReq{
		GUID: guid,
	}

	var resp FeiNiuResponse[any]
	if err := c.doRequest(ctx, "POST", "playlist/purge-track", req, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("清理无效歌曲失败: %s (code %d)", resp.Msg, resp.Code)
	}
	return nil
}

// SearchTracks 在飞牛曲库中搜索歌曲
func (c *Client) SearchTracks(ctx context.Context, keyword string, page, size int) (*FeiNiuPageData[FeiNiuTrack], error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	path := fmt.Sprintf("search/track?q=%s&page=%d&size=%d", url.QueryEscape(keyword), page, size)

	var resp FeiNiuResponse[FeiNiuPageData[FeiNiuTrack]]
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("搜索曲库失败: %s (code %d)", resp.Msg, resp.Code)
	}
	return &resp.Data, nil
}

// GetCoverReader 代理拉取飞牛封面图片流
func (c *Client) GetCoverReader(ctx context.Context, coverID string, size int) (io.ReadCloser, string, error) {
	c.mu.RLock()
	serverURL := c.serverURL
	token := c.userToken
	accessCode := c.accessCode
	c.mu.RUnlock()

	if serverURL == "" {
		return nil, "", errors.New("未配置飞牛 NAS 服务端地址")
	}

	if size <= 0 {
		size = 400
	}

	fullURL := fmt.Sprintf("%s/music/api/v1/static/cover?coverId=%s&size=%d", serverURL, url.QueryEscape(coverID), size)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, "", err
	}

	if token != "" {
		httpReq.Header.Set("Cookie", fmt.Sprintf("music-token=%s", token))
	}
	if accessCode != "" {
		httpReq.Header.Set("x-access-code", base64.StdEncoding.EncodeToString([]byte(accessCode)))
		httpReq.Header.Set("x-access-source", "app")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("feiniu cover status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	return resp.Body, contentType, nil
}
