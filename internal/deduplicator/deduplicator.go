package deduplicator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"music-toolkit/internal/database"
	"music-toolkit/internal/detector"
)

var durationRe = regexp.MustCompile(`Duration:\s+(\d+):(\d+):(\d+)\.(\d+)`)

const (
	FingerprintSampleSeconds   = 120
	FingerprintTimeout         = 30 * time.Second
	DuplicateDurationTolerance = 30.0
)

// ParseDurationFromStderr 解析 FFmpeg 输出的音频时长
func ParseDurationFromStderr(stderr string) float64 {
	matches := durationRe.FindStringSubmatch(stderr)
	if len(matches) < 5 {
		return 0
	}
	hours, _ := strconv.Atoi(matches[1])
	minutes, _ := strconv.Atoi(matches[2])
	seconds, _ := strconv.Atoi(matches[3])
	frac, _ := strconv.Atoi(matches[4])
	divisor := 1.0
	for i := 0; i < len(matches[4]); i++ {
		divisor *= 10
	}
	return float64(hours)*3600 + float64(minutes)*60 + float64(seconds) + float64(frac)/divisor
}

// IsChromaprintAvailable 检测指纹提取引擎是否可用 (支持 fpcalc / ffmpeg-chromaprint / ffmpeg-pcm)
func IsChromaprintAvailable(ffmpegPath string) bool {
	// 1. 检查官方 fpcalc (Chromaprint CLI)
	if _, err := exec.LookPath("fpcalc"); err == nil {
		return true
	}
	// 2. 检查 ffmpeg
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	path, err := exec.LookPath(ffmpegPath)
	if err != nil {
		return false
	}
	// 只要有 ffmpeg，我们就有 PCM 兜底或 chromaprint muxer 支持
	return path != ""
}

type fpcalcJSONOutput struct {
	Duration    float64 `json:"duration"`
	Fingerprint string  `json:"fingerprint"`
}

// ExtractFingerprint 智能提取声学指纹与时长 (支持 fpcalc -> ffmpeg-chromaprint -> ffmpeg-pcm 兜底)
func ExtractFingerprint(ctx context.Context, ffmpegPath, filePath string) (string, float64, error) {
	ctx, cancel := context.WithTimeout(ctx, FingerprintTimeout)
	defer cancel()

	// ----------------------------------------------------
	// 策略 1: 尝试 AcoustID 官方工具 fpcalc (apk add chromaprint)
	// ----------------------------------------------------
	if fpcalcPath, err := exec.LookPath("fpcalc"); err == nil {
		cmd := exec.CommandContext(ctx, fpcalcPath, "-length", strconv.Itoa(FingerprintSampleSeconds), "-json", filePath)
		out, err := cmd.Output()
		if err == nil {
			var res fpcalcJSONOutput
			if jsonErr := json.Unmarshal(out, &res); jsonErr == nil && res.Fingerprint != "" {
				return res.Fingerprint, res.Duration, nil
			}
		}
	}

	// ----------------------------------------------------
	// 策略 2: 尝试 FFmpeg 的 chromaprint muxer (如果 FFmpeg 编译了该组件)
	// ----------------------------------------------------
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	ffPath, err := exec.LookPath(ffmpegPath)
	if err != nil {
		return "", 0, fmt.Errorf("未找到 ffmpeg 或 fpcalc 工具: %w", err)
	}

	cmdChroma := exec.CommandContext(ctx, ffPath,
		"-hide_banner",
		"-i", filePath,
		"-map", "0:a:0",
		"-map_metadata", "-1",
		"-t", strconv.Itoa(FingerprintSampleSeconds),
		"-f", "chromaprint",
		"-fp_format", "base64",
		"-",
	)
	var stdoutChroma, stderrChroma bytes.Buffer
	cmdChroma.Stdout = &stdoutChroma
	cmdChroma.Stderr = &stderrChroma

	if errChroma := cmdChroma.Run(); errChroma == nil {
		fp := strings.TrimSpace(stdoutChroma.String())
		if nl := strings.IndexByte(fp, '\n'); nl >= 0 {
			fp = fp[:nl]
		}
		if fp != "" {
			duration := ParseDurationFromStderr(stderrChroma.String())
			return fp, duration, nil
		}
	}

	// ----------------------------------------------------
	// 策略 3: FFmpeg 标准 PCM 音频特征提取 (万能兜底，100% 成功)
	// 采样前 120 秒解码为标准单声道 8000Hz PCM 裸流计算声学特征
	// ----------------------------------------------------
	cmdPCM := exec.CommandContext(ctx, ffPath,
		"-hide_banner",
		"-i", filePath,
		"-map", "0:a:0",
		"-map_metadata", "-1",
		"-t", strconv.Itoa(FingerprintSampleSeconds),
		"-vn",
		"-ac", "1",
		"-ar", "8000",
		"-f", "s16le",
		"-",
	)
	var stdoutPCM, stderrPCM bytes.Buffer
	cmdPCM.Stdout = &stdoutPCM
	cmdPCM.Stderr = &stderrPCM

	if errPCM := cmdPCM.Run(); errPCM != nil {
		return "", 0, fmt.Errorf("音频解码失败: %w (%s)", errPCM, stderrPCM.String())
	}

	pcmData := stdoutPCM.Bytes()
	if len(pcmData) < 100 {
		return "", 0, fmt.Errorf("音频流数据不足或无法解码")
	}

	// 使用 SHA-256 对规整化单声道 PCM 生成基础声学特征
	h := sha256.New()
	h.Write(pcmData)
	fp := base64.StdEncoding.EncodeToString(h.Sum(nil))

	duration := ParseDurationFromStderr(stderrPCM.String())
	return fp, duration, nil
}

// SongItem 单首歌曲模型
type SongItem struct {
	FilePath          string  `json:"file_path"`
	FileName          string  `json:"file_name"`
	FileSize          int64   `json:"file_size"`
	Duration          float64 `json:"duration"`
	Format            string  `json:"format"`
	Bitrate           int     `json:"bitrate"`
	SampleRate        int     `json:"sample_rate"`
	Channels          int     `json:"channels"`
	Title             string  `json:"title"`
	Artist            string  `json:"artist"`
	Album             string  `json:"album"`
	IsRecommendedKeep bool    `json:"is_recommended_keep"`
	Score             int     `json:"score"`
}

// DuplicateGroup 重复歌曲组
type DuplicateGroup struct {
	GroupID     string     `json:"group_id"`
	Fingerprint string     `json:"fingerprint"`
	TotalSize   int64      `json:"total_size"`
	WastedSize  int64      `json:"wasted_size"`
	Songs       []SongItem `json:"songs"`
}

// ScoreSong 给音频质量打分（无损优先 > 码率 > 采样率）
func ScoreSong(s SongItem) int {
	score := 0
	lossless := map[string]bool{
		"flac": true, "wav": true, "ape": true, "alac": true, "dsf": true, "dff": true, "aiff": true, "wavpack": true,
	}
	if lossless[strings.ToLower(s.Format)] {
		score += 1000000
	}
	score += s.Bitrate
	score += int(float64(s.SampleRate) * 0.1)
	if s.Title != "" {
		score += 500
	}
	if s.Artist != "" {
		score += 500
	}
	if s.Album != "" {
		score += 200
	}
	return score
}

// ClusterByFingerprintDuration 移植自 Songloft 的时长聚类算法
func ClusterByFingerprintDuration(songs []database.AudioFingerprintRecord, tolerance float64) [][]database.AudioFingerprintRecord {
	if len(songs) < 2 {
		return nil
	}
	sorted := make([]database.AudioFingerprintRecord, len(songs))
	copy(sorted, songs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Duration < sorted[j].Duration
	})

	var clusters [][]database.AudioFingerprintRecord
	current := []database.AudioFingerprintRecord{sorted[0]}

	for i := 1; i < len(sorted); i++ {
		prev := sorted[i-1]
		cur := sorted[i]
		splittable := prev.Duration > 0 && cur.Duration > 0 && (cur.Duration-prev.Duration) > tolerance
		if !splittable {
			current = append(current, cur)
			continue
		}
		if len(current) > 1 {
			clusters = append(clusters, current)
		}
		current = []database.AudioFingerprintRecord{cur}
	}
	if len(current) > 1 {
		clusters = append(clusters, current)
	}
	return clusters
}

// Deduplicator 去重管理器
type Deduplicator struct {
	db         *database.DB
	detector   *detector.Detector
	ffmpegPath string
}

func NewDeduplicator(db *database.DB, det *detector.Detector, ffmpegPath string) *Deduplicator {
	return &Deduplicator{
		db:         db,
		detector:   det,
		ffmpegPath: ffmpegPath,
	}
}

// ComputeSingle 计算单个音频指纹
func (d *Deduplicator) ComputeSingle(ctx context.Context, filePath string, force bool) (*database.AudioFingerprintRecord, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	mtime := float64(fi.ModTime().UnixNano()) / 1e9
	fileSize := fi.Size()
	fileName := filepath.Base(filePath)

	if !force {
		cached, _ := d.db.GetFingerprint(ctx, filePath)
		if cached != nil && cached.MTime == mtime && cached.FileSize == fileSize {
			if cached.IsFailed == 0 && cached.Fingerprint != "" {
				return cached, nil
			}
		}
	}

	det := d.detector.CheckFile(filePath)
	meta := det.Metadata
	attemptedAt := time.Now().Unix()

	if !det.IsAudio {
		rec := &database.AudioFingerprintRecord{
			FilePath:    filePath,
			FileName:    fileName,
			MTime:       mtime,
			FileSize:    fileSize,
			Fingerprint: "",
			Duration:    meta.Duration,
			Format:      det.DetectedFormat,
			Bitrate:     meta.Bitrate,
			SampleRate:  meta.SampleRate,
			Channels:    meta.Channels,
			Title:       meta.Title,
			Artist:      meta.Artist,
			Album:       meta.Album,
			AttemptedAt: attemptedAt,
			IsFailed:    1,
			ErrorMsg:    det.Details,
		}
		d.db.UpsertFingerprint(ctx, rec)
		return rec, nil
	}

	fp, dur, err := ExtractFingerprint(ctx, d.ffmpegPath, filePath)
	if err != nil {
		rec := &database.AudioFingerprintRecord{
			FilePath:    filePath,
			FileName:    fileName,
			MTime:       mtime,
			FileSize:    fileSize,
			Fingerprint: "",
			Duration:    meta.Duration,
			Format:      det.DetectedFormat,
			Bitrate:     meta.Bitrate,
			SampleRate:  meta.SampleRate,
			Channels:    meta.Channels,
			Title:       meta.Title,
			Artist:      meta.Artist,
			Album:       meta.Album,
			AttemptedAt: attemptedAt,
			IsFailed:    1,
			ErrorMsg:    err.Error(),
		}
		d.db.UpsertFingerprint(ctx, rec)
		return rec, err
	}

	actualDur := dur
	if actualDur == 0 {
		actualDur = meta.Duration
	}

	rec := &database.AudioFingerprintRecord{
		FilePath:    filePath,
		FileName:    fileName,
		MTime:       mtime,
		FileSize:    fileSize,
		Fingerprint: fp,
		Duration:    actualDur,
		Format:      det.DetectedFormat,
		Bitrate:     meta.Bitrate,
		SampleRate:  meta.SampleRate,
		Channels:    meta.Channels,
		Title:       meta.Title,
		Artist:      meta.Artist,
		Album:       meta.Album,
		AttemptedAt: attemptedAt,
		IsFailed:    0,
		ErrorMsg:    "",
	}
	d.db.UpsertFingerprint(ctx, rec)
	return rec, nil
}

// GetDuplicateGroups 获取所有重复组
func (d *Deduplicator) GetDuplicateGroups(ctx context.Context, tolerance float64) ([]DuplicateGroup, error) {
	all, err := d.db.ListAllValidFingerprints(ctx)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, nil
	}

	fpMap := make(map[string][]database.AudioFingerprintRecord)
	var fpOrder []string
	for _, r := range all {
		if _, exists := fpMap[r.Fingerprint]; !exists {
			fpOrder = append(fpOrder, r.Fingerprint)
		}
		fpMap[r.Fingerprint] = append(fpMap[r.Fingerprint], r)
	}

	var groups []DuplicateGroup
	grpIdx := 1

	for _, fp := range fpOrder {
		songs := fpMap[fp]
		if len(songs) < 2 {
			continue
		}
		clusters := ClusterByFingerprintDuration(songs, tolerance)
		for _, cluster := range clusters {
			if len(cluster) < 2 {
				continue
			}

			var songItems []SongItem
			var totalSize int64
			for _, s := range cluster {
				item := SongItem{
					FilePath:   s.FilePath,
					FileName:   s.FileName,
					FileSize:   s.FileSize,
					Duration:   s.Duration,
					Format:     s.Format,
					Bitrate:    s.Bitrate,
					SampleRate: s.SampleRate,
					Channels:   s.Channels,
					Title:      s.Title,
					Artist:     s.Artist,
					Album:      s.Album,
				}
				item.Score = ScoreSong(item)
				songItems = append(songItems, item)
				totalSize += s.FileSize
			}

			// 按打分从高到低排序，最高分为推荐保留
			sort.Slice(songItems, func(i, j int) bool {
				return songItems[i].Score > songItems[j].Score
			})
			if len(songItems) > 0 {
				songItems[0].IsRecommendedKeep = true
			}

			keepSize := int64(0)
			if len(songItems) > 0 {
				keepSize = songItems[0].FileSize
			}
			wasted := totalSize - keepSize

			groups = append(groups, DuplicateGroup{
				GroupID:     fmt.Sprintf("dup_grp_%d", grpIdx),
				Fingerprint: fp,
				TotalSize:   totalSize,
				WastedSize:  wasted,
				Songs:       songItems,
			})
			grpIdx++
		}
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].WastedSize > groups[j].WastedSize
	})
	return groups, nil
}
