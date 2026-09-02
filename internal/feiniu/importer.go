package feiniu

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"music-toolkit/internal/playlist"
)

// Importer 飞牛外部歌单智能导入引擎
type Importer struct {
	client *Client
}

// NewImporter 创建歌单导入器
func NewImporter(client *Client) *Importer {
	return &Importer{
		client: client,
	}
}

var (
	// 正则：去除括号及修饰词
	reBracketContent = regexp.MustCompile(`\s*[\(\[（【<][^\)\]）】>]*[\)\]）】>]\s*`)
	// 正则：去除常见版本后缀
	reNoiseSuffixes = regexp.MustCompile(`(?i)\s*(-|_|\s)\s*(live|remix|instrumental|cover|remaster|remastered|explicit|clean|伴奏|现场版|重制版|原声版|高保真|无损版)\b.*$`)
	// 正则：歌手分隔符
	reArtistSplit = regexp.MustCompile(`[/,&、+，]|(?i)\s+feat\.?\s+|\s+ft\.?\s+|\s+vs\.?\s+`)
)

// CleanSongTitle 清洗歌曲名称中的噪音修饰词与括号
func CleanSongTitle(title string) string {
	cleaned := strings.TrimSpace(title)
	cleaned = reBracketContent.ReplaceAllString(cleaned, " ")
	cleaned = reNoiseSuffixes.ReplaceAllString(cleaned, "")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if cleaned == "" {
		return strings.TrimSpace(title)
	}
	return cleaned
}

// ExtractPrimaryArtist 提取主歌手名
func ExtractPrimaryArtist(artist string) string {
	parts := reArtistSplit.Split(artist, -1)
	if len(parts) > 0 {
		first := strings.TrimSpace(parts[0])
		if first != "" {
			return first
		}
	}
	return strings.TrimSpace(artist)
}

// LevenshteinDistance 计算两个字符串的编辑距离
func LevenshteinDistance(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	len1, len2 := len(r1), len(r2)

	if len1 == 0 {
		return len2
	}
	if len2 == 0 {
		return len1
	}

	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
		matrix[i][0] = i
	}
	for j := 0; j <= len2; j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}
			minVal := matrix[i-1][j] + 1 // 删除
			if matrix[i][j-1]+1 < minVal {
				minVal = matrix[i][j-1] + 1 // 插入
			}
			if matrix[i-1][j-1]+cost < minVal {
				minVal = matrix[i-1][j-1] + cost // 替换
			}
			matrix[i][j] = minVal
		}
	}
	return matrix[len1][len2]
}

// calcTitleScore 计算歌名相似度 (0.0 ~ 1.0)
func calcTitleScore(qTitle, candidateTitle string) float64 {
	s1 := strings.ToLower(strings.TrimSpace(qTitle))
	s2 := strings.ToLower(strings.TrimSpace(candidateTitle))

	if s1 == s2 {
		return 1.0
	}

	c1 := strings.ToLower(CleanSongTitle(s1))
	c2 := strings.ToLower(CleanSongTitle(s2))
	if c1 == c2 && c1 != "" {
		return 0.95
	}

	// 子串包含检测
	if strings.Contains(s1, s2) || strings.Contains(s2, s1) || (c1 != "" && (strings.Contains(c1, c2) || strings.Contains(c2, c1))) {
		maxLen := max(len([]rune(s1)), len([]rune(s2)))
		if maxLen > 0 {
			minLen := min(len([]rune(s1)), len([]rune(s2)))
			return 0.80 + 0.15*float64(minLen)/float64(maxLen)
		}
	}

	// 编辑距离打分
	dist := LevenshteinDistance(s1, s2)
	maxLen := max(len([]rune(s1)), len([]rune(s2)))
	if maxLen == 0 {
		return 1.0
	}
	ratio := 1.0 - float64(dist)/float64(maxLen)
	if ratio < 0 {
		return 0
	}
	return ratio
}

// calcArtistScore 计算歌手相似度 (0.0 ~ 1.0)
func calcArtistScore(qArtist string, candidateArtists []FeiNiuArtist) float64 {
	q := strings.ToLower(strings.TrimSpace(qArtist))
	if q == "" {
		return 0.7 // 无外部歌手时给予中性分
	}

	pArtist := strings.ToLower(ExtractPrimaryArtist(q))

	var candidateNames []string
	for _, a := range candidateArtists {
		candidateNames = append(candidateNames, strings.ToLower(strings.TrimSpace(a.Name)))
	}
	combinedCandidate := strings.Join(candidateNames, " ")

	if strings.Contains(combinedCandidate, q) || strings.Contains(q, combinedCandidate) {
		return 1.0
	}

	if pArtist != "" && (strings.Contains(combinedCandidate, pArtist) || strings.Contains(pArtist, combinedCandidate)) {
		return 0.92
	}

	// 针对每个候选歌手做最佳子匹配
	bestScore := 0.0
	for _, cn := range candidateNames {
		if cn == "" {
			continue
		}
		dist := LevenshteinDistance(pArtist, cn)
		maxLen := max(len([]rune(pArtist)), len([]rune(cn)))
		if maxLen > 0 {
			score := 1.0 - float64(dist)/float64(maxLen)
			if score > bestScore {
				bestScore = score
			}
		}
	}
	return bestScore
}

// ScoreCandidate 综合计算候选歌曲得分
func ScoreCandidate(song playlist.SongItem, track FeiNiuTrack) float64 {
	titleScore := calcTitleScore(song.SongName, track.Title)
	artistScore := calcArtistScore(song.Artist, track.Artists)

	// 音质微量偏好（最多加 0.05）
	qualityBonus := 0.0
	format := strings.ToLower(track.AudioSpec.Format)
	if format == "flac" || format == "ape" || format == "wav" || format == "alac" || format == "dsf" || format == "dff" {
		qualityBonus = 0.05
	} else if track.AudioSpec.Bitrate >= 320000 || track.AudioSpec.Bitrate == 320 {
		qualityBonus = 0.03
	}

	total := titleScore*0.55 + artistScore*0.40 + qualityBonus
	if total > 1.0 {
		total = 1.0
	}
	return total
}

// MatchSong 在飞牛曲库中搜索并匹配单首歌曲
func (imp *Importer) MatchSong(ctx context.Context, song playlist.SongItem) (*ImportMatchItem, error) {
	primaryArtist := ExtractPrimaryArtist(song.Artist)
	cleanTitle := CleanSongTitle(song.SongName)

	var candidates []FeiNiuTrack
	seenGUID := make(map[string]bool)

	addCandidates := func(tracks []FeiNiuTrack) {
		for _, t := range tracks {
			if !seenGUID[t.GUID] && t.GUID != "" {
				seenGUID[t.GUID] = true
				candidates = append(candidates, t)
			}
		}
	}

	findBest := func() (*FeiNiuTrack, float64) {
		var best *FeiNiuTrack
		bestScore := 0.0
		for _, cand := range candidates {
			score := ScoreCandidate(song, cand)
			if score > bestScore {
				bestScore = score
				best = &cand
			}
		}
		return best, bestScore
	}

	// 搜索策略 1: 纯歌名（最精准，飞牛曲库搜索直接按歌名索引）
	if cleanTitle != "" {
		res, err := imp.client.SearchTracks(ctx, cleanTitle, 1, 20)
		if err == nil && res != nil {
			addCandidates(res.List)
			// 若已获得高分命中（>= 0.85），直接提前收敛，不再浪费额外 HTTP 请求
			if best, score := findBest(); score >= 0.85 && best != nil {
				return buildMatchResult(song, best, score), nil
			}
		}
	}

	// 搜索策略 2: 若原始歌名与清洗后不同，搜索原始歌名
	if len(candidates) < 5 && song.SongName != "" && song.SongName != cleanTitle {
		res, err := imp.client.SearchTracks(ctx, song.SongName, 1, 20)
		if err == nil && res != nil {
			addCandidates(res.List)
			if best, score := findBest(); score >= 0.85 && best != nil {
				return buildMatchResult(song, best, score), nil
			}
		}
	}

	// 搜索策略 3: 歌名 + 主歌手联合搜索
	if len(candidates) == 0 && primaryArtist != "" {
		q := strings.TrimSpace(cleanTitle + " " + primaryArtist)
		res, err := imp.client.SearchTracks(ctx, q, 1, 20)
		if err == nil && res != nil {
			addCandidates(res.List)
		}
	}

	// 搜索策略 4: 主歌手搜索（从该歌手名下寻找匹配歌曲）
	if len(candidates) == 0 && primaryArtist != "" {
		res, err := imp.client.SearchTracks(ctx, primaryArtist, 1, 30)
		if err == nil && res != nil {
			addCandidates(res.List)
		}
	}

	if len(candidates) == 0 {
		return &ImportMatchItem{
			Index:    song.Index,
			SongName: song.SongName,
			Artist:   song.Artist,
			Matched:  false,
			Score:    0,
			Error:    "本地曲库未找到匹配歌曲",
		}, nil
	}

	bestTrack, bestScore := findBest()

	// 判定阈值 (>= 0.65 认定为匹配成功)
	if bestScore >= 0.65 && bestTrack != nil {
		return buildMatchResult(song, bestTrack, bestScore), nil
	}

	return &ImportMatchItem{
		Index:    song.Index,
		SongName: song.SongName,
		Artist:   song.Artist,
		Matched:  false,
		Score:    bestScore,
		Error:    "曲库候选歌曲相似度低于匹配阈值",
	}, nil
}

func buildMatchResult(song playlist.SongItem, bestTrack *FeiNiuTrack, bestScore float64) *ImportMatchItem {
	var artistNames []string
	for _, a := range bestTrack.Artists {
		if a.Name != "" {
			artistNames = append(artistNames, a.Name)
		}
	}
	return &ImportMatchItem{
		Index:         song.Index,
		SongName:      song.SongName,
		Artist:        song.Artist,
		Matched:       true,
		TrackGUID:     bestTrack.GUID,
		MatchedTitle:  bestTrack.Title,
		MatchedArtist: strings.Join(artistNames, " / "),
		Score:         bestScore,
	}
}

// ImportPlaylist 导入整个歌单到飞牛 NAS (并发多协程加速匹配)
func (imp *Importer) ImportPlaylist(ctx context.Context, req ImportPlaylistRequest) (*ImportPlaylistResult, error) {
	if len(req.Songs) == 0 {
		return nil, fmt.Errorf("待导入歌曲列表为空")
	}

	targetGUID := req.PlaylistGUID
	targetName := strings.TrimSpace(req.Name)

	// 若未指定现有歌单，则创建新歌单
	if targetGUID == "" {
		if targetName == "" {
			targetName = fmt.Sprintf("导入歌单 %s", time.Now().Format("2006-01-02 15:04"))
		}
		newPl, err := imp.client.CreatePlaylist(ctx, targetName, "")
		if err != nil {
			return nil, fmt.Errorf("创建飞牛歌单失败: %w", err)
		}
		targetGUID = newPl.GUID
		targetName = newPl.Name
	}

	totalSongs := len(req.Songs)
	results := make([]ImportMatchItem, totalSongs)

	// 使用并发工作池加速歌曲匹配检索 (6 并发)
	concurrency := 6
	if totalSongs < concurrency {
		concurrency = totalSongs
	}

	type matchTask struct {
		idx  int
		song playlist.SongItem
	}

	taskChan := make(chan matchTask, totalSongs)
	for i, s := range req.Songs {
		taskChan <- matchTask{idx: i, song: s}
	}
	close(taskChan)

	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range taskChan {
				select {
				case <-ctx.Done():
					results[t.idx] = ImportMatchItem{
						Index:    t.song.Index,
						SongName: t.song.SongName,
						Artist:   t.song.Artist,
						Matched:  false,
						Error:    "操作被取消",
					}
					continue
				default:
				}

				matchItem, err := imp.MatchSong(ctx, t.song)
				if err != nil {
					results[t.idx] = ImportMatchItem{
						Index:    t.song.Index,
						SongName: t.song.SongName,
						Artist:   t.song.Artist,
						Matched:  false,
						Error:    err.Error(),
					}
				} else if matchItem != nil {
					results[t.idx] = *matchItem
				}
			}
		}()
	}
	wg.Wait()

	result := &ImportPlaylistResult{
		PlaylistGUID: targetGUID,
		PlaylistName: targetName,
		Total:        totalSongs,
		Results:      results,
	}

	var matchedGUIDs []string
	var unmatchedSongs []playlist.SongItem

	for i, item := range results {
		if item.Matched && item.TrackGUID != "" {
			result.MatchedCount++
			matchedGUIDs = append(matchedGUIDs, item.TrackGUID)
		} else {
			result.UnmatchedCount++
			unmatchedSongs = append(unmatchedSongs, req.Songs[i])
		}
	}

	result.UnmatchedSongs = unmatchedSongs

	// 批量将匹配歌曲写入歌单
	if len(matchedGUIDs) > 0 {
		if err := imp.client.AddTracksToPlaylist(ctx, targetGUID, matchedGUIDs); err != nil {
			return result, fmt.Errorf("匹配完成但写入飞牛歌单失败: %w", err)
		}
	}

	return result, nil
}
