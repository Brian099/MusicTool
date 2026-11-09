package feiniu

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"music-toolkit/internal/database"
)

// newTestMux 构造模拟飞牛 NAS 的最小接口集合：
// 登录、曲库搜索（返回空）、封面上传、歌单创建、歌曲写入。
func newTestMux(t *testing.T, uploadedCover *bool, createBody *string) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/music/api/v1/user/password-login", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(FeiNiuResponse[LoginData]{
			Code: 0,
			Data: LoginData{UserToken: "mock-token-import", Username: "tester"},
		})
	})

	// 曲库搜索返回空结果，保证所有歌曲走"未匹配"分支，不触发真实写入
	mux.HandleFunc("/music/api/v1/search/track", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(FeiNiuResponse[FeiNiuPageData[FeiNiuTrack]]{
			Code: 0,
			Data: FeiNiuPageData[FeiNiuTrack]{Total: 0, List: []FeiNiuTrack{}},
		})
	})

	mux.HandleFunc("/music/api/v1/static/cover/playlist", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Errorf("封面上传表单解析失败: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, _, err := r.FormFile("file"); err != nil {
			t.Errorf("封面上传缺少 file 字段: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		*uploadedCover = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"coverId": "playlist_mock_cover"},
		})
	})

	mux.HandleFunc("/music/api/v1/playlist/create", func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		*createBody = string(buf)
		_ = json.NewEncoder(w).Encode(FeiNiuResponse[FeiNiuPlaylist]{
			Code: 0,
			Data: FeiNiuPlaylist{GUID: "pl-new-1", Name: "网易云热歌榜"},
		})
	})

	mux.HandleFunc("/music/api/v1/playlist/add-track", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
	})

	// 可用的封面图（内容是 PNG，但故意标错 Content-Type）
	mux.HandleFunc("/cover.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpg")
		_, _ = w.Write(pngHeader)
	})

	// 防盗链返回的 HTML 错误页
	mux.HandleFunc("/blocked.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>403 Forbidden</html>"))
	})

	return mux
}

func newTestServer(mux *http.ServeMux) *httptest.Server {
	return httptest.NewServer(mux)
}

func newAuthedClient(t *testing.T, serverURL string) *Client {
	db, err := database.OpenDB(filepath.Join(t.TempDir(), "import.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	client := NewClient(db)
	if _, err := client.SetAuth(t.Context(), serverURL, "tester", "123456", ""); err != nil {
		t.Fatalf("SetAuth failed: %v", err)
	}
	return client
}
