package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"music-toolkit/internal/database"
)

// TestPlaylistHistoryCoverPersistence 历史记录必须能存取歌单封面地址，
// 否则从历史载入后无法带封面导入飞牛。
func TestPlaylistHistoryCoverPersistence(t *testing.T) {
	dir := t.TempDir()
	db, err := database.OpenDB(filepath.Join(dir, "hist.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	const cover = "https://p1.music.126.net/abc.jpg"

	id, err := db.SavePlaylistHistory(ctx, &database.PlaylistHistoryRecord{
		Platform:  "netease",
		SourceURL: "https://music.163.com/playlist?id=1",
		Title:     "热歌榜",
		CoverURL:  cover,
		SongCount: 1,
		SongsJSON: `[{"index":1,"song_name":"a","artist":"b","full_text":"a - b"}]`,
		CreatedAt: 1700000000,
	})
	if err != nil {
		t.Fatalf("save history: %v", err)
	}

	rec, err := db.GetPlaylistHistory(ctx, id)
	if err != nil || rec == nil {
		t.Fatalf("get history: %v", err)
	}
	if rec.CoverURL != cover {
		t.Fatalf("详情封面不符: %q", rec.CoverURL)
	}

	list, err := db.ListPlaylistHistory(ctx, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("list history: %v (len=%d)", err, len(list))
	}
	if list[0].CoverURL != cover {
		t.Fatalf("列表封面不符: %q", list[0].CoverURL)
	}
}

// TestPlaylistHistorySchemaMigration 老版本数据库（无 cover_url 列）打开时应自动补列，
// 否则升级后读写历史会直接报错。
func TestPlaylistHistorySchemaMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	_, err = raw.Exec(`CREATE TABLE playlist_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT,
		source_url TEXT,
		title TEXT,
		song_count INTEGER,
		songs_json TEXT,
		created_at INTEGER
	)`)
	if err != nil {
		raw.Close()
		t.Fatalf("create legacy table: %v", err)
	}
	_, err = raw.Exec(`INSERT INTO playlist_history (platform, source_url, title, song_count, songs_json, created_at)
		VALUES ('qq', 'u', '旧记录', 0, '[]', 1600000000)`)
	if err != nil {
		raw.Close()
		t.Fatalf("seed legacy row: %v", err)
	}
	raw.Close()

	// OpenDB 应执行轻量迁移补齐 cover_url
	db, err := database.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB 迁移失败: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	list, err := db.ListPlaylistHistory(ctx, 10)
	if err != nil {
		t.Fatalf("迁移后读取历史失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("旧记录数量不符: %d", len(list))
	}
	if list[0].Title != "旧记录" {
		t.Fatalf("旧记录内容异常: %+v", list[0])
	}
	if list[0].CoverURL != "" {
		t.Fatalf("旧记录封面应为空: %q", list[0].CoverURL)
	}

	// 迁移后写入带封面的新记录应成功
	id, err := db.SavePlaylistHistory(ctx, &database.PlaylistHistoryRecord{
		Platform: "netease", Title: "新记录", CoverURL: "https://x/y.jpg", SongsJSON: "[]",
	})
	if err != nil {
		t.Fatalf("迁移后写入失败: %v", err)
	}
	rec, _ := db.GetPlaylistHistory(ctx, id)
	if rec == nil || rec.CoverURL != "https://x/y.jpg" {
		t.Fatalf("迁移后封面未正确保存: %+v", rec)
	}
}
