package config

import (
	"os"
	"path/filepath"
	"strconv"
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
	MusicDir   string
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

// LoadConfig 从环境变量与默认值加载配置
func LoadConfig() *Config {
	musicDir := getEnv("MUSIC_DIR", "/music")
	outputDir := getEnv("OUTPUT_DIR", "/output")
	dbPath := getEnv("DB_PATH", "data/music_toolkit.db")
	ffmpegPath := getEnv("FFMPEG_PATH", "ffmpeg")
	port := getEnvInt("PORT", 6826)
	workers := getEnvInt("MAX_WORKERS", 4)

	absMusicDir, _ := filepath.Abs(musicDir)
	absOutputDir, _ := filepath.Abs(outputDir)

	return &Config{
		MusicDir:   absMusicDir,
		OutputDir:  absOutputDir,
		DBPath:     dbPath,
		FFmpegPath: ffmpegPath,
		Port:       port,
		MaxWorkers: workers,
	}
}
