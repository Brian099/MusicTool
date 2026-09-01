package playlist

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var (
	qishuiShareRegex = regexp.MustCompile(`https?://qishui\.douyin\.com/s/[a-zA-Z0-9]+/?`)
	urlRegex         = regexp.MustCompile(`https?://[a-zA-Z0-9./?=&_%-]+`)
)

func parseQiShuiMusic(link string, detailed bool) (string, []SongItem, error) {
	// 1. 提取短链接
	if match := qishuiShareRegex.FindString(link); match != "" {
		link = match
	} else if match := urlRegex.FindString(link); match != "" {
		link = match
	}

	httpClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
		Timeout: 15 * time.Second,
	}

	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		return "", nil, fmt.Errorf("创建汽水音乐请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("请求汽水音乐页面失败: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("解析汽水音乐网页失败: %w", err)
	}

	// 2. 提取歌单标题与作者
	title := strings.TrimSpace(doc.Find("#root > div > div > div > div > div:nth-child(1) > div:nth-child(3) > h1 > p").Text())
	author := strings.TrimSpace(doc.Find("#root > div > div > div > div > div:nth-child(1) > div:nth-child(3) > div > div > div:nth-child(2) > p").Text())

	if title == "" {
		// 备用选择器
		title = strings.TrimSpace(doc.Find("h1").First().Text())
	}
	if title == "" {
		title = "汽水音乐歌单"
	} else if author != "" {
		title = fmt.Sprintf("%s - %s", title, author)
	}

	// 3. 提取歌曲列表
	var songs []SongItem
	idx := 1

	// 主列表选择器
	doc.Find("#root > div > div > div > div > div:nth-child(2) > div > div > div > div > div").Each(func(i int, s *goquery.Selection) {
		songTitle := strings.TrimSpace(s.Find("div:nth-child(2) > div:nth-child(1) > p").Text())
		artist := strings.TrimSpace(s.Find("div:nth-child(2) > div:nth-child(2) > p").Text())

		if songTitle == "" {
			return
		}

		// 去除点号后面的额外字段（例如 "G.E.M. 邓紫棋 • T.I.M.E." -> "G.E.M. 邓紫棋"）
		if strings.Contains(artist, "•") {
			artist = strings.TrimSpace(strings.Split(artist, "•")[0])
		}

		if !detailed {
			songTitle = cleanSongName(songTitle)
		}

		songs = append(songs, SongItem{
			Index:    idx,
			SongName: songTitle,
			Artist:   artist,
			FullText: fmt.Sprintf("%s - %s", songTitle, artist),
		})
		idx++
	})

	if len(songs) == 0 {
		return "", nil, errors.New("未能从汽水音乐页面解析出歌曲列表，请检查链接是否为有效公开歌单")
	}

	return title, songs, nil
}
