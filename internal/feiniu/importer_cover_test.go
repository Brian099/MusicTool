package feiniu

import (
	"context"
	"strings"
	"testing"

	"music-toolkit/internal/playlist"
)

// TestImportPlaylistUploadsCover 验证导入链路：下载原歌单封面 → 上传飞牛 → 建歌单时带上 coverId
func TestImportPlaylistUploadsCover(t *testing.T) {
	var uploadedCover bool
	var createBody string

	mux := newTestMux(t, &uploadedCover, &createBody)
	server := newTestServer(mux)
	defer server.Close()

	client := newAuthedClient(t, server.URL)
	importer := NewImporter(client)

	result, err := importer.ImportPlaylist(context.Background(), ImportPlaylistRequest{
		Name:     "网易云热歌榜",
		CoverURL: server.URL + "/cover.jpg",
		Songs: []playlist.SongItem{
			{Index: 1, SongName: "不存在的歌曲", Artist: "不存在的歌手"},
		},
	})
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if !uploadedCover {
		t.Fatalf("未调用封面上传接口")
	}
	if !result.CoverUploaded {
		t.Fatalf("结果未标记封面已上传 (err=%q)", result.CoverError)
	}
	if result.CoverError != "" {
		t.Fatalf("不应有封面错误: %s", result.CoverError)
	}
	if !strings.Contains(createBody, "playlist_mock_cover") {
		t.Fatalf("创建歌单请求未携带 coverId，body=%s", createBody)
	}
	if result.PlaylistGUID != "pl-new-1" {
		t.Fatalf("歌单 GUID 异常: %s", result.PlaylistGUID)
	}
}

// TestImportPlaylistCoverFailureDoesNotBreakImport 封面下载失败时，歌单导入仍应成功
func TestImportPlaylistCoverFailureDoesNotBreakImport(t *testing.T) {
	var uploadedCover bool
	var createBody string

	mux := newTestMux(t, &uploadedCover, &createBody)
	server := newTestServer(mux)
	defer server.Close()

	client := newAuthedClient(t, server.URL)
	importer := NewImporter(client)

	result, err := importer.ImportPlaylist(context.Background(), ImportPlaylistRequest{
		Name:     "封面失效的歌单",
		CoverURL: server.URL + "/blocked.jpg",
		Songs: []playlist.SongItem{
			{Index: 1, SongName: "不存在的歌曲", Artist: "不存在的歌手"},
		},
	})
	if err != nil {
		t.Fatalf("封面失败不应导致导入失败: %v", err)
	}
	if uploadedCover {
		t.Fatalf("封面无效时不应调用上传接口")
	}
	if result.CoverUploaded {
		t.Fatalf("封面无效时不应标记为已上传")
	}
	if result.CoverError == "" {
		t.Fatalf("应记录封面失败原因")
	}
	if strings.Contains(createBody, "playlist_mock_cover") {
		t.Fatalf("封面无效时不应传 coverId，body=%s", createBody)
	}
}

// TestImportPlaylistIntoExistingSkipsCover 导入到已有歌单时不上传封面（避免覆盖用户已有封面）
func TestImportPlaylistIntoExistingSkipsCover(t *testing.T) {
	var uploadedCover bool
	var createBody string

	mux := newTestMux(t, &uploadedCover, &createBody)
	server := newTestServer(mux)
	defer server.Close()

	client := newAuthedClient(t, server.URL)
	importer := NewImporter(client)

	result, err := importer.ImportPlaylist(context.Background(), ImportPlaylistRequest{
		PlaylistGUID: "pl-existing",
		CoverURL:     server.URL + "/cover.jpg",
		Songs: []playlist.SongItem{
			{Index: 1, SongName: "不存在的歌曲", Artist: "不存在的歌手"},
		},
	})
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if uploadedCover {
		t.Fatalf("导入到已有歌单时不应上传封面")
	}
	if result.CoverUploaded {
		t.Fatalf("导入到已有歌单时不应标记封面已上传")
	}
	if result.PlaylistGUID != "pl-existing" {
		t.Fatalf("应使用指定歌单 GUID，实际 %s", result.PlaylistGUID)
	}
}
