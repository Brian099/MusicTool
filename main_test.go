package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"music-toolkit/internal/config"
	"music-toolkit/internal/database"
	"music-toolkit/internal/deduplicator"
	"music-toolkit/internal/detector"
	"music-toolkit/internal/fileop"
	"music-toolkit/internal/server"
)

func TestMagicBytesDetection(t *testing.T) {
	// FLAC
	flacHdr := []byte("fLaC\x00\x00\x00")
	fmtKey, _, ok := detector.DetectByMagicBytes(flacHdr)
	if !ok || fmtKey != "flac" {
		t.Fatalf("expected flac, got %s (ok: %v)", fmtKey, ok)
	}

	// HTML 伪装
	htmlHdr := []byte("<!DOCTYPE html><html><head><title>403 Forbidden</title></head></html>")
	fmtKey, _, ok = detector.DetectByMagicBytes(htmlHdr)
	if !ok || fmtKey != "corrupt_or_text" {
		t.Fatalf("expected corrupt_or_text, got %s (ok: %v)", fmtKey, ok)
	}

	// M4A
	m4aHdr := []byte("\x00\x00\x00\x20ftypM4A \x00\x00\x02\x00isomiso2")
	fmtKey, _, ok = detector.DetectByMagicBytes(m4aHdr)
	if !ok || fmtKey != "m4a" {
		t.Fatalf("expected m4a, got %s (ok: %v)", fmtKey, ok)
	}
}

func TestSongloftDurationClustering(t *testing.T) {
	// 1. 时长相差在 30s 内 -> 聚类为 1 组
	closeSongs := []database.AudioFingerprintRecord{
		{FilePath: "/a.mp3", Duration: 210.0},
		{FilePath: "/b.m4a", Duration: 212.5},
	}
	clusters := deduplicator.ClusterByFingerprintDuration(closeSongs, 30.0)
	if len(clusters) != 1 || len(clusters[0]) != 2 {
		t.Fatalf("expected 1 cluster of 2 songs, got %d", len(clusters))
	}

	// 2. 时长相差大于 30s (片段 vs 全曲) -> 拆分且不足 2 首被丢弃
	farSongs := []database.AudioFingerprintRecord{
		{FilePath: "/short.mp3", Duration: 60.0},
		{FilePath: "/full.flac", Duration: 240.0},
	}
	clustersFar := deduplicator.ClusterByFingerprintDuration(farSongs, 30.0)
	if len(clustersFar) != 0 {
		t.Fatalf("expected 0 clusters, got %d", len(clustersFar))
	}

	// 3. 链式连续相近分布 [200, 215, 230] (相邻 <= 30s) -> 聚合为 1 组
	chainSongs := []database.AudioFingerprintRecord{
		{FilePath: "/1.mp3", Duration: 200.0},
		{FilePath: "/2.mp3", Duration: 215.0},
		{FilePath: "/3.mp3", Duration: 230.0},
	}
	clustersChain := deduplicator.ClusterByFingerprintDuration(chainSongs, 30.0)
	if len(clustersChain) != 1 || len(clustersChain[0]) != 3 {
		t.Fatalf("expected 1 cluster of 3 songs, got %d", len(clustersChain))
	}

	// 4. 未知时长 0 -> 保守不拆分
	unknownSongs := []database.AudioFingerprintRecord{
		{FilePath: "/good.mp3", Duration: 210.0},
		{FilePath: "/unknown.aac", Duration: 0.0},
	}
	clustersUnknown := deduplicator.ClusterByFingerprintDuration(unknownSongs, 30.0)
	if len(clustersUnknown) != 1 || len(clustersUnknown[0]) != 2 {
		t.Fatalf("expected 1 cluster of 2 songs with unknown duration, got %d", len(clustersUnknown))
	}
}

func TestSmartSongScoring(t *testing.T) {
	flacSong := deduplicator.SongItem{
		FilePath:   "/music/song.flac",
		Format:     "flac",
		Bitrate:    900000,
		SampleRate: 44100,
		Title:      "Song",
	}
	mp3Song := deduplicator.SongItem{
		FilePath:   "/music/song.mp3",
		Format:     "mp3",
		Bitrate:    320000,
		SampleRate: 44100,
		Title:      "Song",
	}
	scoreFlac := deduplicator.ScoreSong(flacSong)
	scoreMp3 := deduplicator.ScoreSong(mp3Song)
	if scoreFlac <= scoreMp3 {
		t.Fatalf("lossless flac score (%d) should be greater than mp3 score (%d)", scoreFlac, scoreMp3)
	}
}

func TestDatabaseAndFileOperations(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "music_test_go_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	rec := &database.AudioFingerprintRecord{
		FilePath:    filepath.Join(tempDir, "song.mp3"),
		FileName:    "song.mp3",
		MTime:       12345.67,
		FileSize:    5000,
		Fingerprint: "AQABc90...",
		Duration:    210.5,
		Format:      "mp3",
		Bitrate:     320000,
	}
	if err := db.UpsertFingerprint(ctx, rec); err != nil {
		t.Fatalf("upsert fingerprint: %v", err)
	}

	cached, err := db.GetFingerprint(ctx, rec.FilePath)
	if err != nil || cached == nil || cached.Fingerprint != "AQABc90..." {
		t.Fatalf("failed to read cached fingerprint: %v", cached)
	}

	// 测试文件操作
	fileOp := fileop.NewFileOperator(tempDir, filepath.Join(tempDir, "output"))
	testFile := filepath.Join(tempDir, "fake.mp3")
	os.WriteFile(testFile, []byte("audio data"), 0644)

	// 原地重命名
	ok, newPath, msg := fileOp.RenameInPlace(testFile, ".m4a")
	if !ok || filepath.Ext(newPath) != ".m4a" {
		t.Fatalf("rename in place failed: %s, msg: %s", newPath, msg)
	}

	// 移入回收站
	ok, recyclePath, msg := fileOp.MoveToRecycleBin(newPath, "")
	if !ok || !strings.Contains(recyclePath, "_recycle_bin") {
		t.Fatalf("move to recycle failed: %s, msg: %s", recyclePath, msg)
	}
}

func TestServerRoutingInitialization(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "server_test_*")
	defer os.RemoveAll(tempDir)
	db, _ := database.OpenDB(filepath.Join(tempDir, "test.db"))
	defer db.Close()

	cfg := &config.Config{
		MusicDir:  tempDir,
		OutputDir: filepath.Join(tempDir, "out"),
		Port:      6826,
	}
	frontendFS := GetFrontendFileSystem()
	srv := server.NewServer(cfg, db, frontendFS)
	handler := srv.Handler()
	if handler == nil {
		t.Fatal("handler should not be nil")
	}

	// 测试无损数据存储与查询
	rec := &database.LosslessRecord{
		FilePath:       filepath.Join(tempDir, "test.flac"),
		FileName:       "test.flac",
		Format:         "flac",
		SampleRate:     44100,
		Bitrate:        900000,
		Duration:       210.5,
		Grade:          "true_lossless",
		GradeText:      "💎 真无损 (CD 音质)",
		CutoffFreqHz:   22050,
		HighFreqEnergy: 28.5,
		Confidence:     92,
		Details:        "测试通过",
		UpdatedAt:      123456,
	}
	if err := db.UpsertLosslessRecord(context.Background(), rec); err != nil {
		t.Fatalf("upsert lossless record failed: %v", err)
	}

	list, err := db.ListLosslessRecords(context.Background())
	if err != nil || len(list) != 1 || list[0].Grade != "true_lossless" {
		t.Fatalf("list lossless records failed: %v, list: %+v", err, list)
	}
}

func TestFormatActionsAPI(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "format_action_test_*")
	defer os.RemoveAll(tempDir)
	outDir := filepath.Join(tempDir, "output")
	db, _ := database.OpenDB(filepath.Join(tempDir, "test.db"))
	defer db.Close()

	cfg := &config.Config{
		MusicDir:  tempDir,
		OutputDir: outDir,
		Port:      6826,
	}
	frontendFS := GetFrontendFileSystem()
	srv := server.NewServer(cfg, db, frontendFS)
	_ = srv

	// 创建测试 FLAC 文件伪装成 .mp3
	flacHeader := []byte("fLaC\x00\x00\x00\x22\x10\x00\x10\x00")
	fakeFile := filepath.Join(tempDir, "fake_song.mp3")
	os.WriteFile(fakeFile, flacHeader, 0644)

	// 插入一条初始 format_record
	ctx := context.Background()
	db.UpsertFormatRecord(ctx, &database.FormatRecord{
		FilePath:       fakeFile,
		FileName:       "fake_song.mp3",
		CurrentExt:     ".mp3",
		DetectedFormat: "flac",
		SuggestedExt:   ".flac",
		IsMismatch:     1,
		IsAudio:        1,
		Details:        "FLAC 无损音频",
		Status:         "detected",
		UpdatedAt:      123456,
	})

	records, _ := db.ListFormatRecords(ctx)
	if len(records) != 1 || records[0].IsMismatch != 1 {
		t.Fatalf("expected 1 mismatch record, got %d", len(records))
	}

	// 测试数据库 DeleteFormatRecord
	if err := db.DeleteFormatRecord(ctx, fakeFile); err != nil {
		t.Fatalf("DeleteFormatRecord failed: %v", err)
	}
	recordsAfter, _ := db.ListFormatRecords(ctx)
	if len(recordsAfter) != 0 {
		t.Fatalf("expected 0 records after delete, got %d", len(recordsAfter))
	}
}

func TestFeiNiuDatabaseAndServerAPI(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "feiniu_test_*")
	defer os.RemoveAll(tempDir)
	db, err := database.OpenDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// 1. 初始应无配置
	cfg, err := db.GetFeiNiuConfig(ctx)
	if err != nil || cfg != nil {
		t.Fatalf("expected nil config, got %v", cfg)
	}

	// 2. 保存配置
	now := int64(1700000000)
	err = db.SaveFeiNiuConfig(ctx, &database.FeiNiuConfigRecord{
		ID:           1,
		ServerURL:    "http://172.17.0.1:5666",
		Username:     "admin",
		PasswordHash: "mock-hash",
		DeviceID:     "dev-123",
		AccessCode:   "code-456",
		UserToken:    "token-789",
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("SaveFeiNiuConfig failed: %v", err)
	}

	// 3. 读取配置
	savedCfg, err := db.GetFeiNiuConfig(ctx)
	if err != nil || savedCfg == nil {
		t.Fatalf("GetFeiNiuConfig failed: %v", err)
	}
	if savedCfg.Username != "admin" || savedCfg.UserToken != "token-789" {
		t.Fatalf("unexpected saved config: %+v", savedCfg)
	}

	// 4. 清除配置
	if err := db.ClearFeiNiuConfig(ctx); err != nil {
		t.Fatalf("ClearFeiNiuConfig failed: %v", err)
	}
	clearedCfg, _ := db.GetFeiNiuConfig(ctx)
	if clearedCfg != nil {
		t.Fatalf("expected nil config after clear, got %+v", clearedCfg)
	}
}

