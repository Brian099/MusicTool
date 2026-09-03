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

### 4. 📋 歌单文本提取与跨平台迁移 (Playlist Extractor)
- 🌐 **网易云音乐 / QQ 音乐 / 汽水音乐全支持**：支持直接粘贴各平台 Web/App 分享短链或整段分享文案，自动识别并抓取全量歌曲。
- ⚙️ **多种输出格式与排序控制**：支持 `歌名 - 歌手` (默认)、`歌手 - 歌名`、`仅歌名`，支持保留/清洗括号内 Live/Remix 标签，支持倒序排列。
- ⚡ **纯 Go 内嵌 JS 签名引擎**：无需安装 Node.js 或外部服务，本地极速计算 QQ 音乐签名并支持万首大歌单自动分页拉取。
- 💾 **本地 SQLite 历史持久化**：无需配置 MySQL/Redis，自动记录提取历史，支持一键重新展开查看与管理。
- 📋 **一键复制与多格式导出**：一键复制纯文本，无缝对接 TuneMyMusic / Spotlistr 迁移至 Apple Music / Spotify / YouTube Music，支持导出 TXT / CSV。

### 5. 🐂 飞牛 NAS 音乐歌单管理与智能导入 (FeiNiu Music Sync & Manager)
- 🔐 **密码加密与 401 自动无感保活**：支持直连飞牛 FnOS 原生音乐服务（支持局域网 IP / 域名 / 端口 / 安全码），采用本地 SHA-256 哈希传输凭据，遭遇 Token 过期自动加锁无感重新登录换取新会话。
- 📑 **全功能歌单管理卡片面板**：Web 端直接浏览飞牛 NAS 歌单、封面展示、一键新建、重命名、删除歌单、查看歌单曲目、从歌单移除歌曲与一键清理失效歌曲。
- 🚀 **外部歌单三级阶梯式智能匹配导入**：提取网易云/QQ/汽水音乐歌单后，可一键导入飞牛歌单！自动对歌名/歌手进行噪音清洗（去除 Live/Remix/重制版/副歌手干扰），在飞牛 NAS 本地曲库中精准检索匹配对应曲目并批量写入，最后输出匹配率及未收录歌曲补全清单。

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

---

## 🔐 双重认证与系统解锁机制

系统采用灵活的双重认证模式，全面兼容非飞牛环境（普通 PC / Linux 服务器 / Docker 部署）与飞牛 NAS 原生环境：
1. **系统本地账号验证**：
   - **首次使用**：若系统尚未创建管理员账号，前端自动引导设置管理员用户名和密码，创建成功即自动登录并解锁系统；
   - **后续使用**：使用已设定的管理员用户名与密码登录。
2. **飞牛 NAS 音乐直连验证**：
   - 输入飞牛 NAS 地址与用户凭据直连登录，自动保活并全量解锁。
3. **全局解锁规则**：只要满足 **【本地账号已登录】** 或 **【飞牛 NAS 已连接】** 任意一种，即可全量解锁并使用系统全部音频工具（格式检查、音频去重、真假无损、歌单提取等）。

---

## 🛠️ 本地编译与打包指南

### 1. 编译 Windows 本地运行版 (`.exe`)
在项目根目录下执行：
```powershell
go build -ldflags="-s -w" -o music-toolkit.exe .
```
> 生成产物：`music-toolkit.exe`，双击或在终端执行即可启动服务。

### 2. 交叉编译 Linux AMD64（Linux 服务器 / 飞牛 NAS x86_64）
```powershell
# PowerShell:
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"; go build -ldflags="-s -w" -o music-toolkit-linux-amd64 .

# CMD (命令提示符):
set CGO_ENABLED=0&& set GOOS=linux&& set GOARCH=amd64&& go build -ldflags="-s -w" -o music-toolkit-linux-amd64 .

# Linux / macOS Bash:
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o music-toolkit-linux-amd64 .
```

### 3. 交叉编译 Linux ARM64（ARM 架构设备）
```powershell
# PowerShell:
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="arm64"; go build -ldflags="-s -w" -o music-toolkit-linux-arm64 .

# CMD (命令提示符):
set CGO_ENABLED=0&& set GOOS=linux&& set GOARCH=arm64&& go build -ldflags="-s -w" -o music-toolkit-linux-arm64 .

# Linux / macOS Bash:
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o music-toolkit-linux-arm64 .
```

### 4. 飞牛应用原生安装包 (`.fpk`) 一键更新与打包
飞牛 NAS 官方原生应用包采用 `.fpk` 格式，本地更新并打包步骤如下：

```powershell
# 步骤 1：编译最新的 Linux AMD64 二进制
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"; go build -ldflags="-s -w" -o music-toolkit-linux-amd64 .

# 步骤 2：将新编译的二进制复制到飞牛应用目录中
Copy-Item -Path music-toolkit-linux-amd64 -Destination .\飞牛应用\app\music-toolkit-linux-amd64 -Force

# 步骤 3：进入飞牛应用目录，使用 fnpack 工具构建安装包
cd 飞牛应用
.\fnpack.exe build
cd ..
```
> 执行完毕后，将在 `飞牛应用/` 目录下生成最新的 `music-toolkit.fpk` 文件，可直接上传至飞牛 NAS 应用中心进行离线安装或升级。

