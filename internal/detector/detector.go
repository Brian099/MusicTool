package detector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// AudioMetadata 音频元数据
type AudioMetadata struct {
	Duration   float64 `json:"duration"`
	Bitrate    int     `json:"bitrate"`
	SampleRate int     `json:"sample_rate"`
	Channels   int     `json:"channels"`
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	Album      string  `json:"album"`
}

// DetectionResult 格式检测结果
type DetectionResult struct {
	FilePath       string        `json:"file_path"`
	FileName       string        `json:"file_name"`
	CurrentExt     string        `json:"current_ext"`
	DetectedFormat string        `json:"detected_format"`
	SuggestedExt   string        `json:"suggested_ext"`
	IsMismatch     bool          `json:"is_mismatch"`
	IsAudio        bool          `json:"is_audio"`
	Details        string        `json:"details"`
	Metadata       AudioMetadata `json:"metadata"`
}

// FormatValidExtensions 合法后缀映射
var FormatValidExtensions = map[string]map[string]bool{
	"mp3":      {".mp3": true},
	"m4a":      {".m4a": true, ".aac": true, ".mp4": true, ".m4b": true, ".m4p": true, ".alac": true},
	"flac":     {".flac": true},
	"wav":      {".wav": true, ".wave": true},
	"ogg":      {".ogg": true, ".oga": true},
	"opus":     {".opus": true, ".ogg": true},
	"wma":      {".wma": true, ".asf": true},
	"ape":      {".ape": true},
	"aiff":     {".aiff": true, ".aif": true, ".aifc": true},
	"wavpack":  {".wv": true},
	"musepack": {".mpc": true, ".mp+": true},
	"dsf":      {".dsf": true},
	"dff":      {".dff": true},
	"ac3":      {".ac3": true},
	"dts":      {".dts": true},
}

// FormatPrimaryExtension 推荐主后缀
var FormatPrimaryExtension = map[string]string{
	"mp3":      ".mp3",
	"m4a":      ".m4a",
	"flac":     ".flac",
	"wav":      ".wav",
	"ogg":      ".ogg",
	"opus":     ".opus",
	"wma":      ".wma",
	"ape":      ".ape",
	"aiff":     ".aiff",
	"wavpack":  ".wv",
	"musepack": ".mpc",
	"dsf":      ".dsf",
	"dff":      ".dff",
	"ac3":      ".ac3",
	"dts":      ".dts",
}

// DetectByMagicBytes 通过文件头魔数探测格式
func DetectByMagicBytes(header []byte) (formatKey string, details string, ok bool) {
	if len(header) < 4 {
		return "", "", false
	}

	headerLower := bytes.ToLower(header)
	if len(headerLower) > 64 {
		headerLower = headerLower[:64]
	}

	// 网页或错误文本伪装检查
	if bytes.HasPrefix(headerLower, []byte("<!doctype html")) ||
		bytes.HasPrefix(headerLower, []byte("<html")) ||
		bytes.HasPrefix(headerLower, []byte("<?xml")) ||
		bytes.HasPrefix(headerLower, []byte("{\n")) ||
		bytes.HasPrefix(headerLower, []byte("{\"error")) ||
		bytes.HasPrefix(headerLower, []byte("{\"code")) {
		return "corrupt_or_text", "文本或 HTML 伪装（通常是下载失败或防盗链报错页面）", true
	}

	// FLAC: 'fLaC'
	if bytes.HasPrefix(header, []byte("fLaC")) {
		return "flac", "FLAC 无损音频", true
	}

	// WAV: 'RIFF....WAVE'
	if bytes.HasPrefix(header, []byte("RIFF")) && len(header) >= 12 && string(header[8:12]) == "WAVE" {
		return "wav", "RIFF WAVE 波形音频", true
	}

	// AIFF: 'FORM....AIFF' or 'FORM....AIFC'
	if bytes.HasPrefix(header, []byte("FORM")) && len(header) >= 12 && (string(header[8:12]) == "AIFF" || string(header[8:12]) == "AIFC") {
		return "aiff", "AIFF 音频", true
	}

	// Ogg
	if bytes.HasPrefix(header, []byte("OggS")) {
		if len(header) >= 36 && bytes.Contains(header[28:36], []byte("OpusHead")) {
			return "opus", "Ogg Opus 音频流", true
		}
		return "ogg", "Ogg Vorbis/FLAC 容器", true
	}

	// MP4/M4A 容器: '....ftyp'
	if len(header) >= 8 && string(header[4:8]) == "ftyp" {
		brand := ""
		if len(header) >= 12 {
			brand = string(header[8:12])
		}
		return "m4a", fmt.Sprintf("MP4/M4A 容器 (ISO Base Media, brand: %s)", brand), true
	}

	// MP3 ID3v2: 'ID3' 或 MPEG 同步字 0xFFEx
	if bytes.HasPrefix(header, []byte("ID3")) || (len(header) >= 2 && header[0] == 0xFF && (header[1]&0xE0) == 0xE0) {
		return "mp3", "MPEG Audio Layer 3 (MP3)", true
	}

	// APE: 'MAC '
	if bytes.HasPrefix(header, []byte("MAC ")) {
		return "ape", "Monkey's Audio (APE)", true
	}

	// WavPack: 'wvpk'
	if bytes.HasPrefix(header, []byte("wvpk")) {
		return "wavpack", "WavPack 无损音频", true
	}

	// DSF: 'DSD '
	if bytes.HasPrefix(header, []byte("DSD ")) {
		return "dsf", "DSD Stream File (DSF)", true
	}

	// WMA/ASF
	wmaGuid := []byte{0x30, 0x26, 0xb2, 0x75, 0x8e, 0x66, 0xcf, 0x11}
	if bytes.HasPrefix(header, wmaGuid) {
		return "wma", "Windows Media Audio (ASF/WMA)", true
	}

	return "", "", false
}

// Detector 音频检测器
type Detector struct {
	hasFFprobe bool
}

// NewDetector 创建检测器
func NewDetector() *Detector {
	_, err := exec.LookPath("ffprobe")
	return &Detector{
		hasFFprobe: err == nil,
	}
}

// CheckFile 全面检测单个文件的真实格式与一致性
func (d *Detector) CheckFile(filePath string) DetectionResult {
	fileName := filepath.Base(filePath)
	currentExt := strings.ToLower(filepath.Ext(filePath))

	// 1. 读取头部 1024 字节魔数
	f, err := os.Open(filePath)
	if err != nil {
		return DetectionResult{
			FilePath:       filePath,
			FileName:       fileName,
			CurrentExt:     currentExt,
			DetectedFormat: "error",
			SuggestedExt:   currentExt,
			IsMismatch:     true,
			IsAudio:        false,
			Details:        fmt.Sprintf("无法读取文件: %v", err),
		}
	}
	header := make([]byte, 1024)
	n, _ := f.Read(header)
	f.Close()
	header = header[:n]

	// 检查魔数
	magicFmt, magicDetails, ok := DetectByMagicBytes(header)
	if ok && magicFmt == "corrupt_or_text" {
		return DetectionResult{
			FilePath:       filePath,
			FileName:       fileName,
			CurrentExt:     currentExt,
			DetectedFormat: "corrupt_or_text",
			SuggestedExt:   ".txt",
			IsMismatch:     true,
			IsAudio:        false,
			Details:        magicDetails,
		}
	}

	// 2. 使用 FFprobe 提取精准音频流与标签
	if d.hasFFprobe {
		ffprobeFmt, ffDetails, meta, probeOk := d.detectWithFFprobe(filePath)
		if probeOk {
			validExts := FormatValidExtensions[ffprobeFmt]
			isMismatch := !validExts[currentExt]
			suggested := FormatPrimaryExtension[ffprobeFmt]
			if suggested == "" {
				suggested = currentExt
			}

			return DetectionResult{
				FilePath:       filePath,
				FileName:       fileName,
				CurrentExt:     currentExt,
				DetectedFormat: ffprobeFmt,
				SuggestedExt:   suggested,
				IsMismatch:     isMismatch,
				IsAudio:        true,
				Details:        ffDetails,
				Metadata:       meta,
			}
		}
	}

	// 3. 兜底魔数
	if ok && magicFmt != "" {
		validExts := FormatValidExtensions[magicFmt]
		isMismatch := !validExts[currentExt]
		suggested := FormatPrimaryExtension[magicFmt]
		if suggested == "" {
			suggested = currentExt
		}
		return DetectionResult{
			FilePath:       filePath,
			FileName:       fileName,
			CurrentExt:     currentExt,
			DetectedFormat: magicFmt,
			SuggestedExt:   suggested,
			IsMismatch:     isMismatch,
			IsAudio:        true,
			Details:        magicDetails,
		}
	}

	// 4. 未知或损坏
	return DetectionResult{
		FilePath:       filePath,
		FileName:       fileName,
		CurrentExt:     currentExt,
		DetectedFormat: "unknown",
		SuggestedExt:   currentExt,
		IsMismatch:     true,
		IsAudio:        false,
		Details:        "无法识别为有效的音频格式（可能已损坏或非音频）",
	}
}

type ffprobeOutput struct {
	Format struct {
		FormatName string            `json:"format_name"`
		Duration   string            `json:"duration"`
		BitRate    string            `json:"bit_rate"`
		Tags       map[string]string `json:"tags"`
	} `json:"format"`
	Streams []struct {
		CodecType  string `json:"codec_type"`
		CodecName  string `json:"codec_name"`
		SampleRate string `json:"sample_rate"`
		Channels   int    `json:"channels"`
		BitRate    string `json:"bit_rate"`
		Duration   string `json:"duration"`
	} `json:"streams"`
}

func (d *Detector) detectWithFFprobe(filePath string) (string, string, AudioMetadata, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", filePath)
	out, err := cmd.Output()
	if err != nil {
		return "", "", AudioMetadata{}, false
	}

	var data ffprobeOutput
	if err := json.Unmarshal(out, &data); err != nil {
		return "", "", AudioMetadata{}, false
	}

	// 查找音频流
	var audioStream *struct {
		CodecType  string `json:"codec_type"`
		CodecName  string `json:"codec_name"`
		SampleRate string `json:"sample_rate"`
		Channels   int    `json:"channels"`
		BitRate    string `json:"bit_rate"`
		Duration   string `json:"duration"`
	}
	for i := range data.Streams {
		if data.Streams[i].CodecType == "audio" {
			audioStream = &data.Streams[i]
			break
		}
	}

	if audioStream == nil {
		return "", "", AudioMetadata{}, false
	}

	var meta AudioMetadata
	dur, _ := strconv.ParseFloat(data.Format.Duration, 64)
	if dur == 0 && audioStream.Duration != "" {
		dur, _ = strconv.ParseFloat(audioStream.Duration, 64)
	}
	meta.Duration = dur

	br, _ := strconv.Atoi(data.Format.BitRate)
	if br == 0 && audioStream.BitRate != "" {
		br, _ = strconv.Atoi(audioStream.BitRate)
	}
	meta.Bitrate = br

	sr, _ := strconv.Atoi(audioStream.SampleRate)
	meta.SampleRate = sr
	meta.Channels = audioStream.Channels

	if data.Format.Tags != nil {
		for k, v := range data.Format.Tags {
			kLower := strings.ToLower(k)
			if kLower == "title" {
				meta.Title = v
			} else if kLower == "artist" {
				meta.Artist = v
			} else if kLower == "album" {
				meta.Album = v
			}
		}
	}

	codec := strings.ToLower(audioStream.CodecName)
	fmtName := strings.ToLower(data.Format.FormatName)

	formatKey := codec
	details := fmt.Sprintf("FFprobe 探测: %s (codec: %s)", strings.ToUpper(codec), codec)

	if strings.Contains(codec, "mp3") || strings.Contains(fmtName, "mp3") {
		formatKey = "mp3"
		details = "FFprobe 探测: MP3 音频"
	} else if strings.Contains(codec, "aac") || strings.Contains(codec, "alac") || strings.Contains(fmtName, "mp4") || strings.Contains(fmtName, "mov") || strings.Contains(fmtName, "m4a") {
		formatKey = "m4a"
		details = fmt.Sprintf("FFprobe 探测: M4A/AAC/ALAC (codec: %s)", codec)
	} else if strings.Contains(codec, "flac") {
		formatKey = "flac"
		details = "FFprobe 探测: FLAC 无损音频"
	} else if strings.Contains(fmtName, "wav") || strings.Contains(codec, "pcm_") {
		formatKey = "wav"
		details = "FFprobe 探测: WAV/PCM 音频"
	} else if strings.Contains(codec, "opus") {
		formatKey = "opus"
		details = "FFprobe 探测: Opus 音频"
	} else if strings.Contains(codec, "vorbis") || strings.Contains(fmtName, "ogg") {
		formatKey = "ogg"
		details = "FFprobe 探测: OGG 音频"
	} else if strings.Contains(codec, "wmav") || strings.Contains(fmtName, "asf") {
		formatKey = "wma"
		details = "FFprobe 探测: WMA 音频"
	} else if strings.Contains(codec, "ape") {
		formatKey = "ape"
		details = "FFprobe 探测: APE (Monkey's Audio)"
	}

	return formatKey, details, meta, true
}
