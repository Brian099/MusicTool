package playlist

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	qqMusicAPIURL              = "https://u6.y.qq.com/cgi-bin/musics.fcg?sign=%s&_=%d"
	qqMusicErrorResponseLength = 108
	qqMusicPageSize            = 30
	qqMusicMaxSongs            = 10000
)

var (
	qqPlaylistLinkRegex = regexp.MustCompile(`.*playlist/(\d+)`)
	qqIDParamRegex      = regexp.MustCompile(`id=(\d+)`)
	qqDetailsParamRegex = regexp.MustCompile(`(?:tid|id)=([0-9]+)`)
	qqShortLinkRegex    = regexp.MustCompile(`(fcgi-bin|i\.y\.qq\.com|c6\.y\.qq\.com)`)
)

type qqMusicFullResp struct {
	Code int `json:"code"`
	Req0 struct {
		Code int `json:"code"`
		Data struct {
			Dirinfo struct {
				Title   string `json:"title"`
				Songnum int    `json:"songnum"`
			} `json:"dirinfo"`
			Songlist []struct {
				Name   string `json:"name"`
				Singer []struct {
					Name string `json:"name"`
				} `json:"singer"`
			} `json:"songlist"`
		} `json:"data"`
	} `json:"req_0"`
}

func parseQQMusic(link string, detailed bool) (string, []SongItem, error) {
	tid, err := extractQQPlaylistID(link)
	if err != nil || tid == 0 {
		return "", nil, fmt.Errorf("解析QQ音乐歌单ID失败: %w", err)
	}

	// 1. 获取第一页并得到总数
	firstPageData, err := fetchQQPlaylistPage(tid, 0, qqMusicPageSize)
	if err != nil {
		return "", nil, fmt.Errorf("获取QQ音乐歌单第一页失败: %w", err)
	}

	var firstResp qqMusicFullResp
	if err := json.Unmarshal(firstPageData, &firstResp); err != nil {
		return "", nil, fmt.Errorf("解析QQ音乐数据失败: %w", err)
	}

	title := firstResp.Req0.Data.Dirinfo.Title
	totalSongs := firstResp.Req0.Data.Dirinfo.Songnum
	if totalSongs == 0 {
		totalSongs = len(firstResp.Req0.Data.Songlist)
	}

	if totalSongs > qqMusicMaxSongs {
		totalSongs = qqMusicMaxSongs
	}

	var allSongs []struct {
		Name   string `json:"name"`
		Singer []struct {
			Name string `json:"name"`
		} `json:"singer"`
	}
	allSongs = append(allSongs, firstResp.Req0.Data.Songlist...)

	// 2. 如果超过第一页，分页拉取后续所有歌曲
	if totalSongs > qqMusicPageSize {
		pageCount := (totalSongs + qqMusicPageSize - 1) / qqMusicPageSize
		for page := 1; page < pageCount; page++ {
			begin := page * qqMusicPageSize
			num := qqMusicPageSize
			if begin+num > totalSongs {
				num = totalSongs - begin
			}
			pData, pErr := fetchQQPlaylistPage(tid, begin, num)
			if pErr != nil {
				continue
			}
			var pResp qqMusicFullResp
			if err := json.Unmarshal(pData, &pResp); err == nil {
				allSongs = append(allSongs, pResp.Req0.Data.Songlist...)
			}
		}
	}

	// 3. 构建规范化的歌曲列表
	songItems := make([]SongItem, 0, len(allSongs))
	for idx, s := range allSongs {
		sName := s.Name
		if !detailed {
			sName = cleanSongName(sName)
		}
		singers := make([]string, len(s.Singer))
		for sIdx, singer := range s.Singer {
			singers[sIdx] = singer.Name
		}
		singerStr := strings.Join(singers, " / ")
		songItems = append(songItems, SongItem{
			Index:    idx + 1,
			SongName: sName,
			Artist:   singerStr,
			FullText: fmt.Sprintf("%s - %s", sName, singerStr),
		})
	}

	if title == "" {
		title = fmt.Sprintf("QQ音乐歌单_%d", tid)
	}

	return title, songItems, nil
}

func fetchQQPlaylistPage(tid int64, songBegin, songNum int) ([]byte, error) {
	platforms := []string{"-1", "android", "iphone", "h5", "wxfshare", "iphone_wx", "windows"}
	httpClient := &http.Client{Timeout: 12 * time.Second}

	var lastErr error
	for _, platform := range platforms {
		paramMap := map[string]any{
			"req_0": map[string]any{
				"module": "music.srfDissInfo.aiDissInfo",
				"method": "uniform_get_Dissinfo",
				"param": map[string]any{
					"disstid":      tid,
					"enc_host_uin": "",
					"tag":          1,
					"userinfo":     1,
					"song_begin":   songBegin,
					"song_num":     songNum,
					"onlysonglist": 0,
				},
			},
			"comm": map[string]any{
				"g_tk":     0,
				"uin":      "0",
				"format":   "json",
				"ct":       6,
				"cv":       0,
				"platform": platform,
			},
		}

		paramJSON, _ := json.Marshal(paramMap)
		paramStr := string(paramJSON)

		sign, err := getQQMusicSign(paramStr)
		if err != nil {
			lastErr = err
			continue
		}

		reqURL := fmt.Sprintf(qqMusicAPIURL, sign, time.Now().UnixMilli())
		req, err := http.NewRequest("POST", reqURL, strings.NewReader(paramStr))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if len(data) != qqMusicErrorResponseLength && len(data) > 50 {
			return data, nil
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("QQ音乐所有终端平台参数均未能获取到有效响应")
}

func extractQQPlaylistID(link string) (int64, error) {
	// 如果是短链接，跟随重定向
	if qqShortLinkRegex.MatchString(link) {
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

	if m := qqPlaylistLinkRegex.FindStringSubmatch(link); len(m) > 1 {
		return strconv.ParseInt(m[1], 10, 64)
	}
	if m := qqIDParamRegex.FindStringSubmatch(link); len(m) > 1 {
		return strconv.ParseInt(m[1], 10, 64)
	}
	if m := qqDetailsParamRegex.FindStringSubmatch(link); len(m) > 1 {
		return strconv.ParseInt(m[1], 10, 64)
	}

	return 0, errors.New("未能从链接中解析出QQ音乐歌单 ID (例如 ?id=123456 或 playlist/123456)")
}
