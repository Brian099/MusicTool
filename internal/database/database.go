package database

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

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

// PlaylistHistoryRecord 歌单提取历史记录
type PlaylistHistoryRecord struct {
	ID        int64  `json:"id"`
	Platform  string `json:"platform"`
	SourceURL string `json:"source_url"`
	Title     string `json:"title"`
	SongCount int    `json:"song_count"`
	SongsJSON string `json:"songs_json"`
	CreatedAt int64  `json:"created_at"`
}

// FeiNiuConfigRecord 飞牛 NAS 连接与鉴权凭据记录
type FeiNiuConfigRecord struct {
	ID           int    `json:"id"`
	ServerURL    string `json:"server_url"`
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	DeviceID     string `json:"device_id"`
	AccessCode   string `json:"access_code"`
	UserToken    string `json:"user_token"`
	UpdatedAt    int64  `json:"updated_at"`
}

// SystemUser 系统本地管理员账户记录
type SystemUser struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Salt         string `json:"-"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
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

	CREATE TABLE IF NOT EXISTS playlist_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		platform TEXT,
		source_url TEXT,
		title TEXT,
		song_count INTEGER,
		songs_json TEXT,
		created_at INTEGER
	);
	CREATE INDEX IF NOT EXISTS idx_playlist_created ON playlist_history(created_at DESC);

	CREATE TABLE IF NOT EXISTS feiniu_config (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		server_url TEXT,
		username TEXT,
		password_hash TEXT,
		device_id TEXT,
		access_code TEXT,
		user_token TEXT,
		updated_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS system_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		salt TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
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

// DeleteFormatRecord 删除单条格式检测记录
func (d *DB) DeleteFormatRecord(ctx context.Context, filePath string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.ExecContext(ctx, "DELETE FROM format_records WHERE file_path = ?", filePath)
	return err
}

// DeleteFormatRecords 批量删除格式检测记录
func (d *DB) DeleteFormatRecords(ctx context.Context, filePaths []string) error {
	if len(filePaths) == 0 {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, fp := range filePaths {
		d.db.ExecContext(ctx, "DELETE FROM format_records WHERE file_path = ?", fp)
	}
	return nil
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

// SavePlaylistHistory 保存歌单提取历史
func (d *DB) SavePlaylistHistory(ctx context.Context, r *PlaylistHistoryRecord) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `INSERT INTO playlist_history (platform, source_url, title, song_count, songs_json, created_at)
	          VALUES (?, ?, ?, ?, ?, ?)`
	res, err := d.db.ExecContext(ctx, query, r.Platform, r.SourceURL, r.Title, r.SongCount, r.SongsJSON, r.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListPlaylistHistory 获取歌单提取历史列表（默认不返回过大的 songs_json，或保留前 N 条）
func (d *DB) ListPlaylistHistory(ctx context.Context, limit int) ([]PlaylistHistoryRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, platform, source_url, title, song_count, songs_json, created_at 
	          FROM playlist_history ORDER BY created_at DESC LIMIT ?`
	rows, err := d.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PlaylistHistoryRecord
	for rows.Next() {
		var r PlaylistHistoryRecord
		if err := rows.Scan(&r.ID, &r.Platform, &r.SourceURL, &r.Title, &r.SongCount, &r.SongsJSON, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

// GetPlaylistHistory 获取单条歌单历史详情
func (d *DB) GetPlaylistHistory(ctx context.Context, id int64) (*PlaylistHistoryRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `SELECT id, platform, source_url, title, song_count, songs_json, created_at 
	          FROM playlist_history WHERE id = ?`
	row := d.db.QueryRowContext(ctx, query, id)

	var r PlaylistHistoryRecord
	if err := row.Scan(&r.ID, &r.Platform, &r.SourceURL, &r.Title, &r.SongCount, &r.SongsJSON, &r.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// DeletePlaylistHistory 删除单条歌单提取历史
func (d *DB) DeletePlaylistHistory(ctx context.Context, id int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.ExecContext(ctx, "DELETE FROM playlist_history WHERE id = ?", id)
	return err
}

// ClearPlaylistHistory 清空所有歌单历史
func (d *DB) ClearPlaylistHistory(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.ExecContext(ctx, "DELETE FROM playlist_history")
	return err
}

// GetFeiNiuConfig 获取飞牛 NAS 连接凭据配置
func (d *DB) GetFeiNiuConfig(ctx context.Context) (*FeiNiuConfigRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `SELECT id, server_url, username, password_hash, device_id, access_code, user_token, updated_at 
	          FROM feiniu_config WHERE id = 1`
	row := d.db.QueryRowContext(ctx, query)

	var r FeiNiuConfigRecord
	if err := row.Scan(&r.ID, &r.ServerURL, &r.Username, &r.PasswordHash, &r.DeviceID, &r.AccessCode, &r.UserToken, &r.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// SaveFeiNiuConfig 保存或更新飞牛 NAS 连接凭据配置
func (d *DB) SaveFeiNiuConfig(ctx context.Context, cfg *FeiNiuConfigRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `INSERT INTO feiniu_config (id, server_url, username, password_hash, device_id, access_code, user_token, updated_at)
	          VALUES (1, ?, ?, ?, ?, ?, ?, ?)
	          ON CONFLICT(id) DO UPDATE SET
	              server_url = excluded.server_url,
	              username = excluded.username,
	              password_hash = excluded.password_hash,
	              device_id = excluded.device_id,
	              access_code = excluded.access_code,
	              user_token = excluded.user_token,
	              updated_at = excluded.updated_at`
	_, err := d.db.ExecContext(ctx, query,
		cfg.ServerURL,
		cfg.Username,
		cfg.PasswordHash,
		cfg.DeviceID,
		cfg.AccessCode,
		cfg.UserToken,
		cfg.UpdatedAt,
	)
	return err
}

// ClearFeiNiuConfig 清空飞牛凭据配置
func (d *DB) ClearFeiNiuConfig(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.ExecContext(ctx, "DELETE FROM feiniu_config WHERE id = 1")
	return err
}

// ----------------- 系统本地用户管理 -----------------

// HashUserPassword 计算密码加盐 SHA-256 哈希值
func HashUserPassword(password, salt string) string {
	sum := sha256.Sum256([]byte(password + ":" + salt))
	return hex.EncodeToString(sum[:])
}

// GenerateSalt 生成 16 字节随机十六进制盐值
func GenerateSalt() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// CountUsers 统计系统中本地用户数量 (用于判断是否需要首次初始化创建账号)
func (d *DB) CountUsers(ctx context.Context) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var count int
	err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM system_users").Scan(&count)
	return count, err
}

// CreateUser 创建系统管理员或本地用户
func (d *DB) CreateUser(ctx context.Context, username, password string) (*SystemUser, error) {
	if username == "" {
		return nil, errors.New("用户名不能为空")
	}
	if len(password) < 4 {
		return nil, errors.New("密码长度不能少于 4 位")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	salt := GenerateSalt()
	hash := HashUserPassword(password, salt)
	now := time.Now().Unix()

	query := `INSERT INTO system_users (username, password_hash, salt, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`
	res, err := d.db.ExecContext(ctx, query, username, hash, salt, now, now)
	if err != nil {
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}
	id, _ := res.LastInsertId()
	return &SystemUser{
		ID:        id,
		Username:  username,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// GetUserByUsername 根据用户名获取用户详情
func (d *DB) GetUserByUsername(ctx context.Context, username string) (*SystemUser, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `SELECT id, username, password_hash, salt, created_at, updated_at FROM system_users WHERE username = ?`
	row := d.db.QueryRowContext(ctx, query, username)

	var u SystemUser
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Salt, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// VerifyUserPassword 验证用户名与原始密码，成功返回用户实例
func (d *DB) VerifyUserPassword(ctx context.Context, username, password string) (*SystemUser, error) {
	u, err := d.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, errors.New("用户名或密码错误")
	}

	expectedHash := HashUserPassword(password, u.Salt)
	if expectedHash != u.PasswordHash {
		return nil, errors.New("用户名或密码错误")
	}

	return u, nil
}



