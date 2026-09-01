package feiniu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"music-toolkit/internal/database"
	"music-toolkit/internal/playlist"
)

func TestSHA256AndDeviceID(t *testing.T) {
	hash := SHA256("123456")
	if hash != "8d969eef6ecad3c29a3a629280e686cf0c3f5d5a86aff3ca12020c923adc6c92" {
		t.Fatalf("unexpected SHA256 hash: %s", hash)
	}

	devID := GenerateDeviceID()
	if len(devID) != 32 {
		t.Fatalf("unexpected device id length: %d", len(devID))
	}
}

func TestCleanSongTitleAndArtist(t *testing.T) {
	cases := []struct {
		input       string
		expected    string
		artistInput string
		expArtist   string
	}{
		{
			input:       "七里香 (2024 Remaster HD)",
			expected:    "七里香",
			artistInput: "周杰伦 / 费玉清",
			expArtist:   "周杰伦",
		},
		{
			input:       "晴天 - Live现场版",
			expected:    "晴天",
			artistInput: "周杰伦 feat. 五月天",
			expArtist:   "周杰伦",
		},
		{
			input:       "夜曲【伴奏】",
			expected:    "夜曲",
			artistInput: "周杰伦, 阿信",
			expArtist:   "周杰伦",
		},
	}

	for _, tc := range cases {
		cleaned := CleanSongTitle(tc.input)
		if cleaned != tc.expected {
			t.Errorf("CleanSongTitle(%q) = %q; want %q", tc.input, cleaned, tc.expected)
		}
		pArtist := ExtractPrimaryArtist(tc.artistInput)
		if pArtist != tc.expArtist {
			t.Errorf("ExtractPrimaryArtist(%q) = %q; want %q", tc.artistInput, pArtist, tc.expArtist)
		}
	}
}

func TestScoreCandidate(t *testing.T) {
	song := playlist.SongItem{
		SongName: "晴天 (Live)",
		Artist:   "周杰伦 / 蔡依林",
	}

	trackMatch := FeiNiuTrack{
		GUID:  "guid-1",
		Title: "晴天",
		Artists: []FeiNiuArtist{
			{GUID: "art-1", Name: "周杰伦"},
		},
		AudioSpec: FeiNiuAudioSpec{
			Format:  "flac",
			Bitrate: 900000,
		},
	}

	score := ScoreCandidate(song, trackMatch)
	if score < 0.80 {
		t.Fatalf("ScoreCandidate expected >= 0.80, got %f", score)
	}
}

func TestClientMockServerAndAutoRelogin(t *testing.T) {
	var loginCount int32
	var reqCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)

		if r.URL.Path == "/music/api/v1/user/password-login" {
			atomic.AddInt32(&loginCount, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(FeiNiuResponse[LoginData]{
				Code: 0,
				Data: LoginData{
					UserToken: "mock-token-123",
					Username:  "testuser",
				},
			})
			return
		}

		if r.URL.Path == "/music/api/v1/playlist/list" {
			cookie := r.Header.Get("Cookie")
			// 第一次请求返回 401 模拟 token 过期
			if atomic.LoadInt32(&reqCount) == 2 {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(FeiNiuResponse[any]{
					Code: 401,
					Msg:  "INVALID TOKEN",
				})
				return
			}

			if cookie != "music-token=mock-token-123" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(FeiNiuResponse[FeiNiuPageData[FeiNiuPlaylist]]{
				Code: 0,
				Data: FeiNiuPageData[FeiNiuPlaylist]{
					Total: 1,
					List: []FeiNiuPlaylist{
						{GUID: "pl-1", Name: "我的最爱", TrackCount: 10},
					},
				},
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	db, err := database.OpenDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer db.Close()

	client := NewClient(db)

	ctx := context.Background()
	loginData, err := client.SetAuth(ctx, server.URL, "testuser", "123456", "")
	if err != nil {
		t.Fatalf("SetAuth failed: %v", err)
	}
	if loginData.UserToken != "mock-token-123" {
		t.Fatalf("unexpected token: %s", loginData.UserToken)
	}

	// 第一次调用 GetPlaylists 会命中 401 并自动触发 ensureLogin 重新重试
	playlists, err := client.GetPlaylists(ctx, 1, 10)
	if err != nil {
		t.Fatalf("GetPlaylists failed: %v", err)
	}
	if playlists.Total != 1 || len(playlists.List) != 1 {
		t.Fatalf("unexpected playlist list: %+v", playlists)
	}

	if atomic.LoadInt32(&loginCount) != 2 {
		t.Fatalf("expected 2 logins (initial + auto relogin), got %d", loginCount)
	}
}
