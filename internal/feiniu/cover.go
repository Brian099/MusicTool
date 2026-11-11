package feiniu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// maxCoverBytes 封面图片下载上限，超过即中止，避免把超大图塞进内存
	maxCoverBytes = 8 << 20 // 8MB
	// coverDownloadTimeout 封面下载超时
	coverDownloadTimeout = 20 * time.Second
	// coverDownloadUA 下载封面使用的 UA，部分图床对空 UA 会返回 403
	coverDownloadUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// coverImageKind 通过文件魔数判定图片真实格式，返回用于文件名的扩展名。
// 不能信任 Content-Type：网易云封面实际是 PNG，但响应头声称 image/jpg。
func coverImageKind(data []byte) (string, bool) {
	switch {
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "jpg", true
	case len(data) >= 8 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G':
		return "png", true
	case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "webp", true
	case len(data) >= 6 && string(data[0:6]) == "GIF89a":
		return "gif", true
	case len(data) >= 6 && string(data[0:6]) == "GIF87a":
		return "gif", true
	}
	return "", false
}

// PrepareCoverImage 校验封面图片内容并返回用于文件名的扩展名。
// 供下载外部封面与前端本地上传两条链路共用：大小限制 + 魔数校验，
// 避免把防盗链错误页或非图片文件传给飞牛 NAS。
func PrepareCoverImage(data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("封面图片内容为空")
	}
	if len(data) > maxCoverBytes {
		return "", fmt.Errorf("封面图片超过 %d MB 限制", maxCoverBytes>>20)
	}
	ext, ok := coverImageKind(data)
	if !ok {
		head := string(data[:min(len(data), 40)])
		return "", fmt.Errorf("封面内容不是有效图片（可能是防盗链错误页）: %s", strings.TrimSpace(head))
	}
	return ext, nil
}

// downloadCoverImage 下载外部歌单封面并校验为真实图片。
// 防盗链、下载失败或返回 HTML 错误页时返回错误，调用方据此跳过封面上传而不影响歌单导入。
func downloadCoverImage(ctx context.Context, rawURL string) ([]byte, string, error) {
	u := strings.TrimSpace(rawURL)
	if u == "" {
		return nil, "", errors.New("封面地址为空")
	}
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	}

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, "", fmt.Errorf("创建封面下载请求失败: %w", err)
	}
	req.Header.Set("User-Agent", coverDownloadUA)
	// QQ 音乐图床对部分场景校验来源，带上 Referer 更稳妥；对其他图床无副作用
	if strings.Contains(u, "gtimg.cn") || strings.Contains(u, "qq.com") {
		req.Header.Set("Referer", "https://y.qq.com/")
	}

	client := &http.Client{Timeout: coverDownloadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("下载封面失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("下载封面失败: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCoverBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("读取封面数据失败: %w", err)
	}
	if len(data) == 0 {
		return nil, "", errors.New("封面数据为空")
	}

	ext, err := PrepareCoverImage(data)
	if err != nil {
		return nil, "", err
	}
	return data, ext, nil
}
