package playlist

import "strings"

// normalizeCoverURL 规范化歌单封面地址：
//   - 去掉首尾空白
//   - 补全协议相对地址（//host/path -> https://host/path）
//   - 剥离已有的尺寸参数（网易云 ?param=300y300 之类），
//     由调用方按需重新拼接，避免出现重复参数
func normalizeCoverURL(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	}
	if idx := strings.Index(u, "?param="); idx >= 0 {
		u = u[:idx]
	}
	return u
}
