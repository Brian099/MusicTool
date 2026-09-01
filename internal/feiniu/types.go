package feiniu

import "music-toolkit/internal/playlist"

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

// FeiNiuArtist 歌手信息
type FeiNiuArtist struct {
	GUID string `json:"guid"`
	Name string `json:"name"`
}

// FeiNiuAudioSpec 音频规格
type FeiNiuAudioSpec struct {
	Format     string  `json:"format"`
	Bitrate    int     `json:"bitrate"`
	SampleRate int     `json:"sampleRate"`
	Channels   int     `json:"channels"`
	Duration   float64 `json:"duration"`
	Path       string  `json:"path"`
}

// FeiNiuTrack 飞牛单曲模型
type FeiNiuTrack struct {
	GUID        string          `json:"guid"`
	Title       string          `json:"title"`
	ArtistGUIDs []string        `json:"artistGUIDs"`
	Artists     []FeiNiuArtist  `json:"artists"`
	Album       string          `json:"album"`
	AlbumGUID   string          `json:"albumGUID"`
	CoverID     string          `json:"coverId"`
	Duration    float64         `json:"duration"`
	AudioSpec   FeiNiuAudioSpec `json:"audioSpec"`
	CreatedAt   int64           `json:"createdAt"`
	UpdatedAt   int64           `json:"updatedAt"`
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
