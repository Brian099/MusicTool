# ==========================================
# Stage 1: 跨平台纯静态 Go 二进制构建
# ==========================================
FROM golang:alpine AS builder

WORKDIR /build

# 安装基础构建工具
RUN apk add --no-cache git ca-certificates

# 预下载依赖以利用 Docker 缓存加速
COPY go.mod go.sum ./
RUN go mod download

# 复制源码和前端内嵌文件
COPY . .

# 纯静态编译，移除符号表与调试信息以极致压缩体积
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /build/music-toolkit .

# ==========================================
# Stage 2: 极轻量运行时镜像 (基于 Alpine)
# ==========================================
FROM alpine:3.19

WORKDIR /app

# 安装 ffmpeg、chromaprint (包含官方 fpcalc 模块) 与时区支持
RUN apk add --no-cache \
    ffmpeg \
    chromaprint \
    tzdata \
    ca-certificates && \
    rm -rf /var/cache/apk/*

# 从构建阶段复制生成的单一静态二进制
COPY --from=builder /build/music-toolkit /app/music-toolkit
RUN chmod +x /app/music-toolkit

# 设置默认环境变量
ENV MUSIC_DIR=/music \
    OUTPUT_DIR=/output \
    DB_PATH=/data/music_toolkit.db \
    MAX_WORKERS=4 \
    PORT=6826

# 持久化卷声明
VOLUME ["/music", "/output", "/data"]

# 暴露服务端口
EXPOSE 6826

# 启动服务
ENTRYPOINT ["/app/music-toolkit"]
