package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed frontend/*
var frontendFS embed.FS

// GetFrontendFileSystem 获取内嵌前端静态文件系统的子路径
func GetFrontendFileSystem() http.FileSystem {
	sub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}
