package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultAudioExtensions 默认常见音频后缀集合
var DefaultAudioExtensions = map[string]bool{
	".mp3":  true,
	".m4a":  true,
	".aac":  true,
	".flac": true,
	".wav":  true,
	".ogg":  true,
	".opus": true,
	".wma":  true,
	".ape":  true,
	".alac": true,
	".aif":  true,
	".aiff": true,
	".dsf":  true,
	".dff":  true,
	".mp4":  true,
	".m4b":  true,
	".wv":   true,
	".mpc":  true,
	".ac3":  true,
	".dts":  true,
}

// Config 运行时全局配置
type Config struct {
	MusicDir   string   // 主音乐目录 (首个有效路径)
	MusicDirs  []string // 所有已配置/已发现的音乐目录列表
	OutputDir  string
	DBPath     string
	FFmpegPath string
	Port       int
	MaxWorkers int
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

// ParsePathList 智能拆分多路径字符串 (支持逗号、分号、或 Linux 下的冒号)
func ParsePathList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	var rawItems []string
	// 先按分号和逗号切分
	semiParts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ','
	})

	for _, p := range semiParts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// 如果包含冒号，且不是类似 C:\ 或 D:\ 的 Windows 盘符
		if strings.Contains(p, ":") {
			colonParts := strings.Split(p, ":")
			var buf string
			for _, cp := range colonParts {
				// Windows 盘符判定 (单个英文字母)
				if len(cp) == 1 && ((cp[0] >= 'a' && cp[0] <= 'z') || (cp[0] >= 'A' && cp[0] <= 'Z')) {
					buf = cp + ":"
				} else {
					if buf != "" {
						rawItems = append(rawItems, buf+cp)
						buf = ""
					} else {
						if strings.TrimSpace(cp) != "" {
							rawItems = append(rawItems, strings.TrimSpace(cp))
						}
					}
				}
			}
			if buf != "" {
				rawItems = append(rawItems, buf)
			}
		} else {
			rawItems = append(rawItems, p)
		}
	}

	seen := make(map[string]bool)
	var result []string
	for _, item := range rawItems {
		item = strings.Trim(strings.TrimSpace(item), "\"'")
		if item == "" {
			continue
		}
		abs, err := filepath.Abs(item)
		if err != nil {
			abs = item
		}
		if !seen[abs] {
			seen[abs] = true
			result = append(result, abs)
		}
	}
	return result
}

// LoadConfig 从环境变量与默认值加载配置
func LoadConfig() *Config {
	// 收集各个可能的环境变量
	var allDirStrings []string
	for _, envKey := range []string{"MUSIC_DIRS", "TRIM_DATA_ACCESSIBLE_PATHS", "TRIM_DATA_SHARE_PATHS", "MUSIC_DIR"} {
		if val := os.Getenv(envKey); val != "" {
			allDirStrings = append(allDirStrings, val)
		}
	}

	var discoveredDirs []string
	seen := make(map[string]bool)

	for _, raw := range allDirStrings {
		for _, p := range ParsePathList(raw) {
			if !seen[p] {
				seen[p] = true
				discoveredDirs = append(discoveredDirs, p)
			}
		}
	}

	// 如果没有配置任何有效目录，默认使用 /music 或当前 ./data
	if len(discoveredDirs) == 0 {
		discoveredDirs = append(discoveredDirs, "/music")
	}

	// 检查目录是否存在，若存在且包含子目录（如 Docker 子卷挂载），自动将子目录加入候选
	var finalDirs []string
	for _, dir := range discoveredDirs {
		finalDirs = append(finalDirs, dir)
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			entries, err := os.ReadDir(dir)
			if err == nil {
				for _, entry := range entries {
					if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") && !strings.HasPrefix(entry.Name(), "@") {
						subPath := filepath.Join(dir, entry.Name())
						if !seen[subPath] {
							seen[subPath] = true
							finalDirs = append(finalDirs, subPath)
						}
					}
				}
			}
		}
	}

	outputDir := getEnv("OUTPUT_DIR", "/output")
	dbPath := getEnv("DB_PATH", "data/music_toolkit.db")
	ffmpegPath := getEnv("FFMPEG_PATH", "ffmpeg")
	port := getEnvInt("PORT", 6826)
	workers := getEnvInt("MAX_WORKERS", 4)

	absOutputDir, _ := filepath.Abs(outputDir)

	return &Config{
		MusicDir:   finalDirs[0],
		MusicDirs:  finalDirs,
		OutputDir:  absOutputDir,
		DBPath:     dbPath,
		FFmpegPath: ffmpegPath,
		Port:       port,
		MaxWorkers: workers,
	}
}
