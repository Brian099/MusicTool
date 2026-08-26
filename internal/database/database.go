package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// AudioFingerprintRecord 音频指纹记录
type AudioFingerprintRecord struct {
	FilePath    string  `json:"file_path"`
	FileName    string  `json:"file_name"`
	MTime       float64 `json:"mtime"`
	FileSize    int64   `json:"file_size"`
	Fingerprint string  `json:"fingerprint"`
	Duration    float64 `json:"duration"`
	Format      string  `json:"format"`
	Bitrate     int     `json:"bitrate"`
	SampleRate  int     `json:"sample_rate"`
	Channels    int     `json:"channels"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	Album       string  `json:"album"`
	AttemptedAt int64   `json:"attempted_at"`
	IsFailed    int     `json:"is_failed"`
	ErrorMsg    string  `json:"error_msg"`
}

// FormatRecord 格式检测记录
type FormatRecord struct {
	FilePath       string  `json:"file_path"`
	FileName       string  `json:"file_name"`
	MTime          float64 `json:"mtime"`
	FileSize       int64   `json:"file_size"`
	CurrentExt     string  `json:"current_ext"`
	DetectedFormat string  `json:"detected_format"`
	SuggestedExt   string  `json:"suggested_ext"`
	IsMismatch     int     `json:"is_mismatch"`
	IsAudio        int     `json:"is_audio"`
	Details        string  `json:"details"`
	Status         string  `json:"status"`
	UpdatedAt      int64   `json:"updated_at"`
}

// LosslessRecord 真假无损检测记录
type LosslessRecord struct {
	FilePath       string  `json:"file_path"`
	FileName       string  `json:"file_name"`
	MTime          float64 `json:"mtime"`
	FileSize       int64   `json:"file_size"`
	Format         string  `json:"format"`
	SampleRate     int     `json:"sample_rate"`
	Bitrate        int     `json:"bitrate"`
	Duration       float64 `json:"duration"`
	Grade          string  `json:"grade"`
	GradeText      string  `json:"grade_text"`
	CutoffFreqHz   int     `json:"cutoff_freq_hz"`
	HighFreqEnergy float64 `json:"high_freq_energy"`
	Confidence     int     `json:"confidence"`
	Details        string  `json:"details"`
	UpdatedAt      int64   `json:"updated_at"`
}

// DB 数据库操作句柄
type DB struct {
	db *sql.DB
	mu sync.Mutex
}

// OpenDB 初始化并打开 SQLite 数据库
func OpenDB(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// 限制连接池，SQLite 适合单写多读
	db.SetMaxOpenConns(1)

	d := &DB{db: db}
	if err := d.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return d, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS audio_fingerprints (
		file_path TEXT PRIMARY KEY,
		file_name TEXT,
		mtime REAL,
		file_size INTEGER,
		fingerprint TEXT,
		duration REAL,
		format TEXT,
		bitrate INTEGER,
		sample_rate INTEGER,
		channels INTEGER,
		title TEXT,
		artist TEXT,
		album TEXT,
		attempted_at INTEGER,
		is_failed INTEGER DEFAULT 0,
		error_msg TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_fp ON audio_fingerprints(fingerprint);
	CREATE INDEX IF NOT EXISTS idx_mtime ON audio_fingerprints(mtime);

	CREATE TABLE IF NOT EXISTS format_records (
		file_path TEXT PRIMARY KEY,
		file_name TEXT,
		mtime REAL,
		file_size INTEGER,
		current_ext TEXT,
		detected_format TEXT,
		suggested_ext TEXT,
		is_mismatch INTEGER,
		is_audio INTEGER,
		details TEXT,
		status TEXT,
		updated_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS lossless_records (
		file_path TEXT PRIMARY KEY,
		file_name TEXT,
		mtime REAL,
		file_size INTEGER,
		format TEXT,
		sample_rate INTEGER,
		bitrate INTEGER,
		duration REAL,
		grade TEXT,
		grade_text TEXT,
		cutoff_freq_hz INTEGER,
		high_freq_energy REAL,
		confidence INTEGER,
		details TEXT,
		updated_at INTEGER
	);
	`
	_, err := d.db.Exec(schema)
	return err
}

// GetFingerprint 获取单个文件的指纹缓存
func (d *DB) GetFingerprint(ctx context.Context, filePath string) (*AudioFingerprintRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `SELECT file_path, file_name, mtime, file_size, fingerprint, duration, format, 
	                 bitrate, sample_rate, channels, title, artist, album, attempted_at, is_failed, error_msg 
	          FROM audio_fingerprints WHERE file_path = ?`
	row := d.db.QueryRowContext(ctx, query, filePath)

	var r AudioFingerprintRecord
	err := row.Scan(
		&r.FilePath, &r.FileName, &r.MTime, &r.FileSize, &r.Fingerprint, &r.Duration, &r.Format,
		&r.Bitrate, &r.SampleRate, &r.Channels, &r.Title, &r.Artist, &r.Album, &r.AttemptedAt, &r.IsFailed, &r.ErrorMsg,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// UpsertFingerprint 插入或更新指纹记录
func (d *DB) UpsertFingerprint(ctx context.Context, r *AudioFingerprintRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `
	INSERT INTO audio_fingerprints (
		file_path, file_name, mtime, file_size, fingerprint, duration, format,
		bitrate, sample_rate, channels, title, artist, album, attempted_at, is_failed, error_msg
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(file_path) DO UPDATE SET
		file_name=excluded.file_name,
		mtime=excluded.mtime,
		file_size=excluded.file_size,
		fingerprint=excluded.fingerprint,
		duration=excluded.duration,
		format=excluded.format,
		bitrate=excluded.bitrate,
		sample_rate=excluded.sample_rate,
		channels=excluded.channels,
		title=excluded.title,
		artist=excluded.artist,
		album=excluded.album,
		attempted_at=excluded.attempted_at,
		is_failed=excluded.is_failed,
		error_msg=excluded.error_msg;
	`
	_, err := d.db.ExecContext(ctx, query,
		r.FilePath, r.FileName, r.MTime, r.FileSize, r.Fingerprint, r.Duration, r.Format,
		r.Bitrate, r.SampleRate, r.Channels, r.Title, r.Artist, r.Album, r.AttemptedAt, r.IsFailed, r.ErrorMsg,
	)
	return err
}

// ListAllValidFingerprints 列出所有有效的非空指纹记录
func (d *DB) ListAllValidFingerprints(ctx context.Context) ([]AudioFingerprintRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `SELECT file_path, file_name, mtime, file_size, fingerprint, duration, format, 
	                 bitrate, sample_rate, channels, title, artist, album, attempted_at, is_failed, error_msg 
	          FROM audio_fingerprints 
	          WHERE is_failed = 0 AND fingerprint != '' AND fingerprint IS NOT NULL
	          ORDER BY fingerprint, duration`
	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []AudioFingerprintRecord
	for rows.Next() {
		var r AudioFingerprintRecord
		if err := rows.Scan(
			&r.FilePath, &r.FileName, &r.MTime, &r.FileSize, &r.Fingerprint, &r.Duration, &r.Format,
			&r.Bitrate, &r.SampleRate, &r.Channels, &r.Title, &r.Artist, &r.Album, &r.AttemptedAt, &r.IsFailed, &r.ErrorMsg,
		); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

// ClearAllFingerprints 清空所有指纹
func (d *DB) ClearAllFingerprints(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.ExecContext(ctx, "DELETE FROM audio_fingerprints")
	return err
}

// ResetFailedFingerprints 重置失败指纹
func (d *DB) ResetFailedFingerprints(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.ExecContext(ctx, "UPDATE audio_fingerprints SET is_failed = 0, attempted_at = 0, error_msg = '' WHERE is_failed = 1")
	return err
}

// RemoveMissingFiles 清理不存在的文件
func (d *DB) RemoveMissingFiles(ctx context.Context, existingMap map[string]bool) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.QueryContext(ctx, "SELECT file_path FROM audio_fingerprints")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var toDelete []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err == nil {
			if !existingMap[p] {
				toDelete = append(toDelete, p)
			}
		}
	}

	for _, p := range toDelete {
		d.db.ExecContext(ctx, "DELETE FROM audio_fingerprints WHERE file_path = ?", p)
		d.db.ExecContext(ctx, "DELETE FROM format_records WHERE file_path = ?", p)
	}
	return len(toDelete), nil
}

// UpsertFormatRecord 插入格式记录
func (d *DB) UpsertFormatRecord(ctx context.Context, r *FormatRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `
	INSERT INTO format_records (
		file_path, file_name, mtime, file_size, current_ext, detected_format,
		suggested_ext, is_mismatch, is_audio, details, status, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(file_path) DO UPDATE SET
		file_name=excluded.file_name,
		mtime=excluded.mtime,
		file_size=excluded.file_size,
		current_ext=excluded.current_ext,
		detected_format=excluded.detected_format,
		suggested_ext=excluded.suggested_ext,
		is_mismatch=excluded.is_mismatch,
		is_audio=excluded.is_audio,
		details=excluded.details,
		status=excluded.status,
		updated_at=excluded.updated_at;
	`
	_, err := d.db.ExecContext(ctx, query,
		r.FilePath, r.FileName, r.MTime, r.FileSize, r.CurrentExt, r.DetectedFormat,
		r.SuggestedExt, r.IsMismatch, r.IsAudio, r.Details, r.Status, r.UpdatedAt,
	)
	return err
}

// ListFormatRecords 获取格式记录列表
func (d *DB) ListFormatRecords(ctx context.Context) ([]FormatRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.QueryContext(ctx, "SELECT file_path, file_name, mtime, file_size, current_ext, detected_format, suggested_ext, is_mismatch, is_audio, details, status, updated_at FROM format_records ORDER BY is_mismatch DESC, file_name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []FormatRecord
	for rows.Next() {
		var r FormatRecord
		if err := rows.Scan(
			&r.FilePath, &r.FileName, &r.MTime, &r.FileSize, &r.CurrentExt, &r.DetectedFormat,
			&r.SuggestedExt, &r.IsMismatch, &r.IsAudio, &r.Details, &r.Status, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

// ClearFormatRecords 清空格式记录
func (d *DB) ClearFormatRecords(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.ExecContext(ctx, "DELETE FROM format_records")
	return err
}

// UpsertLosslessRecord 插入或更新真假无损检测记录
func (d *DB) UpsertLosslessRecord(ctx context.Context, r *LosslessRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `
	INSERT INTO lossless_records (
		file_path, file_name, mtime, file_size, format, sample_rate, bitrate,
		duration, grade, grade_text, cutoff_freq_hz, high_freq_energy, confidence, details, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(file_path) DO UPDATE SET
		file_name=excluded.file_name,
		mtime=excluded.mtime,
		file_size=excluded.file_size,
		format=excluded.format,
		sample_rate=excluded.sample_rate,
		bitrate=excluded.bitrate,
		duration=excluded.duration,
		grade=excluded.grade,
		grade_text=excluded.grade_text,
		cutoff_freq_hz=excluded.cutoff_freq_hz,
		high_freq_energy=excluded.high_freq_energy,
		confidence=excluded.confidence,
		details=excluded.details,
		updated_at=excluded.updated_at;
	`
	_, err := d.db.ExecContext(ctx, query,
		r.FilePath, r.FileName, r.MTime, r.FileSize, r.Format, r.SampleRate, r.Bitrate,
		r.Duration, r.Grade, r.GradeText, r.CutoffFreqHz, r.HighFreqEnergy, r.Confidence, r.Details, r.UpdatedAt,
	)
	return err
}

// ListLosslessRecords 获取真假无损检测记录列表
func (d *DB) ListLosslessRecords(ctx context.Context) ([]LosslessRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `SELECT file_path, file_name, mtime, file_size, format, sample_rate, bitrate,
	                 duration, grade, grade_text, cutoff_freq_hz, high_freq_energy, confidence, details, updated_at
	          FROM lossless_records ORDER BY grade ASC, file_name ASC`
	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []LosslessRecord
	for rows.Next() {
		var r LosslessRecord
		if err := rows.Scan(
			&r.FilePath, &r.FileName, &r.MTime, &r.FileSize, &r.Format, &r.SampleRate, &r.Bitrate,
			&r.Duration, &r.Grade, &r.GradeText, &r.CutoffFreqHz, &r.HighFreqEnergy, &r.Confidence, &r.Details, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

// ClearLosslessRecords 清空真假无损检测记录
func (d *DB) ClearLosslessRecords(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.ExecContext(ctx, "DELETE FROM lossless_records")
	return err
}
