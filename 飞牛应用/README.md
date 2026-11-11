# 飞牛 NAS 原生应用 (FnOS App) 打包说明

本文档记录如何维护与打包 `music-toolkit.fpk` 飞牛官方格式应用包。

## 目录结构

- `manifest`: 飞牛应用清单文件（版本号、架构、名称、桌面入口等）
- `fnpack.exe`: 飞牛官方 Windows 端打包工具命令行程序
- `app/`:
  - `music-toolkit-linux-amd64`: Linux 平台运行的主程序二进制（已内嵌 Web 前端）
  - `bin/`: 内置静态依赖二进制（`ffmpeg`、`ffprobe`、`fpcalc`）
  - `ui/`: 应用桌面快捷方式及配置
- `cmd/`:
  - `main`: 飞牛后台服务启动与停止控制脚本（已配置挂载目录识别与环境变量导出）
  - `install_init`, `install_callback`, `uninstall_init`, `upgrade_callback` 等生命周期钩子
- `config/`: 权限与应用配置
- `wizard/`: 安装向导配置
- `ICON.PNG`, `ICON_256.PNG`: 应用图标

---

## 本地打包流程 (PowerShell)

在项目根目录下依次执行：

```powershell
# 1. 编译最新的 Linux AMD64 二进制 (无 CGo 纯静态)
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"; go build -ldflags="-s -w" -o music-toolkit-linux-amd64 .

# 2. 将编译出的二进制覆盖复制到应用 app 目录
Copy-Item -Path music-toolkit-linux-amd64 -Destination .\飞牛应用\app\music-toolkit-linux-amd64 -Force

# 3. 进入飞牛应用目录调用打包工具
cd 飞牛应用
.\fnpack.exe build
cd ..
```

执行后，将在当前目录生成最新的 `music-toolkit.fpk`，可直接在飞牛 NAS 的【应用中心】->【手动安装】中上传使用。

> 自 2.1.2 起，安装包不再纳入 Git 版本库，改为发布到 GitHub Release 附件：
> `gh release create v<版本> 飞牛应用/music-toolkit.fpk`

---

## 部署后的音乐目录授权（重要）

`.fpk` 安装完成后默认**没有任何音乐目录权限**，此时界面顶栏会显示「未挂载」，扫描也无法进行。授权步骤如下：

1. 打开飞牛 **应用中心 → 已安装 → 音乐工具箱**
2. 点击应用卡片的 **设置**（或「⋯」→ 权限）
3. 在 **文件夹访问权限 / 可访问目录** 中勾选音乐所在的文件夹（如 `音乐`、`Music`）
4. **停止 → 启动** 应用（目录变量只在进程启动时读取，不重启不生效）

### 背后的机制

飞牛会把授权结果通过环境变量注入应用，`cmd/main` 再导出为 `MUSIC_DIR`：

| 环境变量 | 说明 |
| :--- | :--- |
| `TRIM_DATA_ACCESSIBLE_PATHS` | 已授权的可访问目录（优先采用） |
| `TRIM_DATA_SHARE_PATHS` | 共享文件夹目录（兜底） |
| `MUSIC_DIR` | 最终传给主程序的音乐库路径，两者都没有时回退 `/music` |

### 注意事项

- **多目录**：可以一次授权多个文件夹，应用支持 `,` `;` `:` 分隔的多路径，界面会出现目录下拉框供选择扫描范围。
- **读写权限**：仅做检测（格式检查 / 去重比对 / 真假无损鉴别）给**只读**即可；若要**原地改后缀**、**删除重复**、**移入回收站**，必须给**可写**，否则会报权限错误。
- **数据库与输出目录**：`DB_PATH`、`OUTPUT_DIR` 都位于应用数据目录（`TRIM_PKGVAR`）下，与音乐库分离，不需要额外授权。
- **临时指定路径**：也可以在 Web 界面各面板的「音乐目录」输入框直接填绝对路径（如 `/vol1/1000/Music`），仅对该次扫描生效，不改变全局挂载。
- **确认是否生效**：顶栏「音乐库」状态标签变绿并显示目录数量即为挂载成功，悬停可查看每个目录的文件数与占用；仍然失败时查看应用数据目录下的 `info.log` 启动日志。
