package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"music-toolkit/internal/config"
	"music-toolkit/internal/database"
	"music-toolkit/internal/server"
)

func main() {
	cfg := config.LoadConfig()

	fmt.Println("==================================================")
	fmt.Println("🎵 Music Toolkit (Go Version: Format Checker & Songloft Dedup)")
	fmt.Printf("🚀 服务启动中，Web 前台访问地址: http://0.0.0.0:%d\n", cfg.Port)
	fmt.Printf("📁 音乐库目录: %s\n", cfg.MusicDir)
	fmt.Printf("📂 输出目录: %s\n", cfg.OutputDir)
	fmt.Printf("💾 数据库路径: %s\n", cfg.DBPath)
	fmt.Println("==================================================")

	db, err := database.OpenDB(cfg.DBPath)
	if err != nil {
		slog.Error("打开数据库失败", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	frontendFS := GetFrontendFileSystem()
	srv := server.NewServer(cfg, db, frontendFS)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      srv.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP 服务异常退出", "err", err)
			os.Exit(1)
		}
	}()

	// 捕获系统退出信号实现平滑退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("正在关闭服务...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("服务关闭出错", "err", err)
	}
	slog.Info("服务已安全退出")
}
