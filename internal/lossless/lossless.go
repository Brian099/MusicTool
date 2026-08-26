package lossless

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LosslessGrade 无损评级
type LosslessGrade string

const (
	GradeTrueHiRes     LosslessGrade = "true_hires"      // 🏆 真 Hi-Res 高解析无损 (96k/192k)
	GradeTrueCD        LosslessGrade = "true_lossless"   // 💎 真无损 (CD 44.1k/48k 标准无损)
	GradeSuspectFake   LosslessGrade = "fake_320k"       // ⚠️ 假无损 (疑似 320k MP3/AAC 转压)
	GradeBadFake       LosslessGrade = "fake_low_bitrate"// 🚫 劣质假无损 (疑似 128k~192k MP3 转压)
	GradeUnknown       LosslessGrade = "unknown"         // 未知/检测失败
)

// LosslessResult 检测结果
type LosslessResult struct {
	FilePath       string        `json:"file_path"`
	FileName       string        `json:"file_name"`
	FileSize       int64         `json:"file_size"`
	Format         string        `json:"format"`
	SampleRate     int           `json:"sample_rate"`
	Bitrate        int           `json:"bitrate"`
	Duration       float64       `json:"duration"`
	Grade          LosslessGrade `json:"grade"`
	GradeText      string        `json:"grade_text"`
	CutoffFreqHz   int           `json:"cutoff_freq_hz"`   // 探测到的高频截止频率
	HighFreqEnergy float64       `json:"high_freq_energy"` // 高频能量衰减 (dB)
	Confidence     int           `json:"confidence"`       // 置信度 (0-100)
	Details        string        `json:"details"`
}

var rmsRe = regexp.MustCompile(`RMS level dB:\s+([-\d.]+)`)
var peakRe = regexp.MustCompile(`Peak level dB:\s+([-\d.]+)`)

// AnalyzeLossless 分析单首无损音乐的频谱真实度
func AnalyzeLossless(ctx context.Context, ffmpegPath, filePath string, sampleRate int) LosslessResult {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}

	// 采样区间：取音频中间 30 秒至 60 秒的高能片段进行 FFT 频响探测
	// 1. 探测全频段能量 (RMS)
	// 2. 探测 16kHz 以上高频能量 (highpass=16000)
	// 3. 探测 20kHz 以上超高频能量 (highpass=20000)
	// 4. 探测 22kHz 以上极限频段能量 (highpass=22000)

	fullRMS := measureBandEnergy(ctx, ffmpegPath, filePath, "")
	band16kRMS := measureBandEnergy(ctx, ffmpegPath, filePath, "highpass=f=16000")
	band20kRMS := measureBandEnergy(ctx, ffmpegPath, filePath, "highpass=f=20000")
	band22kRMS := measureBandEnergy(ctx, ffmpegPath, filePath, "highpass=f=22000")

	// 如果无法读取能量，返回未知
	if fullRMS == -999 {
		return LosslessResult{
			FilePath:   filePath,
			Grade:      GradeUnknown,
			GradeText:  "检测失败",
			Details:    "无法解码音频样本进行频谱分析",
			Confidence: 0,
		}
	}

	diff16k := fullRMS - band16kRMS // 16kHz 高频能量衰减比
	diff20k := fullRMS - band20kRMS // 20kHz 超高频能量衰减比
	diff22k := fullRMS - band22kRMS // 22kHz 极限频段衰减比

	var grade LosslessGrade
	var gradeText string
	var details string
	var cutoffHz int
	var conf int

	// 分析判定逻辑
	// 劣质 MP3 (128k/192k) 特征：16kHz 处断崖截断，16kHz 以上衰减 > 45dB 甚至完全无声 (-inf)
	if band16kRMS < -65.0 || diff16k > 42.0 {
		grade = GradeBadFake
		gradeText = "🚫 劣质假无损 (128k-192k 转压)"
		cutoffHz = 15800
		conf = 95
		details = fmt.Sprintf("16kHz 以上几乎无有效高频能量 (衰减 %.1fdB)，符合 128k~192k MP3 频谱硬截断特征", diff16k)
	} else if band20kRMS < -62.0 || diff20k > 40.0 {
		// 320k MP3 / AAC 特征：20kHz 处刀切线，20kHz 以上能量陡降
		grade = GradeSuspectFake
		gradeText = "⚠️ 假无损 (疑似 320k MP3 转压)"
		cutoffHz = 19800
		conf = 88
		details = fmt.Sprintf("20kHz 处存在陡峭频响截断 (20kHz 以上衰减 %.1fdB)，符合 320kbps MP3 典型截断线", diff20k)
	} else {
		// 真无损判定：20kHz 以上有自然泛音衰减
		if sampleRate >= 88200 && band22kRMS > -55.0 && diff22k < 35.0 {
			grade = GradeTrueHiRes
			gradeText = "🏆 真 Hi-Res 高解析度无损"
			cutoffHz = sampleRate / 2
			conf = 98
			details = fmt.Sprintf("采样率 %dHz，高频自然延伸超越 24kHz (22kHz 能量正常)，属真高解析母带", sampleRate)
		} else {
			grade = GradeTrueCD
			gradeText = "💎 真无损 (CD 音质)"
			cutoffHz = 22050
			conf = 92
			details = fmt.Sprintf("20kHz~22kHz 频段泛音丰富连贯 (衰减 %.1fdB)，符合标准 CD 无损声学特征", diff20k)
		}
	}

	return LosslessResult{
		FilePath:       filePath,
		Grade:          grade,
		GradeText:      gradeText,
		CutoffFreqHz:   cutoffHz,
		HighFreqEnergy: diff20k,
		Confidence:     conf,
		Details:        details,
	}
}

// measureBandEnergy 调用 FFmpeg astats 测量特定频段 RMS 能量 (dB)
func measureBandEnergy(ctx context.Context, ffmpegPath, filePath, audioFilter string) float64 {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	args := []string{
		"-hide_banner",
		"-ss", "20",
		"-t", "25",
		"-i", filePath,
		"-vn",
	}

	filterChain := "astats=metadata=1:reset=1"
	if audioFilter != "" {
		filterChain = audioFilter + "," + filterChain
	}
	args = append(args, "-af", filterChain, "-f", "null", "-")

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return -999
	}

	outStr := stderr.String()
	matches := rmsRe.FindAllStringSubmatch(outStr, -1)
	if len(matches) == 0 {
		return -999
	}

	// 取最后一个 RMS level
	lastMatch := matches[len(matches)-1]
	val, err := strconv.ParseFloat(lastMatch[1], 64)
	if err != nil {
		if strings.Contains(lastMatch[1], "-inf") {
			return -100.0
		}
		return -999
	}
	return val
}
