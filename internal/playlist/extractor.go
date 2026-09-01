package playlist

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	neteaseURLRegx = regexp.MustCompile(`(163cn)|(\.163\.)`)
	qqMusicURLRegx = regexp.MustCompile(`(\.qq\.)`)
	qishuiURLRegx  = regexp.MustCompile(`(qishui)|(douyin)`)

	// 匹配括号修饰字符：(Live), [Remix], （feat. xxx）等
	bracketRegex = regexp.MustCompile(`[([（【].*?[)\]）】]`)
)

// cleanSongName 去除歌曲名中的多余括号和辅助标签
func cleanSongName(name string) string {
	cleaned := bracketRegex.ReplaceAllString(name, "")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return strings.TrimSpace(name)
	}
	return cleaned
}

// Extractor 歌单解析器
type Extractor struct{}

func NewExtractor() *Extractor {
	return &Extractor{}
}

// Parse 解析歌单链接并按要求格式化
func (e *Extractor) Parse(req ParseRequest) (*ParseResult, error) {
	link := strings.TrimSpace(req.URL)
	if link == "" {
		return nil, errors.New("歌单链接不能为空")
	}

	start := time.Now()

	var platform string
	var title string
	var songs []SongItem
	var err error

	switch {
	case neteaseURLRegx.MatchString(link):
		platform = "netease"
		title, songs, err = parseNetEase(link, req.Detailed)
	case qqMusicURLRegx.MatchString(link):
		platform = "qq"
		title, songs, err = parseQQMusic(link, req.Detailed)
	case qishuiURLRegx.MatchString(link):
		platform = "qishui"
		title, songs, err = parseQiShuiMusic(link, req.Detailed)
	default:
		return nil, errors.New("未识别的音乐链接格式，目前支持网易云音乐、QQ音乐与汽水音乐歌单")
	}

	if err != nil {
		return nil, err
	}

	// 1. 处理格式 (Format)
	formatSongItems(songs, req.Format)

	// 2. 处理顺序 (Order)
	if req.Order == "reverse" {
		for i, j := 0, len(songs)-1; i < j; i, j = i+1, j-1 {
			songs[i], songs[j] = songs[j], songs[i]
		}
		for i := range songs {
			songs[i].Index = i + 1
		}
	}

	// 3. 构建导出纯文本
	textList := make([]string, len(songs))
	for i, s := range songs {
		textList[i] = s.FullText
	}
	rawText := strings.Join(textList, "\n")

	elapsed := float64(time.Since(start).Milliseconds()) / 1000.0

	return &ParseResult{
		Platform:  platform,
		SourceURL: link,
		Title:     title,
		SongCount: len(songs),
		Songs:     songs,
		TextList:  textList,
		RawText:   rawText,
		ParseTime: elapsed,
	}, nil
}

func formatSongItems(songs []SongItem, format string) {
	for i := range songs {
		sName := songs[i].SongName
		sArtist := songs[i].Artist

		switch format {
		case "singer-song":
			if sArtist != "" {
				songs[i].FullText = fmt.Sprintf("%s - %s", sArtist, sName)
			} else {
				songs[i].FullText = sName
			}
		case "song":
			songs[i].FullText = sName
		default: // "song-singer"
			if sArtist != "" {
				songs[i].FullText = fmt.Sprintf("%s - %s", sName, sArtist)
			} else {
				songs[i].FullText = sName
			}
		}
	}
}
