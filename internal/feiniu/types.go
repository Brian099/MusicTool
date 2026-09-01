package feiniu

import (
	"encoding/json"

	"music-toolkit/internal/playlist"
)

// FeiNiuResponse 飞牛官方通用响应结构
type FeiNiuResponse[T any] struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data T      `json:"data"`
}

// FeiNiuPageData 飞牛分页通用结构
type FeiNiuPageData[T any] struct {
	List  []T    `json:"list"`
	Total int    `json:"total"`
	Sort  string `json:"sort,omitempty"`
}

// LoginRequest 密码登录请求
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` // 客户端计算的 SHA-256
	DeviceId string `json:"deviceId"` // 32位十六进制
}

// LoginData 登录成功响应数据
type LoginData struct {
	UserToken string `json:"userToken"`
	Username  string `json:"username"`
}

// FeiNiuPlaylist 飞牛歌单模型
type FeiNiuPlaylist struct {
	GUID       string `json:"guid"`
	Name       string `json:"name"`
	CoverID    string `json:"coverId"`
	TrackCount int    `json:"trackCount"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

func (p *FeiNiuPlaylist) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["guid"].(string); ok {
		p.GUID = v
	}
	if v, ok := raw["name"].(string); ok {
		p.Name = v
	}
	if v, ok := raw["coverId"].(string); ok {
		p.CoverID = v
	}
	if v, ok := raw["trackCount"].(float64); ok {
		p.TrackCount = int(v)
	} else if v, ok := raw["count"].(float64); ok {
		p.TrackCount = int(v)
	} else if v, ok := raw["songCount"].(float64); ok {
		p.TrackCount = int(v)
	} else if v, ok := raw["itemCount"].(float64); ok {
		p.TrackCount = int(v)
	} else if v, ok := raw["total"].(float64); ok {
		p.TrackCount = int(v)
	}
	if v, ok := raw["createdAt"].(float64); ok {
		p.CreatedAt = int64(v)
	}
	if v, ok := raw["updatedAt"].(float64); ok {
		p.UpdatedAt = int64(v)
	}
	return nil
}

// FeiNiuAlbum 飞牛专辑模型（支持嵌套对象或字符串解析）
type FeiNiuAlbum struct {
	GUID       string `json:"guid,omitempty"`
	Name       string `json:"name,omitempty"`
	Title      string `json:"title,omitempty"`
	CoverID    string `json:"coverId,omitempty"`
	TrackCount int    `json:"trackCount,omitempty"`
}

func (a *FeiNiuAlbum) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		a.Name = s
		a.Title = s
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	if v, ok := raw["guid"].(string); ok {
		a.GUID = v
	}
	if v, ok := raw["name"].(string); ok {
		a.Name = v
	}
	if v, ok := raw["title"].(string); ok && a.Name == "" {
		a.Name = v
		a.Title = v
	}
	if v, ok := raw["coverId"].(string); ok {
		a.CoverID = v
	}
	if v, ok := raw["trackCount"].(float64); ok {
		a.TrackCount = int(v)
	}
	return nil
}

// FeiNiuArtist 歌手信息（支持嵌套对象或字符串解析）
type FeiNiuArtist struct {
	GUID string `json:"guid,omitempty"`
	Name string `json:"name,omitempty"`
}

func (a *FeiNiuArtist) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		a.Name = s
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	if v, ok := raw["guid"].(string); ok {
		a.GUID = v
	}
	if v, ok := raw["name"].(string); ok {
		a.Name = v
	}
	return nil
}

// FeiNiuAudioSpec 音频规格
type FeiNiuAudioSpec struct {
	Format     string  `json:"format,omitempty"`
	Bitrate    int     `json:"bitrate,omitempty"`
	SampleRate int     `json:"sampleRate,omitempty"`
	Channels   int     `json:"channels,omitempty"`
	Channel    int     `json:"channel,omitempty"`
	Duration   float64 `json:"duration,omitempty"`
	Path       string  `json:"path,omitempty"`
}

// FeiNiuTrack 飞牛单曲模型
type FeiNiuTrack struct {
	GUID        string          `json:"guid"`
	Title       string          `json:"title"`
	ArtistGUIDs []string        `json:"artistGUIDs,omitempty"`
	Artists     []FeiNiuArtist  `json:"artists,omitempty"`
	Album       FeiNiuAlbum     `json:"album,omitempty"`
	AlbumGUID   string          `json:"albumGUID,omitempty"`
	CoverID     string          `json:"coverId,omitempty"`
	Duration    float64         `json:"duration,omitempty"`
	AudioSpec   FeiNiuAudioSpec `json:"audioSpec,omitempty"`
	CreatedAt   int64           `json:"createdAt,omitempty"`
	UpdatedAt   int64           `json:"updatedAt,omitempty"`
}

func (t *FeiNiuTrack) GetArtistNames() []string {
	var names []string
	for _, a := range t.Artists {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	return names
}

func (t *FeiNiuTrack) GetAlbumName() string {
	if t.Album.Name != "" {
		return t.Album.Name
	}
	if t.Album.Title != "" {
		return t.Album.Title
	}
	return ""
}

// ConnectRequest 连接飞牛 NAS 请求
type ConnectRequest struct {
	ServerURL  string `json:"server_url"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	AccessCode string `json:"access_code,omitempty"`
}

// StatusResponse 飞牛连接状态
type StatusResponse struct {
	Connected bool   `json:"connected"`
	ServerURL string `json:"server_url"`
	Username  string `json:"username"`
	UpdatedAt int64  `json:"updated_at"`
	Error     string `json:"error,omitempty"`
}

// PlaylistCreateReq 创建歌单
type PlaylistCreateReq struct {
	Name    string `json:"name"`
	CoverID string `json:"coverId,omitempty"`
}

// PlaylistEditReq 编辑歌单
type PlaylistEditReq struct {
	GUID    string `json:"guid"`
	Name    string `json:"name"`
	CoverID string `json:"coverId,omitempty"`
}

// PlaylistDeleteReq 删除歌单
type PlaylistDeleteReq struct {
	GUID string `json:"guid"`
}

// PlaylistTracksActionReq 添加/移除歌曲
type PlaylistTracksActionReq struct {
	GUID       string   `json:"guid"`
	TrackGUIDs []string `json:"trackGUIDs"`
}

// ImportPlaylistRequest 外部歌单导入请求
type ImportPlaylistRequest struct {
	Name         string              `json:"name"`          // 歌单名称（若为空且新建则使用原歌单名）
	PlaylistGUID string              `json:"playlist_guid"` // 目标歌单 GUID（若为空则自动新建同名歌单）
	Songs        []playlist.SongItem `json:"songs"`         // 待导入歌曲列表
}

// ImportMatchItem 单曲匹配详情
type ImportMatchItem struct {
	Index         int     `json:"index"`
	SongName      string  `json:"song_name"`
	Artist        string  `json:"artist"`
	Matched       bool    `json:"matched"`
	TrackGUID     string  `json:"track_guid,omitempty"`
	MatchedTitle  string  `json:"matched_title,omitempty"`
	MatchedArtist string  `json:"matched_artist,omitempty"`
	Score         float64 `json:"score"`
	Error         string  `json:"error,omitempty"`
}

// ImportPlaylistResult 歌单导入综合报告
type ImportPlaylistResult struct {
	PlaylistGUID   string              `json:"playlist_guid"`
	PlaylistName   string              `json:"playlist_name"`
	Total          int                 `json:"total"`
	MatchedCount   int                 `json:"matched_count"`
	UnmatchedCount int                 `json:"unmatched_count"`
	Results        []ImportMatchItem   `json:"results"`
	UnmatchedSongs []playlist.SongItem `json:"unmatched_songs"` // 未匹配歌曲清单（便于一键复制补全）
}
