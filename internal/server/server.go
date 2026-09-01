package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"music-toolkit/internal/config"
	"music-toolkit/internal/database"
	"music-toolkit/internal/deduplicator"
	"music-toolkit/internal/detector"
	"music-toolkit/internal/feiniu"
	"music-toolkit/internal/fileop"
	"music-toolkit/internal/lossless"
	"music-toolkit/internal/playlist"
)

// TaskProgress 通用任务进度
type TaskProgress struct {
	Status        string  `json:"status"` // idle, running, done, cancelled, error
	Total         int64   `json:"total"`
	Scanned       int64   `json:"scanned"`
	Computed      int64   `json:"computed"`
	Mismatched    int64   `json:"mismatched"`
	FakeOrCorrupt int64   `json:"fake_or_corrupt"`
	TrueLossless  int64   `json:"true_lossless"`
	FakeLossless  int64   `json:"fake_lossless"`
	Failed        int64   `json:"failed"`
	CurrentFile   string  `json:"current_file"`
	StartTime     int64   `json:"start_time"`
	Elapsed       float64 `json:"elapsed"`
	Error         string  `json:"error"`
}

// Server Web 服务器
type Server struct {
	cfg          *config.Config
	db           *database.DB
	detector     *detector.Detector
	dedup        *deduplicator.Deduplicator
	fileOp       *fileop.FileOperator
	playlist     *playlist.Extractor
	feiniuClient *feiniu.Client
	fnImporter   *feiniu.Importer
	frontendFS   http.FileSystem

	mu            sync.Mutex
	formatProg    TaskProgress
	dedupProg     TaskProgress
	losslessProg  TaskProgress
	cancelDedupFn context.CancelFunc
}

func NewServer(cfg *config.Config, db *database.DB, frontendFS http.FileSystem) *Server {
	det := detector.NewDetector()
	dedup := deduplicator.NewDeduplicator(db, det, cfg.FFmpegPath)
	fileOp := fileop.NewFileOperator(cfg.MusicDir, cfg.OutputDir)
	pl := playlist.NewExtractor()
	fnClient := feiniu.NewClient(db)
	fnImporter := feiniu.NewImporter(fnClient)

	s := &Server{
		cfg:          cfg,
		db:           db,
		detector:     det,
		dedup:        dedup,
		fileOp:       fileOp,
		playlist:     pl,
		feiniuClient: fnClient,
		fnImporter:   fnImporter,
		frontendFS:   frontendFS,
		formatProg:   TaskProgress{Status: "idle"},
		dedupProg:    TaskProgress{Status: "idle"},
		losslessProg: TaskProgress{Status: "idle"},
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// API 路由
	mux.HandleFunc("GET /api/system/status", s.handleSystemStatus)
	mux.HandleFunc("GET /api/system/dir-stats", s.handleDirStats)
	mux.HandleFunc("POST /api/format/scan", s.handleFormatScan)
	mux.HandleFunc("GET /api/format/progress", s.handleFormatProgress)
	mux.HandleFunc("GET /api/format/records", s.handleFormatRecords)
	mux.HandleFunc("POST /api/format/action-single", s.handleFormatActionSingle)
	mux.HandleFunc("POST /api/format/action-batch", s.handleFormatActionBatch)
	mux.HandleFunc("POST /api/format/fix-single", s.handleFormatActionSingle)
	mux.HandleFunc("POST /api/format/fix-batch", s.handleFormatActionBatch)
	mux.HandleFunc("GET /api/format/export", s.handleFormatExport)

	mux.HandleFunc("POST /api/dedup/compute", s.handleDedupCompute)
	mux.HandleFunc("POST /api/dedup/cancel", s.handleDedupCancel)
	mux.HandleFunc("GET /api/dedup/progress", s.handleDedupProgress)
	mux.HandleFunc("GET /api/dedup/groups", s.handleDedupGroups)
	mux.HandleFunc("POST /api/dedup/clean", s.handleDedupClean)
	mux.HandleFunc("POST /api/dedup/clean-recommended", s.handleDedupCleanRecommended)

	mux.HandleFunc("POST /api/lossless/scan", s.handleLosslessScan)
	mux.HandleFunc("GET /api/lossless/progress", s.handleLosslessProgress)
	mux.HandleFunc("GET /api/lossless/records", s.handleLosslessRecords)
	mux.HandleFunc("GET /api/lossless/export", s.handleLosslessExport)

	// 歌单提取相关路由
	mux.HandleFunc("POST /api/playlist/parse", s.handlePlaylistParse)
	mux.HandleFunc("GET /api/playlist/history", s.handlePlaylistHistoryList)
	mux.HandleFunc("GET /api/playlist/history-detail", s.handlePlaylistHistoryDetail)
	mux.HandleFunc("POST /api/playlist/history-delete", s.handlePlaylistHistoryDelete)
	mux.HandleFunc("POST /api/playlist/history-clear", s.handlePlaylistHistoryClear)
	mux.HandleFunc("POST /api/playlist/export", s.handlePlaylistExport)

	// 飞牛音乐歌单与连接路由
	mux.HandleFunc("POST /api/feiniu/connect", s.handleFeiNiuConnect)
	mux.HandleFunc("GET /api/feiniu/status", s.handleFeiNiuStatus)
	mux.HandleFunc("POST /api/feiniu/disconnect", s.handleFeiNiuDisconnect)
	mux.HandleFunc("GET /api/feiniu/playlists", s.handleFeiNiuPlaylists)
	mux.HandleFunc("GET /api/feiniu/playlist/tracks", s.handleFeiNiuPlaylistTracks)
	mux.HandleFunc("POST /api/feiniu/playlist/create", s.handleFeiNiuPlaylistCreate)
	mux.HandleFunc("POST /api/feiniu/playlist/edit", s.handleFeiNiuPlaylistEdit)
	mux.HandleFunc("POST /api/feiniu/playlist/delete", s.handleFeiNiuPlaylistDelete)
	mux.HandleFunc("POST /api/feiniu/playlist/add-tracks", s.handleFeiNiuPlaylistAddTracks)
	mux.HandleFunc("POST /api/feiniu/playlist/remove-tracks", s.handleFeiNiuPlaylistRemoveTracks)
	mux.HandleFunc("POST /api/feiniu/playlist/purge", s.handleFeiNiuPlaylistPurge)
	mux.HandleFunc("POST /api/feiniu/playlist/import", s.handleFeiNiuPlaylistImport)
	mux.HandleFunc("GET /api/feiniu/cover", s.handleFeiNiuCover)

	mux.HandleFunc("GET /api/audio/stream", s.handleAudioStream)

	// 静态文件托管
	fileServer := http.FileServer(s.frontendFS)
	mux.Handle("GET /static/", http.StripPrefix("/static/", fileServer))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		f, err := s.frontendFS.Open("index.html")
		if err != nil {
			http.Error(w, "index.html not found", 404)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.Copy(w, f)
	})

	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func collectAudioFiles(targetDir, outputDir string) []string {
	var list []string
	absTarget, _ := filepath.Abs(targetDir)
	absOutput, _ := filepath.Abs(outputDir)

	if _, err := os.Stat(absTarget); os.IsNotExist(err) {
		return nil
	}

	filepath.Walk(absTarget, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		absPath, _ := filepath.Abs(path)
		if info.IsDir() {
			if absPath == absOutput || strings.HasPrefix(absPath, absOutput+string(os.PathSeparator)) {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if config.DefaultAudioExtensions[ext] {
			list = append(list, path)
		}
		return nil
	})
	return list
}

// DirStats 目录统计信息
type DirStats struct {
	Path         string         `json:"path"`
	Exists       bool           `json:"exists"`
	TotalFiles   int            `json:"total_files"`
	TotalSize    int64          `json:"total_size"`
	FormatCounts map[string]int `json:"format_counts"`
}

func scanDirStats(targetDir, outputDir string) DirStats {
	absTarget, _ := filepath.Abs(targetDir)
	absOutput, _ := filepath.Abs(outputDir)

	stats := DirStats{
		Path:         targetDir,
		FormatCounts: make(map[string]int),
	}

	fi, err := os.Stat(absTarget)
	if os.IsNotExist(err) || fi == nil || !fi.IsDir() {
		stats.Exists = false
		return stats
	}
	stats.Exists = true

	filepath.Walk(absTarget, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		absPath, _ := filepath.Abs(path)
		if info.IsDir() {
			if absPath == absOutput || strings.HasPrefix(absPath, absOutput+string(os.PathSeparator)) {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if config.DefaultAudioExtensions[ext] {
			stats.TotalFiles++
			stats.TotalSize += info.Size()
			stats.FormatCounts[ext]++
		}
		return nil
	})

	return stats
}

// ----------------- 系统状态与目录查询 -----------------

func (s *Server) handleDirStats(w http.ResponseWriter, r *http.Request) {
	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		dirPath = s.cfg.MusicDir
	}
	stats := scanDirStats(dirPath, s.cfg.OutputDir)
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	_, ffmpegErr := exec.LookPath(s.cfg.FFmpegPath)
	chromaOk := deduplicator.IsChromaprintAvailable(s.cfg.FFmpegPath)
	musicStats := scanDirStats(s.cfg.MusicDir, s.cfg.OutputDir)

	s.mu.Lock()
	fmtProg := s.formatProg
	dedupProg := s.dedupProg
	lossProg := s.losslessProg
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"music_dir":        s.cfg.MusicDir,
		"music_dir_exists": musicStats.Exists,
		"output_dir":       s.cfg.OutputDir,
		"has_ffmpeg":       ffmpegErr == nil,
		"has_chromaprint":  chromaOk,
		"music_stats":      musicStats,
		"tasks": map[string]any{
			"format_scan":   fmtProg,
			"dedup_compute": dedupProg,
			"lossless_scan": lossProg,
		},
	})
}

// ----------------- 格式检查 -----------------

type FormatScanReq struct {
	MusicDir string `json:"music_dir"`
	Workers  int    `json:"workers"`
}

func (s *Server) handleFormatScan(w http.ResponseWriter, r *http.Request) {
	var req FormatScanReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Workers = 4
	}

	s.mu.Lock()
	if s.formatProg.Status == "running" {
		s.mu.Unlock()
		http.Error(w, `{"detail":"格式扫描任务已在运行中"}`, http.StatusBadRequest)
		return
	}
	s.formatProg = TaskProgress{
		Status:    "running",
		StartTime: time.Now().Unix(),
	}
	s.mu.Unlock()

	go s.runFormatScanAsync(req)
	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "message": "格式检测任务已启动"})
}

func (s *Server) runFormatScanAsync(req FormatScanReq) {
	scanDir := req.MusicDir
	if scanDir == "" {
		scanDir = s.cfg.MusicDir
	}
	workers := req.Workers
	if workers <= 0 {
		workers = s.cfg.MaxWorkers
	}

	files := collectAudioFiles(scanDir, s.cfg.OutputDir)

	s.mu.Lock()
	s.formatProg.Total = int64(len(files))
	s.mu.Unlock()

	if len(files) == 0 {
		s.mu.Lock()
		s.formatProg.Status = "done"
		s.formatProg.Elapsed = float64(time.Now().Unix() - s.formatProg.StartTime)
		s.mu.Unlock()
		return
	}

	s.db.ClearFormatRecords(context.Background())

	var scanned, mismatched, fakeOrCorrupt atomic.Int64
	ch := make(chan string, workers*2)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fp := range ch {
				det := s.detector.CheckFile(fp)
				fi, _ := os.Stat(fp)
				mtime := float64(0)
				fileSize := int64(0)
				if fi != nil {
					mtime = float64(fi.ModTime().UnixNano()) / 1e9
					fileSize = fi.Size()
				}

				statusMsg := "ok"
				if det.IsMismatch {
					statusMsg = "detected"
				}

				isMismatchInt := 0
				if det.IsMismatch {
					isMismatchInt = 1
					mismatched.Add(1)
					if !det.IsAudio {
						fakeOrCorrupt.Add(1)
					}
				}
				isAudioInt := 0
				if det.IsAudio {
					isAudioInt = 1
				}

				rec := &database.FormatRecord{
					FilePath:       fp,
					FileName:       det.FileName,
					MTime:          mtime,
					FileSize:       fileSize,
					CurrentExt:     det.CurrentExt,
					DetectedFormat: det.DetectedFormat,
					SuggestedExt:   det.SuggestedExt,
					IsMismatch:     isMismatchInt,
					IsAudio:        isAudioInt,
					Details:        det.Details,
					Status:         statusMsg,
					UpdatedAt:      time.Now().Unix(),
				}
				s.db.UpsertFormatRecord(context.Background(), rec)

				curScanned := scanned.Add(1)
				s.mu.Lock()
				s.formatProg.Scanned = curScanned
				s.formatProg.Mismatched = mismatched.Load()
				s.formatProg.FakeOrCorrupt = fakeOrCorrupt.Load()
				s.formatProg.CurrentFile = det.FileName
				s.formatProg.Elapsed = float64(time.Now().Unix() - s.formatProg.StartTime)
				s.mu.Unlock()
			}
		}()
	}

	for _, f := range files {
		ch <- f
	}
	close(ch)
	wg.Wait()

	s.mu.Lock()
	s.formatProg.Status = "done"
	s.formatProg.Elapsed = float64(time.Now().Unix() - s.formatProg.StartTime)
	s.mu.Unlock()
}

func (s *Server) handleFormatProgress(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	prog := s.formatProg
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, prog)
}

func (s *Server) handleFormatRecords(w http.ResponseWriter, r *http.Request) {
	records, err := s.db.ListFormatRecords(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	mismatchOnly := r.URL.Query().Get("mismatch_only") == "true"
	query := strings.ToLower(r.URL.Query().Get("query"))

	var filtered []database.FormatRecord
	for _, rec := range records {
		if mismatchOnly && rec.IsMismatch != 1 {
			continue
		}
		if query != "" {
			if !strings.Contains(strings.ToLower(rec.FileName), query) && !strings.Contains(strings.ToLower(rec.Details), query) {
				continue
			}
		}
		filtered = append(filtered, rec)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":   len(filtered),
		"records": filtered,
	})
}

type FormatActionReq struct {
	FilePath      string `json:"file_path"`
	Action        string `json:"action"` // rename_fix, copy, move, recycle, delete
	OutputDir     string `json:"output_dir,omitempty"`
	KeepStructure bool   `json:"keep_structure"`
	FixExtension  bool   `json:"fix_extension"`
	SuggestedExt  string `json:"suggested_ext,omitempty"`
}

func (s *Server) handleFormatActionSingle(w http.ResponseWriter, r *http.Request) {
	var req FormatActionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	outDir := req.OutputDir
	if outDir == "" {
		outDir = s.cfg.OutputDir
	}

	var success bool
	var tgt, msg string

	switch req.Action {
	case "rename_fix":
		suggested := req.SuggestedExt
		if suggested == "" {
			det := s.detector.CheckFile(req.FilePath)
			suggested = det.SuggestedExt
		}
		success, tgt, msg = s.fileOp.RenameInPlace(req.FilePath, suggested)
		if success {
			s.db.DeleteFormatRecord(r.Context(), req.FilePath)
			if _, err := os.Stat(tgt); err == nil {
				det := s.detector.CheckFile(tgt)
				fi, _ := os.Stat(tgt)
				s.db.UpsertFormatRecord(r.Context(), &database.FormatRecord{
					FilePath:       tgt,
					FileName:       det.FileName,
					MTime:          float64(fi.ModTime().UnixNano()) / 1e9,
					FileSize:       fi.Size(),
					CurrentExt:     det.CurrentExt,
					DetectedFormat: det.DetectedFormat,
					SuggestedExt:   det.SuggestedExt,
					IsMismatch:     0,
					IsAudio:        1,
					Details:        det.Details,
					Status:         "后缀已修正",
					UpdatedAt:      time.Now().Unix(),
				})
			}
		}
	case "copy":
		suggested := req.SuggestedExt
		if suggested == "" {
			det := s.detector.CheckFile(req.FilePath)
			suggested = det.SuggestedExt
		}
		success, tgt, msg = s.fileOp.ProcessMismatched(req.FilePath, outDir, "copy", req.KeepStructure, req.FixExtension, suggested, s.cfg.MusicDir)
		if success {
			if fi, err := os.Stat(req.FilePath); err == nil {
				det := s.detector.CheckFile(req.FilePath)
				s.db.UpsertFormatRecord(r.Context(), &database.FormatRecord{
					FilePath:       req.FilePath,
					FileName:       det.FileName,
					MTime:          float64(fi.ModTime().UnixNano()) / 1e9,
					FileSize:       fi.Size(),
					CurrentExt:     det.CurrentExt,
					DetectedFormat: det.DetectedFormat,
					SuggestedExt:   det.SuggestedExt,
					IsMismatch:     1,
					IsAudio:        1,
					Details:        det.Details,
					Status:         "已复制到: " + filepath.Base(tgt),
					UpdatedAt:      time.Now().Unix(),
				})
			}
		}
	case "move":
		suggested := req.SuggestedExt
		if suggested == "" {
			det := s.detector.CheckFile(req.FilePath)
			suggested = det.SuggestedExt
		}
		success, tgt, msg = s.fileOp.ProcessMismatched(req.FilePath, outDir, "move", req.KeepStructure, req.FixExtension, suggested, s.cfg.MusicDir)
		if success {
			s.db.DeleteFormatRecord(r.Context(), req.FilePath)
		}
	case "recycle":
		success, tgt, msg = s.fileOp.MoveToRecycleBin(req.FilePath, "")
		if success {
			s.db.DeleteFormatRecord(r.Context(), req.FilePath)
		}
	case "delete":
		success, tgt, msg = s.fileOp.DeletePermanently(req.FilePath)
		if success {
			s.db.DeleteFormatRecord(r.Context(), req.FilePath)
		}
	default:
		http.Error(w, `{"detail":"未知操作类型"}`, 400)
		return
	}

	if !success {
		http.Error(w, fmt.Sprintf(`{"detail":"%s"}`, msg), 400)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"target":  tgt,
		"message": msg,
	})
}

type FormatBatchActionReq struct {
	FilePaths     []string `json:"file_paths"`
	Action        string   `json:"action"` // rename_fix, copy, move, recycle, delete
	OutputDir     string   `json:"output_dir,omitempty"`
	KeepStructure bool   `json:"keep_structure"`
	FixExtension  bool   `json:"fix_extension"`
}

func (s *Server) handleFormatActionBatch(w http.ResponseWriter, r *http.Request) {
	var req FormatBatchActionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	outDir := req.OutputDir
	if outDir == "" {
		outDir = s.cfg.OutputDir
	}

	type actionResult struct {
		FilePath string `json:"file_path"`
		Success  bool   `json:"success"`
		Target   string `json:"target"`
		Message  string `json:"msg"`
	}
	var results []actionResult

	for _, fp := range req.FilePaths {
		det := s.detector.CheckFile(fp)
		var success bool
		var tgt, msg string

		switch req.Action {
		case "rename_fix":
			success, tgt, msg = s.fileOp.RenameInPlace(fp, det.SuggestedExt)
			if success {
				s.db.DeleteFormatRecord(r.Context(), fp)
				if _, err := os.Stat(tgt); err == nil {
					detNew := s.detector.CheckFile(tgt)
					fi, _ := os.Stat(tgt)
					s.db.UpsertFormatRecord(r.Context(), &database.FormatRecord{
						FilePath:       tgt,
						FileName:       detNew.FileName,
						MTime:          float64(fi.ModTime().UnixNano()) / 1e9,
						FileSize:       fi.Size(),
						CurrentExt:     detNew.CurrentExt,
						DetectedFormat: detNew.DetectedFormat,
						SuggestedExt:   detNew.SuggestedExt,
						IsMismatch:     0,
						IsAudio:        1,
						Details:        detNew.Details,
						Status:         "后缀已修正",
						UpdatedAt:      time.Now().Unix(),
					})
				}
			}
		case "copy":
			success, tgt, msg = s.fileOp.ProcessMismatched(fp, outDir, "copy", req.KeepStructure, req.FixExtension, det.SuggestedExt, s.cfg.MusicDir)
			if success {
				if fi, err := os.Stat(fp); err == nil {
					s.db.UpsertFormatRecord(r.Context(), &database.FormatRecord{
						FilePath:       fp,
						FileName:       det.FileName,
						MTime:          float64(fi.ModTime().UnixNano()) / 1e9,
						FileSize:       fi.Size(),
						CurrentExt:     det.CurrentExt,
						DetectedFormat: det.DetectedFormat,
						SuggestedExt:   det.SuggestedExt,
						IsMismatch:     1,
						IsAudio:        1,
						Details:        det.Details,
						Status:         "已复制到: " + filepath.Base(tgt),
						UpdatedAt:      time.Now().Unix(),
					})
				}
			}
		case "move":
			success, tgt, msg = s.fileOp.ProcessMismatched(fp, outDir, "move", req.KeepStructure, req.FixExtension, det.SuggestedExt, s.cfg.MusicDir)
			if success {
				s.db.DeleteFormatRecord(r.Context(), fp)
			}
		case "recycle":
			success, tgt, msg = s.fileOp.MoveToRecycleBin(fp, "")
			if success {
				s.db.DeleteFormatRecord(r.Context(), fp)
			}
		case "delete":
			success, tgt, msg = s.fileOp.DeletePermanently(fp)
			if success {
				s.db.DeleteFormatRecord(r.Context(), fp)
			}
		default:
			success, tgt, msg = false, "", "未知操作类型"
		}

		results = append(results, actionResult{
			FilePath: fp,
			Success:  success,
			Target:   tgt,
			Message:  msg,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":   len(req.FilePaths),
		"results": results,
	})
}

func (s *Server) handleFormatExport(w http.ResponseWriter, r *http.Request) {
	records, err := s.db.ListFormatRecords(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8-sig")
	w.Header().Set("Content-Disposition", "attachment; filename=mismatched_report.csv")

	writer := csv.NewWriter(w)
	writer.Write([]string{"文件名", "当前后缀", "真实格式", "建议后缀", "是否不一致", "是否音频", "详情", "路径", "状态"})

	for _, rec := range records {
		mismatch := "否"
		if rec.IsMismatch == 1 {
			mismatch = "是"
		}
		audio := "否"
		if rec.IsAudio == 1 {
			audio = "是"
		}
		writer.Write([]string{
			rec.FileName, rec.CurrentExt, rec.DetectedFormat, rec.SuggestedExt,
			mismatch, audio, rec.Details, rec.FilePath, rec.Status,
		})
	}
	writer.Flush()
}

// ----------------- 指纹去重 -----------------

type DedupComputeReq struct {
	Mode     string `json:"mode"` // missing, recompute_all, retry_failed
	MusicDir string `json:"music_dir"`
	Workers  int    `json:"workers"`
}

func (s *Server) handleDedupCompute(w http.ResponseWriter, r *http.Request) {
	var req DedupComputeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Mode = "missing"
		req.Workers = 4
	}

	s.mu.Lock()
	if s.dedupProg.Status == "running" {
		s.mu.Unlock()
		http.Error(w, `{"detail":"指纹计算任务已在运行中"}`, 400)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelDedupFn = cancel
	s.dedupProg = TaskProgress{
		Status:    "running",
		StartTime: time.Now().Unix(),
	}
	s.mu.Unlock()

	go s.runDedupComputeAsync(ctx, req)
	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "message": "音频指纹计算任务已启动"})
}

func (s *Server) handleDedupCancel(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.cancelDedupFn != nil {
		s.cancelDedupFn()
		s.cancelDedupFn = nil
	}
	s.dedupProg.Status = "cancelled"
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelling", "message": "正在取消任务..."})
}

func (s *Server) runDedupComputeAsync(ctx context.Context, req DedupComputeReq) {
	scanDir := req.MusicDir
	if scanDir == "" {
		scanDir = s.cfg.MusicDir
	}
	workers := req.Workers
	if workers <= 0 {
		workers = s.cfg.MaxWorkers
	}

	files := collectAudioFiles(scanDir, s.cfg.OutputDir)

	// 清理不存在的文件
	existingMap := make(map[string]bool)
	for _, f := range files {
		existingMap[f] = true
	}
	s.db.RemoveMissingFiles(ctx, existingMap)

	if req.Mode == "recompute_all" {
		s.db.ClearAllFingerprints(ctx)
	} else if req.Mode == "retry_failed" {
		s.db.ResetFailedFingerprints(ctx)
	}

	var toProcess []string
	for _, f := range files {
		if req.Mode == "recompute_all" {
			toProcess = append(toProcess, f)
		} else {
			cached, _ := s.db.GetFingerprint(ctx, f)
			if cached == nil || (cached.IsFailed == 1 && req.Mode == "retry_failed") || cached.Fingerprint == "" {
				toProcess = append(toProcess, f)
			}
		}
	}

	s.mu.Lock()
	s.dedupProg.Total = int64(len(toProcess))
	s.mu.Unlock()

	if len(toProcess) == 0 {
		s.mu.Lock()
		s.dedupProg.Status = "done"
		s.dedupProg.Elapsed = float64(time.Now().Unix() - s.dedupProg.StartTime)
		s.mu.Unlock()
		return
	}

	var computed, failed atomic.Int64
	ch := make(chan string, workers*2)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fp := range ch {
				select {
				case <-ctx.Done():
					return
				default:
				}

				rec, err := s.dedup.ComputeSingle(ctx, fp, req.Mode == "recompute_all")
				if err != nil || (rec != nil && rec.IsFailed == 1) {
					failed.Add(1)
				} else {
					computed.Add(1)
				}

				s.mu.Lock()
				s.dedupProg.Computed = computed.Load()
				s.dedupProg.Failed = failed.Load()
				if rec != nil {
					s.dedupProg.CurrentFile = rec.FileName
				}
				s.dedupProg.Elapsed = float64(time.Now().Unix() - s.dedupProg.StartTime)
				s.mu.Unlock()
			}
		}()
	}

	for _, f := range toProcess {
		select {
		case <-ctx.Done():
			break
		case ch <- f:
		}
	}
	close(ch)
	wg.Wait()

	s.mu.Lock()
	if ctx.Err() != nil {
		s.dedupProg.Status = "cancelled"
	} else {
		s.dedupProg.Status = "done"
	}
	s.dedupProg.Elapsed = float64(time.Now().Unix() - s.dedupProg.StartTime)
	s.mu.Unlock()
}

func (s *Server) handleDedupProgress(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	prog := s.dedupProg
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, prog)
}

func (s *Server) handleDedupGroups(w http.ResponseWriter, r *http.Request) {
	tolStr := r.URL.Query().Get("tolerance")
	tolerance := deduplicator.DuplicateDurationTolerance
	if tolStr != "" {
		if t, err := strconv.ParseFloat(tolStr, 64); err == nil && t >= 0 {
			tolerance = t
		}
	}

	groups, err := s.dedup.GetDuplicateGroups(r.Context(), tolerance)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	totalDuplicateSongs := 0
	totalWasted := int64(0)
	for _, g := range groups {
		totalDuplicateSongs += len(g.Songs)
		totalWasted += g.WastedSize
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"group_count":           len(groups),
		"total_duplicate_songs": totalDuplicateSongs,
		"total_wasted_size":     totalWasted,
		"groups":                groups,
	})
}

type CleanDupReq struct {
	FilePaths []string `json:"file_paths"`
	Action    string   `json:"action"` // recycle, delete
}

func (s *Server) handleDedupClean(w http.ResponseWriter, r *http.Request) {
	var req CleanDupReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	type cleanResult struct {
		FilePath string `json:"file_path"`
		Success  bool   `json:"success"`
		Target   string `json:"target"`
		Message  string `json:"message"`
	}
	var results []cleanResult
	deletedPaths := make(map[string]bool)

	for _, fp := range req.FilePaths {
		var success bool
		var tgt, msg string
		if req.Action == "recycle" {
			success, tgt, msg = s.fileOp.MoveToRecycleBin(fp, "")
		} else if req.Action == "delete" {
			success, tgt, msg = s.fileOp.DeletePermanently(fp)
		} else {
			success, tgt, msg = false, "", "不支持的清理操作"
		}
		if success {
			deletedPaths[fp] = true
		}
		results = append(results, cleanResult{
			FilePath: fp,
			Success:  success,
			Target:   tgt,
			Message:  msg,
		})
	}

	if len(deletedPaths) > 0 {
		all, _ := s.db.ListAllValidFingerprints(r.Context())
		survived := make(map[string]bool)
		for _, r := range all {
			if !deletedPaths[r.FilePath] {
				survived[r.FilePath] = true
			}
		}
		s.db.RemoveMissingFiles(r.Context(), survived)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":   len(req.FilePaths),
		"results": results,
	})
}

func (s *Server) handleDedupCleanRecommended(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")
	if action == "" {
		action = "recycle"
	}
	tolStr := r.URL.Query().Get("tolerance")
	tolerance := deduplicator.DuplicateDurationTolerance
	if tolStr != "" {
		if t, err := strconv.ParseFloat(tolStr, 64); err == nil && t >= 0 {
			tolerance = t
		}
	}

	groups, err := s.dedup.GetDuplicateGroups(r.Context(), tolerance)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var toClean []string
	for _, g := range groups {
		for _, s := range g.Songs {
			if !s.IsRecommendedKeep {
				toClean = append(toClean, s.FilePath)
			}
		}
	}

	if len(toClean) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"total": 0, "message": "没有发现需要清理的冗余音频"})
		return
	}

	cleanReq := CleanDupReq{FilePaths: toClean, Action: action}
	bodyBytes, _ := json.Marshal(cleanReq)
	r.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	s.handleDedupClean(w, r)
}

func (s *Server) handleAudioStream(w http.ResponseWriter, r *http.Request) {
	rawPath := r.URL.Query().Get("path")
	decoded, err := url.QueryUnescape(rawPath)
	if err != nil {
		decoded = rawPath
	}

	if _, err := os.Stat(decoded); os.IsNotExist(err) {
		http.Error(w, "audio file not found", 404)
		return
	}

	ext := strings.ToLower(filepath.Ext(decoded))
	mimeMap := map[string]string{
		".mp3":  "audio/mpeg",
		".m4a":  "audio/mp4",
		".flac": "audio/flac",
		".wav":  "audio/wav",
		".ogg":  "audio/ogg",
		".opus": "audio/opus",
	}
	if ct, ok := mimeMap[ext]; ok {
		w.Header().Set("Content-Type", ct)
	}

	http.ServeFile(w, r, decoded)
}

// ----------------- 真假无损鉴别 -----------------

type LosslessScanReq struct {
	MusicDir string `json:"music_dir"`
	Workers  int    `json:"workers"`
}

func (s *Server) handleLosslessScan(w http.ResponseWriter, r *http.Request) {
	var req LosslessScanReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Workers = 4
	}

	s.mu.Lock()
	if s.losslessProg.Status == "running" {
		s.mu.Unlock()
		http.Error(w, `{"detail":"无损检测任务已在运行中"}`, 400)
		return
	}
	s.losslessProg = TaskProgress{
		Status:    "running",
		StartTime: time.Now().Unix(),
	}
	s.mu.Unlock()

	go s.runLosslessScanAsync(req)
	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "message": "真假无损检测任务已启动"})
}

func (s *Server) runLosslessScanAsync(req LosslessScanReq) {
	scanDir := req.MusicDir
	if scanDir == "" {
		scanDir = s.cfg.MusicDir
	}
	workers := req.Workers
	if workers <= 0 {
		workers = s.cfg.MaxWorkers
	}

	allAudio := collectAudioFiles(scanDir, s.cfg.OutputDir)

	// 专门过滤待检查的无损音频格式 (.flac, .ape, .wav, .alac, .aiff, .wv)
	var losslessFiles []string
	losslessExts := map[string]bool{
		".flac": true,
		".ape":  true,
		".wav":  true,
		".wv":   true,
		".aiff": true,
		".aif":  true,
		".alac": true,
	}
	for _, f := range allAudio {
		ext := strings.ToLower(filepath.Ext(f))
		if losslessExts[ext] {
			losslessFiles = append(losslessFiles, f)
		}
	}

	s.mu.Lock()
	s.losslessProg.Total = int64(len(losslessFiles))
	s.mu.Unlock()

	if len(losslessFiles) == 0 {
		s.mu.Lock()
		s.losslessProg.Status = "done"
		s.losslessProg.Elapsed = float64(time.Now().Unix() - s.losslessProg.StartTime)
		s.mu.Unlock()
		return
	}

	s.db.ClearLosslessRecords(context.Background())

	var scanned, trueLossless, fakeLossless, failed atomic.Int64
	ch := make(chan string, workers*2)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fp := range ch {
				det := s.detector.CheckFile(fp)
				fi, _ := os.Stat(fp)
				mtime := float64(0)
				fileSize := int64(0)
				if fi != nil {
					mtime = float64(fi.ModTime().UnixNano()) / 1e9
					fileSize = fi.Size()
				}

				meta := det.Metadata
				sr := meta.SampleRate
				if sr == 0 {
					sr = 44100
				}

				res := lossless.AnalyzeLossless(context.Background(), s.cfg.FFmpegPath, fp, sr)
				if res.Grade == lossless.GradeUnknown {
					failed.Add(1)
				} else if res.Grade == lossless.GradeBadFake || res.Grade == lossless.GradeSuspectFake {
					fakeLossless.Add(1)
				} else {
					trueLossless.Add(1)
				}

				rec := &database.LosslessRecord{
					FilePath:       fp,
					FileName:       det.FileName,
					MTime:          mtime,
					FileSize:       fileSize,
					Format:         det.DetectedFormat,
					SampleRate:     sr,
					Bitrate:        meta.Bitrate,
					Duration:       meta.Duration,
					Grade:          string(res.Grade),
					GradeText:      res.GradeText,
					CutoffFreqHz:   res.CutoffFreqHz,
					HighFreqEnergy: res.HighFreqEnergy,
					Confidence:     res.Confidence,
					Details:        res.Details,
					UpdatedAt:      time.Now().Unix(),
				}
				s.db.UpsertLosslessRecord(context.Background(), rec)

				curScanned := scanned.Add(1)
				s.mu.Lock()
				s.losslessProg.Scanned = curScanned
				s.losslessProg.TrueLossless = trueLossless.Load()
				s.losslessProg.FakeLossless = fakeLossless.Load()
				s.losslessProg.Failed = failed.Load()
				s.losslessProg.CurrentFile = det.FileName
				s.losslessProg.Elapsed = float64(time.Now().Unix() - s.losslessProg.StartTime)
				s.mu.Unlock()
			}
		}()
	}

	for _, f := range losslessFiles {
		ch <- f
	}
	close(ch)
	wg.Wait()

	s.mu.Lock()
	s.losslessProg.Status = "done"
	s.losslessProg.Elapsed = float64(time.Now().Unix() - s.losslessProg.StartTime)
	s.mu.Unlock()
}

func (s *Server) handleLosslessProgress(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	prog := s.losslessProg
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, prog)
}

func (s *Server) handleLosslessRecords(w http.ResponseWriter, r *http.Request) {
	records, err := s.db.ListLosslessRecords(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	filter := r.URL.Query().Get("filter") // all, fake_only, true_only
	query := strings.ToLower(r.URL.Query().Get("query"))

	var filtered []database.LosslessRecord
	for _, rec := range records {
		if filter == "fake_only" && rec.Grade != string(lossless.GradeBadFake) && rec.Grade != string(lossless.GradeSuspectFake) {
			continue
		}
		if filter == "true_only" && rec.Grade != string(lossless.GradeTrueCD) && rec.Grade != string(lossless.GradeTrueHiRes) {
			continue
		}
		if query != "" {
			if !strings.Contains(strings.ToLower(rec.FileName), query) && !strings.Contains(strings.ToLower(rec.Details), query) {
				continue
			}
		}
		filtered = append(filtered, rec)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":   len(filtered),
		"records": filtered,
	})
}

func (s *Server) handleLosslessExport(w http.ResponseWriter, r *http.Request) {
	records, err := s.db.ListLosslessRecords(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8-sig")
	w.Header().Set("Content-Disposition", "attachment; filename=lossless_check_report.csv")

	writer := csv.NewWriter(w)
	writer.Write([]string{"文件名", "格式", "采样率", "比特率", "无损判定", "高频截止频率(Hz)", "置信度", "详细分析", "路径"})

	for _, rec := range records {
		writer.Write([]string{
			rec.FileName, rec.Format, strconv.Itoa(rec.SampleRate), strconv.Itoa(rec.Bitrate),
			rec.GradeText, strconv.Itoa(rec.CutoffFreqHz), fmt.Sprintf("%d%%", rec.Confidence),
			rec.Details, rec.FilePath,
		})
	}
	writer.Flush()
}

// ----------------- 歌单提取与历史 -----------------

func (s *Server) handlePlaylistParse(w http.ResponseWriter, r *http.Request) {
	var req playlist.ParseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"detail":"无效的请求参数格式"}`, 400)
		return
	}

	result, err := s.playlist.Parse(req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"detail":%q}`, err.Error()), 400)
		return
	}

	// 默认或勾选时保存到 SQLite 历史
	if req.SaveHistory {
		songsJSON, _ := json.Marshal(result.Songs)
		histID, dbErr := s.db.SavePlaylistHistory(r.Context(), &database.PlaylistHistoryRecord{
			Platform:  result.Platform,
			SourceURL: result.SourceURL,
			Title:     result.Title,
			SongCount: result.SongCount,
			SongsJSON: string(songsJSON),
			CreatedAt: time.Now().Unix(),
		})
		if dbErr == nil {
			result.HistoryID = histID
		}
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePlaylistHistoryList(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 30
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	records, err := s.db.ListPlaylistHistory(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	type historySummary struct {
		ID        int64  `json:"id"`
		Platform  string `json:"platform"`
		SourceURL string `json:"source_url"`
		Title     string `json:"title"`
		SongCount int    `json:"song_count"`
		CreatedAt int64  `json:"created_at"`
	}

	list := make([]historySummary, len(records))
	for i, rec := range records {
		list[i] = historySummary{
			ID:        rec.ID,
			Platform:  rec.Platform,
			SourceURL: rec.SourceURL,
			Title:     rec.Title,
			SongCount: rec.SongCount,
			CreatedAt: rec.CreatedAt,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":   len(list),
		"history": list,
	})
}

func (s *Server) handlePlaylistHistoryDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", 400)
		return
	}

	rec, err := s.db.GetPlaylistHistory(r.Context(), id)
	if err != nil || rec == nil {
		http.Error(w, "history not found", 404)
		return
	}

	var songs []playlist.SongItem
	if err := json.Unmarshal([]byte(rec.SongsJSON), &songs); err != nil {
		songs = []playlist.SongItem{}
	}

	textList := make([]string, len(songs))
	for i, s := range songs {
		textList[i] = s.FullText
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         rec.ID,
		"platform":   rec.Platform,
		"source_url": rec.SourceURL,
		"title":      rec.Title,
		"song_count": rec.SongCount,
		"songs":      songs,
		"text_list":  textList,
		"raw_text":   strings.Join(textList, "\n"),
		"created_at": rec.CreatedAt,
	})
}

func (s *Server) handlePlaylistHistoryDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	if err := s.db.DeletePlaylistHistory(r.Context(), req.ID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handlePlaylistHistoryClear(w http.ResponseWriter, r *http.Request) {
	if err := s.db.ClearPlaylistHistory(r.Context()); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handlePlaylistExport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title  string   `json:"title"`
		Format string   `json:"format"` // txt, csv
		Songs  []string `json:"songs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	filename := req.Title
	if filename == "" {
		filename = "playlist"
	}
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, "\\", "_")

	if req.Format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8-sig")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", url.QueryEscape(filename)))
		writer := csv.NewWriter(w)
		writer.Write([]string{"序号", "歌曲信息"})
		for i, s := range req.Songs {
			writer.Write([]string{strconv.Itoa(i + 1), s})
		}
		writer.Flush()
		return
	}

	// 默认为纯文本 .txt
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.txt", url.QueryEscape(filename)))
	for _, s := range req.Songs {
		w.Write([]byte(s + "\n"))
	}
}

// ----------------- 飞牛音乐相关接口 -----------------

func (s *Server) handleFeiNiuConnect(w http.ResponseWriter, r *http.Request) {
	var req feiniu.ConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json request", 400)
		return
	}

	loginData, err := s.feiniuClient.SetAuth(r.Context(), req.ServerURL, req.Username, req.Password, req.AccessCode)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"username":   loginData.Username,
		"user_token": loginData.UserToken,
	})
}

func (s *Server) handleFeiNiuStatus(w http.ResponseWriter, r *http.Request) {
	status := s.feiniuClient.GetStatus(r.Context())
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleFeiNiuDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := s.feiniuClient.Disconnect(r.Context()); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleFeiNiuPlaylists(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 50
	}

	playlists, err := s.feiniuClient.GetPlaylists(r.Context(), page, size)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    playlists,
	})
}

func (s *Server) handleFeiNiuPlaylistTracks(w http.ResponseWriter, r *http.Request) {
	guid := r.URL.Query().Get("guid")
	if guid == "" {
		http.Error(w, "missing guid", 400)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))

	tracks, err := s.feiniuClient.GetPlaylistTracks(r.Context(), guid, page, size)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    tracks,
	})
}

func (s *Server) handleFeiNiuPlaylistCreate(w http.ResponseWriter, r *http.Request) {
	var req feiniu.PlaylistCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	pl, err := s.feiniuClient.CreatePlaylist(r.Context(), req.Name, req.CoverID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    pl,
	})
}

func (s *Server) handleFeiNiuPlaylistEdit(w http.ResponseWriter, r *http.Request) {
	var req feiniu.PlaylistEditReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	if err := s.feiniuClient.EditPlaylist(r.Context(), req.GUID, req.Name, req.CoverID); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleFeiNiuPlaylistDelete(w http.ResponseWriter, r *http.Request) {
	var req feiniu.PlaylistDeleteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	if err := s.feiniuClient.DeletePlaylist(r.Context(), req.GUID); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleFeiNiuPlaylistAddTracks(w http.ResponseWriter, r *http.Request) {
	var req feiniu.PlaylistTracksActionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	if err := s.feiniuClient.AddTracksToPlaylist(r.Context(), req.GUID, req.TrackGUIDs); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleFeiNiuPlaylistRemoveTracks(w http.ResponseWriter, r *http.Request) {
	var req feiniu.PlaylistTracksActionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	if err := s.feiniuClient.RemoveTracksFromPlaylist(r.Context(), req.GUID, req.TrackGUIDs); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleFeiNiuPlaylistPurge(w http.ResponseWriter, r *http.Request) {
	var req feiniu.PlaylistDeleteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	if err := s.feiniuClient.PurgeInvalidTracks(r.Context(), req.GUID); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleFeiNiuPlaylistImport(w http.ResponseWriter, r *http.Request) {
	var req feiniu.ImportPlaylistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}

	result, err := s.fnImporter.ImportPlaylist(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   err.Error(),
			"data":    result,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    result,
	})
}

func (s *Server) handleFeiNiuCover(w http.ResponseWriter, r *http.Request) {
	coverID := r.URL.Query().Get("coverId")
	if coverID == "" {
		http.Error(w, "missing coverId", 400)
		return
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))

	reader, contentType, err := s.feiniuClient.GetCoverReader(r.Context(), coverID, size)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	io.Copy(w, reader)
}


