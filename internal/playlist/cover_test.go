package playlist

import (
	"encoding/json"
	"testing"
)

func TestNormalizeCoverURL(t *testing.T) {
	cases := map[string]string{
		"":                                            "",
		"  https://p1.music.126.net/a.jpg  ":          "https://p1.music.126.net/a.jpg",
		"//p1.music.126.net/a.jpg":                    "https://p1.music.126.net/a.jpg",
		"https://p1.music.126.net/a.jpg?param=300y300": "https://p1.music.126.net/a.jpg",
		"https://y.gtimg.cn/music/photo_new/a.jpg":    "https://y.gtimg.cn/music/photo_new/a.jpg",
	}
	for in, want := range cases {
		if got := normalizeCoverURL(in); got != want {
			t.Fatalf("normalizeCoverURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNeteaseCoverField 校验网易云响应中封面字段名映射正确（字段改名会直接导致封面丢失）
func TestNeteaseCoverField(t *testing.T) {
	raw := `{
		"code": 200,
		"playlist": {
			"id": 3778678,
			"name": "热歌榜",
			"coverImgUrl": "https://p1.music.126.net/0SUEG8yDACfx0Bw2MYFv4Q==/109951170048519512.jpg",
			"trackCount": 2,
			"trackIds": [{"id": 1}, {"id": 2}]
		}
	}`
	var resp neteasePlaylistResp
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal 失败: %v", err)
	}
	if resp.Playlist.CoverImgURL == "" {
		t.Fatalf("未解析出 coverImgUrl")
	}
	if got := normalizeCoverURL(resp.Playlist.CoverImgURL); got != "https://p1.music.126.net/0SUEG8yDACfx0Bw2MYFv4Q==/109951170048519512.jpg" {
		t.Fatalf("封面地址不符: %s", got)
	}
}

// TestQQCoverField 校验 QQ 音乐 dirinfo.picurl 字段映射
func TestQQCoverField(t *testing.T) {
	raw := `{
		"code": 0,
		"req_0": {
			"code": 0,
			"data": {
				"dirinfo": {
					"title": "我的歌单",
					"songnum": 1,
					"picurl": "https://y.gtimg.cn/music/photo_new/T002R500x500M000003YQu7349cT7g.jpg",
					"picurl2": ""
				},
				"songlist": [{"name": "测试曲", "singer": [{"name": "歌手"}]}]
			}
		}
	}`
	var resp qqMusicFullResp
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal 失败: %v", err)
	}
	if resp.Req0.Data.Dirinfo.Picurl == "" {
		t.Fatalf("未解析出 dirinfo.picurl")
	}
	if resp.Req0.Data.Dirinfo.Title != "我的歌单" {
		t.Fatalf("标题解析异常: %s", resp.Req0.Data.Dirinfo.Title)
	}
}

// TestQQCoverFieldEmptyForToplist 榜单类响应的 picurl 为空，不应报错
func TestQQCoverFieldEmptyForToplist(t *testing.T) {
	raw := `{"code":0,"req_0":{"code":0,"data":{"dirinfo":{"title":"","songnum":0,"picurl":""},"songlist":[]}}}`
	var resp qqMusicFullResp
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal 失败: %v", err)
	}
	if got := normalizeCoverURL(resp.Req0.Data.Dirinfo.Picurl); got != "" {
		t.Fatalf("榜单封面应为空，实际 %q", got)
	}
}
