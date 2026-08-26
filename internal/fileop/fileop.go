package fileop

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileOperator 文件操作器
type FileOperator struct {
	MusicDir  string
	OutputDir string
}

func NewFileOperator(musicDir, outputDir string) *FileOperator {
	return &FileOperator{
		MusicDir:  musicDir,
		OutputDir: outputDir,
	}
}

// RenameInPlace 在原路径原地纠正后缀
func (f *FileOperator) RenameInPlace(filePath, newExt string) (bool, string, string) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false, "", "原文件不存在"
	}

	cleanExt := "." + strings.TrimPrefix(newExt, ".")
	ext := filepath.Ext(filePath)
	base := strings.TrimSuffix(filePath, ext)
	newPath := base + cleanExt

	if newPath == filePath {
		return true, filePath, "后缀已一致，无需重命名"
	}

	if _, err := os.Stat(newPath); err == nil {
		return false, "", fmt.Sprintf("目标文件已存在: %s", filepath.Base(newPath))
	}

	if err := os.Rename(filePath, newPath); err != nil {
		return false, "", fmt.Sprintf("重命名失败: %v", err)
	}
	return true, newPath, "后缀纠正成功"
}

// ProcessMismatched 复制/移动或原地修正异常文件
func (f *FileOperator) ProcessMismatched(filePath, outputDir, action string, keepStructure, fixExt bool, suggestedExt, musicRoot string) (bool, string, string) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false, "", "源文件不存在"
	}

	if action == "rename_fix" {
		return f.RenameInPlace(filePath, suggestedExt)
	}

	if action != "copy" && action != "move" {
		return true, filePath, "仅扫描模式，未改动文件"
	}

	if outputDir == "" {
		outputDir = f.OutputDir
	}
	if musicRoot == "" {
		musicRoot = f.MusicDir
	}

	absFile, _ := filepath.Abs(filePath)
	absRoot, _ := filepath.Abs(musicRoot)
	absOut, _ := filepath.Abs(outputDir)

	var targetPath string
	if keepStructure {
		rel, err := filepath.Rel(absRoot, absFile)
		if err != nil {
			rel = filepath.Base(absFile)
		}
		targetPath = filepath.Join(absOut, rel)
	} else {
		targetPath = filepath.Join(absOut, filepath.Base(absFile))
	}

	if fixExt && suggestedExt != "" {
		cleanExt := "." + strings.TrimPrefix(suggestedExt, ".")
		ext := filepath.Ext(targetPath)
		targetPath = strings.TrimSuffix(targetPath, ext) + cleanExt
	}

	os.MkdirAll(filepath.Dir(targetPath), 0755)

	// 避免重名冲突
	if _, err := os.Stat(targetPath); err == nil && targetPath != absFile {
		ext := filepath.Ext(targetPath)
		base := strings.TrimSuffix(targetPath, ext)
		counter := 1
		for {
			candidate := fmt.Sprintf("%s_%d%s", base, counter, ext)
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				targetPath = candidate
				break
			}
			counter++
		}
	}

	if action == "copy" {
		if err := copyFile(absFile, targetPath); err != nil {
			return false, "", fmt.Sprintf("复制失败: %v", err)
		}
		return true, targetPath, "已复制到输出目录"
	} else if action == "move" {
		if err := os.Rename(absFile, targetPath); err != nil {
			// 跨盘移动 fallback
			if errCopy := copyFile(absFile, targetPath); errCopy == nil {
				os.Remove(absFile)
				return true, targetPath, "已移动到输出目录"
			}
			return false, "", fmt.Sprintf("移动失败: %v", err)
		}
		return true, targetPath, "已移动到输出目录"
	}

	return false, "", "未知操作"
}

// MoveToRecycleBin 安全移入回收站
func (f *FileOperator) MoveToRecycleBin(filePath, recycleDir string) (bool, string, string) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false, "", "文件不存在"
	}

	if recycleDir == "" {
		recycleDir = filepath.Join(f.OutputDir, "_recycle_bin")
	}

	absFile, _ := filepath.Abs(filePath)
	absMusic, _ := filepath.Abs(f.MusicDir)

	rel, err := filepath.Rel(absMusic, absFile)
	if err != nil {
		rel = filepath.Base(absFile)
	}

	targetPath := filepath.Join(recycleDir, rel)
	os.MkdirAll(filepath.Dir(targetPath), 0755)

	if _, err := os.Stat(targetPath); err == nil {
		ext := filepath.Ext(targetPath)
		base := strings.TrimSuffix(targetPath, ext)
		counter := 1
		for {
			candidate := fmt.Sprintf("%s_%d%s", base, counter, ext)
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				targetPath = candidate
				break
			}
			counter++
		}
	}

	if err := os.Rename(absFile, targetPath); err != nil {
		if errCopy := copyFile(absFile, targetPath); errCopy == nil {
			os.Remove(absFile)
			return true, targetPath, "已移入回收站"
		}
		return false, "", fmt.Sprintf("移入回收站失败: %v", err)
	}
	return true, targetPath, "已移入回收站"
}

// DeletePermanently 彻底删除
func (f *FileOperator) DeletePermanently(filePath string) (bool, string, string) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return true, filePath, "文件已被移除"
	}
	if err := os.Remove(filePath); err != nil {
		return false, "", fmt.Sprintf("删除失败: %v", err)
	}
	return true, filePath, "文件已彻底删除"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
