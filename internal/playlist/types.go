package playlist

// ParseRequest 歌单解析请求
type ParseRequest struct {
	URL         string `json:"url"`
	Detailed    bool   `json:"detailed"`     // 是否保留歌曲原名括号等详细信息
	Format      string `json:"format"`       // song-singer (默认), singer-song, song
	Order       string `json:"order"`        // default, reverse
	SaveHistory bool   `json:"save_history"` // 是否存入 SQLite 历史
}

// SongItem 解析出的单曲信息
type SongItem struct {
	Index    int    `json:"index"`
	SongName string `json:"song_name"`
	Artist   string `json:"artist"`
	FullText string `json:"full_text"` // 格式化后的完整显示文本
}

// ParseResult 歌单解析结果
type ParseResult struct {
	Platform   string     `json:"platform"` // netease, qq, qishui
	SourceURL  string     `json:"source_url"`
	Title      string     `json:"title"`
	SongCount  int        `json:"song_count"`
	Songs      []SongItem `json:"songs"`
	TextList   []string   `json:"text_list"` // 纯文本列表（便于直接复制）
	RawText    string     `json:"raw_text"`  // 换行分隔的纯文本
	HistoryID  int64      `json:"history_id,omitempty"`
	ParseTime  float64    `json:"parse_time"` // 解析耗时（秒）
}

// HistoryItem 历史记录条目
type HistoryItem struct {
	ID        int64      `json:"id"`
	Platform  string     `json:"platform"`
	SourceURL string     `json:"source_url"`
	Title     string     `json:"title"`
	SongCount int        `json:"song_count"`
	Songs     []SongItem `json:"songs,omitempty"`
	CreatedAt int64      `json:"created_at"`
}
