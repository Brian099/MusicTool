package playlist

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	neteaseIDRegex      = regexp.MustCompile(`id=(\d+)`)
	neteaseShortLinkReg = regexp.MustCompile(`(163cn\.tv|y\.music\.163\.com)`)
)

type neteasePlaylistResp struct {
	Code     int `json:"code"`
	Playlist struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		TrackCount int    `json:"trackCount"`
		TrackIDs   []struct {
			ID int64 `json:"id"`
		} `json:"trackIds"`
	} `json:"playlist"`
}

type neteaseSongsResp struct {
	Code  int `json:"code"`
	Songs []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Ar   []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"ar"`
	} `json:"songs"`
}

// parseNetEase 解析网易云歌单
func parseNetEase(link string, detailed bool) (string, []SongItem, error) {
	playlistID, err := extractNetEaseID(link)
	if err != nil {
		return "", nil, fmt.Errorf("解析网易云歌单ID失败: %w", err)
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}

	// 1. 获取歌单详情与所有 trackIds
	reqData := url.Values{}
	reqData.Set("id", strconv.FormatInt(playlistID, 10))

	req, err := http.NewRequest("POST", "https://music.163.com/api/v6/playlist/detail", strings.NewReader(reqData.Encode()))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("请求网易云接口失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("读取网易云响应失败: %w", err)
	}

	var pResp neteasePlaylistResp
	if err := json.Unmarshal(bodyBytes, &pResp); err != nil {
		return "", nil, fmt.Errorf("解析网易云响应失败: %w", err)
	}

	if pResp.Code == 401 {
		return "", nil, errors.New("无权限访问该歌单（私密歌单不可见）")
	}
	if pResp.Code != 200 || pResp.Playlist.ID == 0 {
		return "", nil, fmt.Errorf("未找到网易云歌单数据 (code: %d)", pResp.Code)
	}

	trackIDs := pResp.Playlist.TrackIDs
	if len(trackIDs) == 0 {
		return pResp.Playlist.Name, []SongItem{}, nil
	}

	// 2. 分块批量拉取歌曲详情 (每批 400 首)
	const chunkSize = 400
	type songDetail struct {
		Name   string
		Artist string
	}
	detailMap := sync.Map{}

	var wg sync.WaitGroup
	errCh := make(chan error, (len(trackIDs)/chunkSize)+1)

	for i := 0; i < len(trackIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(trackIDs) {
			end = len(trackIDs)
		}
		chunk := trackIDs[i:end]

		wg.Add(1)
		go func(c []struct {
			ID int64 `json:"id"`
		}) {
			defer wg.Done()
			type idParam struct {
				ID int64 `json:"id"`
			}
			paramList := make([]idParam, len(c))
			for idx, item := range c {
				paramList[idx] = idParam{ID: item.ID}
			}
			paramJSON, _ := json.Marshal(paramList)

			val := url.Values{}
			val.Set("c", string(paramJSON))

			subReq, subErr := http.NewRequest("POST", "https://music.163.com/api/v3/song/detail", strings.NewReader(val.Encode()))
			if subErr != nil {
				errCh <- subErr
				return
			}
			subReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			subReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

			subResp, subErr := httpClient.Do(subReq)
			if subErr != nil {
				errCh <- subErr
				return
			}
			defer subResp.Body.Close()

			subBytes, subErr := io.ReadAll(subResp.Body)
			if subErr != nil {
				errCh <- subErr
				return
			}

			var sResp neteaseSongsResp
			if subErr := json.Unmarshal(subBytes, &sResp); subErr != nil {
				errCh <- subErr
				return
			}

			for _, s := range sResp.Songs {
				songName := s.Name
				if !detailed {
					songName = cleanSongName(songName)
				}
				artists := make([]string, len(s.Ar))
				for arIdx, ar := range s.Ar {
					artists[arIdx] = ar.Name
				}
				artistStr := strings.Join(artists, " / ")
				detailMap.Store(s.ID, songDetail{Name: songName, Artist: artistStr})
			}
		}(chunk)
	}

	wg.Wait()
	close(errCh)
	if err, ok := <-errCh; ok && err != nil {
		return "", nil, fmt.Errorf("批量拉取网易云歌曲详情失败: %w", err)
	}

	// 3. 按照原歌单顺序还原列表
	songs := make([]SongItem, 0, len(trackIDs))
	for idx, t := range trackIDs {
		if val, ok := detailMap.Load(t.ID); ok {
			d := val.(songDetail)
			songs = append(songs, SongItem{
				Index:    idx + 1,
				SongName: d.Name,
				Artist:   d.Artist,
				FullText: fmt.Sprintf("%s - %s", d.Name, d.Artist),
			})
		}
	}

	return pResp.Playlist.Name, songs, nil
}

// extractNetEaseID 从短链或标准链接提取歌单 ID
func extractNetEaseID(link string) (int64, error) {
	// 如果是短链或分享短页面，重定向获取实际 URL
	if neteaseShortLinkReg.MatchString(link) {
		clientNoRedirect := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: 10 * time.Second,
		}
		resp, err := clientNoRedirect.Get(link)
		if err == nil && resp != nil {
			if loc := resp.Header.Get("Location"); loc != "" {
				link = loc
			}
			resp.Body.Close()
		}
	}

	match := neteaseIDRegex.FindStringSubmatch(link)
	if len(match) > 1 {
		return strconv.ParseInt(match[1], 10, 64)
	}

	return 0, errors.New("无法从链接中识别出网易云歌单 ID (例如 ?id=123456)")
}
