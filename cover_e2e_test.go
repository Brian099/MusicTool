package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"music-toolkit/internal/config"
	"music-toolkit/internal/database"
	"music-toolkit/internal/server"
)

// 最小 PNG 头，用于封面图床
var e2ePNG = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}

// TestImportPlaylistCoverEndToEnd 走完整 HTTP 链路：
// 登录 → 连接（mock）飞牛 → 导入歌单（带 cover_url）→ 校验封面下载、上传与建歌单时携带 coverId
func TestImportPlaylistCoverEndToEnd(t *testing.T) {
	var coverUploaded bool
	var coverSize int
	var createBody string

	// ---- 模拟飞牛 NAS ----
	fnMux := http.NewServeMux()
	fnMux.HandleFunc("/music/api/v1/user/password-login", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"userToken": "mock-token", "username": "tester"},
		})
	})
	fnMux.HandleFunc("/music/api/v1/search/track", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"total": 0, "list": []any{}},
		})
	})
	fnMux.HandleFunc("/music/api/v1/static/cover/playlist", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Errorf("封面表单解析失败: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("缺少 file 字段: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		coverUploaded = true
		coverSize = len(data)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"coverId": "playlist_e2e_cover"},
		})
	})
	fnMux.HandleFunc("/music/api/v1/playlist/create", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		createBody = string(body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"guid": "pl-e2e-1", "name": "网易云热歌榜"},
		})
	})
	fnMux.HandleFunc("/music/api/v1/playlist/add-track", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
	})
	// 封面图床：故意把 PNG 标成 image/jpg，验证实现按魔数判定
	fnMux.HandleFunc("/cover.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpg")
		_, _ = w.Write(e2ePNG)
	})
	fnServer := httptest.NewServer(fnMux)
	defer fnServer.Close()

	// ---- 被测服务 ----
	tempDir := t.TempDir()
	db, err := database.OpenDB(filepath.Join(tempDir, "e2e.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		MusicDir:  tempDir,
		OutputDir: filepath.Join(tempDir, "out"),
		Port:      6829,
	}
	handler := server.NewServer(cfg, db, GetFrontendFileSystem()).Handler()
	appServer := httptest.NewServer(handler)
	defer appServer.Close()

	// 1. 初始化管理员并取得 token
	initRec := doJSON(t, appServer.URL+"/api/auth/init", "", `{"username":"admin","password":"pass1234"}`)
	var initResp struct {
		Success bool   `json:"success"`
		Token   string `json:"token"`
	}
	mustUnmarshal(t, initRec, &initResp)
	if !initResp.Success || initResp.Token == "" {
		t.Fatalf("初始化管理员失败: %s", initRec)
	}

	// 2. 连接 mock 飞牛 NAS
	connectBody := `{"server_url":"` + fnServer.URL + `","username":"tester","password":"123456"}`
	connectRec := doJSON(t, appServer.URL+"/api/feiniu/connect", initResp.Token, connectBody)
	if !strings.Contains(connectRec, `"success":true`) {
		t.Fatalf("连接飞牛失败: %s", connectRec)
	}

	// 3. 导入歌单，携带封面地址
	importBody := `{
		"name":"网易云热歌榜",
		"cover_url":"` + fnServer.URL + `/cover.jpg",
		"songs":[{"index":1,"song_name":"不存在的歌曲","artist":"不存在的歌手"}]
	}`
	importRec := doJSON(t, appServer.URL+"/api/feiniu/playlist/import", initResp.Token, importBody)

	var importResp struct {
		Success bool `json:"success"`
		Data    struct {
			PlaylistGUID  string `json:"playlist_guid"`
			CoverUploaded bool   `json:"cover_uploaded"`
			CoverError    string `json:"cover_error"`
		} `json:"data"`
	}
	mustUnmarshal(t, importRec, &importResp)

	if !importResp.Success {
		t.Fatalf("导入失败: %s", importRec)
	}
	if !importResp.Data.CoverUploaded {
		t.Fatalf("cover_uploaded 应为 true (cover_error=%q, resp=%s)", importResp.Data.CoverError, importRec)
	}
	if !coverUploaded {
		t.Fatalf("未收到封面上传请求")
	}
	if coverSize != len(e2ePNG) {
		t.Fatalf("上传的封面大小不符: %d", coverSize)
	}
	if !strings.Contains(createBody, "playlist_e2e_cover") {
		t.Fatalf("创建歌单未携带 coverId: %s", createBody)
	}
	if importResp.Data.PlaylistGUID != "pl-e2e-1" {
		t.Fatalf("歌单 GUID 不符: %s", importResp.Data.PlaylistGUID)
	}
}

func doJSON(t *testing.T, url, token, body string) string {
	t.Helper()
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return string(data)
}

func mustUnmarshal(t *testing.T, raw string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		t.Fatalf("解析响应失败: %v (raw=%s)", err, raw)
	}
}
