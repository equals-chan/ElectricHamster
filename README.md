## [ElectricHamster / 电子仓鼠](https://github.com/equals-chan/ElectricHamster)

电子仓鼠症：指那些不断在网络上搜集自己感兴趣的数据、将其保存下来的人，尽管可能在删除前也看不了几眼，却依然乐此不疲。

大量数据的保存与整理极其耗费时间。本项目用于**自动对数据加密压缩、整理**，并记录密码台账：把顶层文件夹批量打包成加密 `.7z`，随机生成密码存入加密金库，支持判重跳过、进度显示、Excel 导出与旧数据迁移。

用 **Go + Wails v3 + Vue 3** 重写，内嵌 7-Zip ZS 引擎（zstd / LZMA2 + AES-256 + 文件名加密）。

### 功能

- 图形界面配置任务：源目录、输出目录、压缩算法/等级、加密、分卷、包含/排除
- 批量加密归档，密码自动生成并写入**加密金库**（Argon2id + AES-256-GCM）
- 判重跳过：内容签名 / 仅文件夹名 / 不判重（适合存档会变的游戏目录）
- 任务级独立编号 + 文件名前后缀 + 起始编号
- 运行进度（整体 + 每文件夹）、暂停 / 继续 / 取消
- 归档记录：校验、解压、导出 Excel（含密码）
- 导入旧版（Java）的 `config.properties` + `db.sqlite3`

### 开发

```
# 后端测试（Linux/macOS，不含 GUI 根包）
go test ./internal/... ./cmd/...

# 前端
cd frontend && npm install && npm run build

# Windows 打包：exe + 便携 zip + NSIS 安装包
scripts/package-windows.sh            # 全部
scripts/package-windows.sh exe        # 仅 exe + 便携 zip
```

详见 [DESIGN.md](DESIGN.md)。

> 使用的其他项目：[7-Zip ZS](https://github.com/mcmilk/7-Zip-zstd)（压缩引擎）、[Wails](https://wails.io/)、[zip4j](https://github.com/srikanth-lingala/zip4j)（旧版所用）。

## 许可证

本项目以 [MIT License](LICENSE) 发布。第三方组件与内嵌 7-Zip ZS 引擎的许可说明见 [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md)。
