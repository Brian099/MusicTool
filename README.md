# 🎵 Music Toolkit (音乐助手工具箱 - Go 高性能轻量版)

[![Build & Push Docker Image](https://github.com/Brian099/MusicTool/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/Brian099/MusicTool/actions/workflows/docker-publish.yml)
[![Docker Pulls](https://img.shields.io/docker/pulls/brian9909/musictool.svg)](https://hub.docker.com/r/brian9909/musictool)
[![Docker Image Size](https://img.shields.io/docker/image-size/brian9909/musictool/latest)](https://hub.docker.com/r/brian9909/musictool)

> 基于 **Go 1.22 + Alpine 多阶段构建** 打造的极致轻量级音乐管理容器，整合 **Songloft 核心音频指纹去重算法 (Chromaprint AcoustID)**、**高精度真实格式/伪装检测修复引擎** 与 **FLAC/APE 真实无损频谱鉴别引擎**，内置高颜值现代化 Web 前台 UI（支持明暗主题无缝切换）。

---

## ⚡ 为什么选择 Go 语言版本？

- 🚀 **极小 Docker 镜像体积**：从 Python 版的 ~300MB 压缩至 **约 60MB ~ 80MB**（体积缩减约 75%）！
- 💾 **超低内存开销**：常驻内存仅 **10MB ~ 25MB**，对 NAS（群晖、威联通、绿联、极空间等）和软路由极度友好。
- 📦 **单一静态二进制**：通过 Go 1.16+ `//go:embed` 将现代化 Web 前端完全内嵌进二进制，无需任何 Python 解释器或外部前端依赖。
- 💪 **高并发 Goroutine 调度**：原生协程调度，大曲库批量提取指纹与频谱扫描速度极快。

---

## ✨ 核心功能

### 1. 🎵 真实格式检测与后缀纠正 (Format Checker & Fixer)
- 🔍 **多重深度探测**：结合二进制魔数（Magic Bytes）与 FFprobe 音频流探测，秒级识别文件真实类型。
- 🚫 **伪装与防盗链识别**：精准捕获被误命名为 `.mp3` 的 HTML 403/404 报错网页、JSON 错误响应或损坏文件。
- ✏️ **灵活修复模式**：
  - **原地修正后缀**：直接将 `.mp3` 纠正为真实格式（如 `.m4a` / `.flac`）。
  - **归档模式**：支持将异常文件复制或移动到输出目录，并可保留原有子文件夹层级结构。
- 📑 **一键导出报告**：支持导出详尽的 `mismatched_report.csv` 报告。

### 2. 🔍 声学指纹深度去重 (Songloft Dedup Engine)
- 🧬 **AcoustID 声学指纹**：移植自 `Songloft` 核心算法，采样音频前 120 秒波形生成独立于标签与文件名的声学指纹。
- 🛡️ **30s 时长守卫保守聚类**：在同指纹歌曲内按音频全片时长进行容差聚类（默认 30s 阈值），精准识别跨格式、跨音质的相同曲目，同时避免误伤长音频或不同剪辑版本。
- ⚡ **SQLite 纯 Go 持久化缓存**：支持增量计算、断点续算，多次扫描无需重复消耗 CPU 计算指纹。
- ⭐ **智能最佳音质推荐**：自动综合无损格式（FLAC/WAV/APE/ALAC 等）、比特率、采样率进行打分，自动高亮标记最佳版本。
- ♻️ **安全清理模式**：支持一键将冗余副本移入回收站文件夹（保留相对路径），或彻底永久删除。
- 🎧 **在线试听播放**：支持在网页端直接点击试听对比两首重复音频的音质差异。

### 3. 💎 FLAC/APE 真假无损鉴别 (Lossless Checker)
- 🔬 **高频截止与泛音能量分析**：
  - **🏆 真 Hi-Res 高解析**：采样率 $\ge 88.2\text{kHz}$，高频平滑延伸超越 $24\text{kHz} \sim 48\text{kHz}$。
  - **💎 真 CD 无损**：$20\text{kHz} \sim 22.05\text{kHz}$ 泛音能量丰富连贯无硬断层。
  - **⚠️ 假无损 (320k)**：探测 $20.0\text{kHz}$ 典型刀切截断线。
  - **🚫 劣质假无损 (128k)**：探测 $16.0\text{kHz}$ 以下硬断崖截断。
- 📊 **置信度评分与报告导出**：实时计算每首无损的截止频率与置信度，支持按假无损快速过滤与一键移入回收站。

---

## 🚀 快速使用 (Docker 镜像直接运行)

无需拉取源码，直接拉取 Docker Hub 预编译镜像即可启动：

### 1. 使用 `docker-compose.yml` (推荐)

```yaml
version: '3.8'

services:
  music-toolkit:
    image: brian9909/musictool:latest
    container_name: music-toolkit
    restart: unless-stopped
    ports:
      - "6826:6826"
    environment:
      - MUSIC_DIR=/music
      - OUTPUT_DIR=/output
      - DB_PATH=/data/music_toolkit.db
      - MAX_WORKERS=4
      - PORT=6826
      - TZ=Asia/Shanghai
    volumes:
      - /path/to/your/music:/music     # 挂载您的真实音乐库目录
      - ./output:/output              # 格式修复输出目录
      - ./data:/data                  # 数据库持久化目录
```

启动命令：
```bash
docker compose up -d
```

### 2. 使用 `docker run` 单行命令

```bash
docker run -d \
  --name music-toolkit \
  --restart unless-stopped \
  -p 6826:6826 \
  -v /path/to/your/music:/music \
  -v ./output:/output \
  -v ./data:/data \
  -e TZ=Asia/Shanghai \
  brian9909/musictool:latest
```

---

## 🌐 访问 Web 前台

启动成功后，浏览器打开：
```text
http://<服务器IP>:6826
```
默认采用现代清新亮色主题，右上角支持 **☀️ 亮色 / 🌙 暗色** 一键切换。
