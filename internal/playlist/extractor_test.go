package playlist

import (
	"testing"
)

func TestCleanSongName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"晴天 (Live)", "晴天"},
		{"夜曲 [2005 专辑版]", "夜曲"},
		{"稻香（feat. 群星）", "稻香"},
		{"七里香【无损立体声】", "七里香"},
		{"普通歌曲名", "普通歌曲名"},
	}

	for _, tt := range tests {
		actual := cleanSongName(tt.input)
		if actual != tt.expected {
			t.Errorf("cleanSongName(%q) = %q, expected %q", tt.input, actual, tt.expected)
		}
	}
}

func TestFormatSongItems(t *testing.T) {
	songs := []SongItem{
		{Index: 1, SongName: "晴天", Artist: "周杰伦"},
		{Index: 2, SongName: "夜曲", Artist: "周杰伦"},
	}

	// 1. song-singer (default)
	formatSongItems(songs, "song-singer")
	if songs[0].FullText != "晴天 - 周杰伦" {
		t.Errorf("expected '晴天 - 周杰伦', got %q", songs[0].FullText)
	}

	// 2. singer-song
	formatSongItems(songs, "singer-song")
	if songs[0].FullText != "周杰伦 - 晴天" {
		t.Errorf("expected '周杰伦 - 晴天', got %q", songs[0].FullText)
	}

	// 3. song only
	formatSongItems(songs, "song")
	if songs[0].FullText != "晴天" {
		t.Errorf("expected '晴天', got %q", songs[0].FullText)
	}
}

func TestExtractNetEaseID(t *testing.T) {
	tests := []struct {
		url      string
		expected int64
	}{
		{"https://music.163.com/#/playlist?id=24381616", 24381616},
		{"https://music.163.com/playlist?id=12345678", 12345678},
	}

	for _, tt := range tests {
		id, err := extractNetEaseID(tt.url)
		if err != nil || id != tt.expected {
			t.Errorf("extractNetEaseID(%q) = %d (err: %v), expected %d", tt.url, id, err, tt.expected)
		}
	}
}

func TestExtractQQPlaylistID(t *testing.T) {
	tests := []struct {
		url      string
		expected int64
	}{
		{"https://y.qq.com/n/ryqq/playlist/789101112", 789101112},
		{"https://y.qq.com/n/ryqq/playlist?id=123456", 123456},
		{"https://i.y.qq.com/n2/m/share/details/taoge.html?id=888888", 888888},
	}

	for _, tt := range tests {
		id, err := extractQQPlaylistID(tt.url)
		if err != nil || id != tt.expected {
			t.Errorf("extractQQPlaylistID(%q) = %d (err: %v), expected %d", tt.url, id, err, tt.expected)
		}
	}
}

func TestQQMusicSign(t *testing.T) {
	sampleData := `{"req_0":{"module":"music.srfDissInfo.aiDissInfo","method":"uniform_get_Dissinfo","param":{"disstid":123456}}}`
	sign, err := getQQMusicSign(sampleData)
	if err != nil {
		t.Fatalf("getQQMusicSign failed: %v", err)
	}
	if len(sign) == 0 {
		t.Fatalf("getQQMusicSign returned empty sign")
	}
	t.Logf("Computed QQ sign: %s", sign)
}
