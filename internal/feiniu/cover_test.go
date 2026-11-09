package feiniu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"music-toolkit/internal/database"
)

// 最小合法 PNG / JPEG 头，用于魔数判定测试
var (
	pngHeader = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	jpgHeader = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
)

func TestCoverImageKind(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
		ok   bool
	}{
		{"png", pngHeader, "png", true},
		{"jpg", jpgHeader, "jpg", true},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "webp", true},
		{"gif", []byte("GIF89a......"), "gif", true},
		{"html", []byte("<!DOCTYPE html><html>403</html>"), "", false},
		{"empty", nil, "", false},
	}
	for _, c := range cases {
		ext, ok := coverImageKind(c.data)
		if ok != c.ok || ext != c.want {
			t.Fatalf("%s: got (%q,%v), want (%q,%v)", c.name, ext, ok, c.want, c.ok)
		}
	}
}

func TestDownloadCoverImage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/good.jpg", func(w http.ResponseWriter, r *http.Request) {
		// 故意用错误的 Content-Type，验证实现只信魔数
		w.Header().Set("Content-Type", "image/jpg")
		_, _ = w.Write(pngHeader)
	})
	mux.HandleFunc("/blocked.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>403 Forbidden</html>"))
	})
	mux.HandleFunc("/missing.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx := context.Background()

	data, ext, err := downloadCoverImage(ctx, server.URL+"/good.jpg")
	if err != nil {
		t.Fatalf("下载有效封面失败: %v", err)
	}
	if ext != "png" {
		t.Fatalf("应按魔数判定为 png，实际 %q", ext)
	}
	if len(data) != len(pngHeader) {
		t.Fatalf("封面数据长度不符: %d", len(data))
	}

	if _, _, err := downloadCoverImage(ctx, server.URL+"/blocked.jpg"); err == nil {
		t.Fatalf("防盗链 HTML 应返回错误")
	} else if !strings.Contains(err.Error(), "不是有效图片") {
		t.Fatalf("错误信息未提示非图片: %v", err)
	}

	if _, _, err := downloadCoverImage(ctx, server.URL+"/missing.jpg"); err == nil {
		t.Fatalf("HTTP 404 应返回错误")
	}

	if _, _, err := downloadCoverImage(ctx, ""); err == nil {
		t.Fatalf("空地址应返回错误")
	}
}

func TestUploadPlaylistCover(t *testing.T) {
	var gotField string
	var gotCookie string
	var gotBodySize int

	mux := http.NewServeMux()
	mux.HandleFunc("/music/api/v1/user/password-login", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(FeiNiuResponse[LoginData]{
			Code: 0,
			Data: LoginData{UserToken: "mock-token-cover", Username: "tester"},
		})
	})
	mux.HandleFunc("/music/api/v1/static/cover/playlist", func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Errorf("解析 multipart 失败: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("缺少 file 字段: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		gotField = header.Filename
		body, _ := io.ReadAll(file)
		gotBodySize = len(body)

		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg":  "ok",
			"data": map[string]any{"coverId": "playlist_abc123"},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	db, err := database.OpenDB(filepath.Join(t.TempDir(), "cover.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer db.Close()

	client := NewClient(db)
	ctx := context.Background()
	if _, err := client.SetAuth(ctx, server.URL, "tester", "123456", ""); err != nil {
		t.Fatalf("SetAuth failed: %v", err)
	}

	coverID, err := client.UploadPlaylistCover(ctx, pngHeader, "playlist_cover.png")
	if err != nil {
		t.Fatalf("上传封面失败: %v", err)
	}
	if coverID != "playlist_abc123" {
		t.Fatalf("coverId 不符: %s", coverID)
	}
	if gotField != "playlist_cover.png" {
		t.Fatalf("表单字段文件名不符: %q", gotField)
	}
	if gotBodySize != len(pngHeader) {
		t.Fatalf("上传内容长度不符: %d", gotBodySize)
	}
	if !strings.Contains(gotCookie, "music-token=mock-token-cover") {
		t.Fatalf("上传请求未携带鉴权 Cookie: %q", gotCookie)
	}

	if _, err := client.UploadPlaylistCover(ctx, nil, "x.png"); err == nil {
		t.Fatalf("空图片应返回错误")
	}
}
