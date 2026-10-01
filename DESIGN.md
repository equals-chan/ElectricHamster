# ElectricHamster 重写设计文档

> 状态：草案 v1
> 目标读者：本项目维护者
> 一句话：把「电子仓鼠」的加密归档 + 密码台账能力，用 Wails + 7-Zip(zstd) 重做成跨平台图形工具（Windows 优先）。

---

## 1. 背景与目标

### 1.1 原项目（v0.2）做了什么

Java + zip4j 的 CLI 工具：

1. 读取 `config.properties`（源目录、压缩输出目录、Excel 路径、SQLite 路径、是否随机密码等）。
2. 扫描源目录的**顶层子文件夹**（跳过文件）。
3. 用 SQLite `t_pwd` 表按**文件夹名**判断是否已压缩过。
4. 未压缩的：从 10000 起分配序号 → 生成随机 10 位密码或使用固定密码 → zip4j AES-256 压缩。
5. 把 `(id, 文件夹名, 密码, 时间)` 写入 SQLite，可选导出到 `.xls`。
6. 控制台进度条 + 彩色输出 + 计时/计数。

### 1.2 保留的能力

- 批量加密归档
- 密码自动生成 + 台账记录
- 已归档检测（幂等，防重复压缩）
- 进度可视 + 结果导出

### 1.3 重写要解决的痛点

| 原问题 | 新方案 |
|---|---|
| 控制台黑框，配置手写、易错（`\\` 转义） | Wails GUI，原生目录选择器，配置由界面管理 |
| 密码明文存 SQLite / Excel | Argon2id + AES-256-GCM 加密金库，运行时解锁 |
| 压缩算法老旧（Deflate/AES-ZIP） | 7-Zip 容器 + zstd（可选 LZMA2），固实压缩，`-mhe=on` 文件名加密 |
| 只按文件夹名判重，改名重复压 | 基于内容签名 `content_sig` 判重 |
| 只压一层，无并发，无断点 | 任务引擎 + worker 池 + 队列持久化（可续跑） |
| 跨平台差（硬编码 `\`，Win 专属） | Go `filepath`，一套代码多平台（本期 Windows 优先） |
| Excel 用 jxl `.xls` | `excelize` 生成 `.xlsx` |

### 1.4 非目标（本期不做）

- 云同步 / 网盘对接
- 增量差异压缩（delta）
- 文件内容级加密（只做归档整体加密）
- 自动监听目录、cron 调度、旧数据迁移（列入后续里程碑 M5+，本期仅预留接口）

---

## 2. 技术选型

| 层 | 选择 | 说明 |
|---|---|---|
| 应用壳 | **Wails v3** | Go 后端 + 系统 WebView，单二进制、低内存。Service + Event 模型契合「后台压缩、前端看进度」 |
| 前端 | **Vue 3 + TypeScript + Vite** | 轻量、生态成熟，Wails 官方支持 |
| UI | **Tailwind CSS**（+ shadcn-vue 可选） | 快速构建表单/列表/进度 |
| 压缩 | **7-Zip ZS（mcmilk fork）`7z` 子进程，go:embed 内嵌** | 唯一同时满足 zstd/LZMA2 + AES-256 + 文件名加密 + 分卷 + 固实的方案 |
| 存储 | **`modernc.org/sqlite`（纯 Go，免 cgo）** | 保留可查询性，去掉 JDBC |
| 密码库 | **`golang.org/x/crypto/argon2` + AES-256-GCM** | 主密码派生 + 认证加密 |
| Excel 导出 | **`github.com/xuri/excelize/v2`** | `.xlsx` |
| ID | **`github.com/google/uuid`** | 内部主键 |
| 配置 | YAML/JSON 于 OS 配置目录 | GUI 托管，不手改 |

### 2.1 为什么内嵌 7zz 而不是纯 Go

- 纯 Go 的 `klauspost/compress` zstd 很快，但 **Go 没有成熟的 7z 写入器**，无法产出带 AES-256 + 固实压缩的 `.7z`。
- 7-Zip ZS 支持在 `.7z` 容器内使用 `-m0=zstd`，且 `7zAES` / `AES256CBC` codec 齐全，`-mhe=on` 加密头部。
- 代价：随包带平台二进制（~1–2MB），注意许可证（7-Zip 本体 LGPL；zstd=BSD、Brotli=MIT、LZ4=BSD 等）。个人自用无碍。

---

## 3. 目录结构

```
ElectricHamster/
├── main.go                     # Wails 启动入口 + 服务装配
├── runtime.go                  # Runtime：store/archiver/vault/运行状态
├── service_task.go             # TaskService（绑定层）
├── service_vault.go            # VaultService（绑定层）
├── views.go                    # 前端 DTO
├── Taskfile.yml / config.yml   # Wails v3 构建（Task）
├── build/                      # 打包资产（图标、清单、NSIS 脚本）
├── scripts/package-windows.sh  # 一键打包：exe + 便携 zip + NSIS 安装包
├── go.mod
├── DESIGN.md
├── internal/
│   ├── archive/                # 压缩引擎抽象
│   │   ├── engine.go           #   Archiver 接口 + 选项
│   │   ├── sevenzip.go         #   7-Zip 实现（调用/进度解析/列表/校验）
│   │   ├── resolver.go         #   二进制发现与释放（env/内嵌/本地/PATH）
│   │   ├── embed_windows.go    #   go:embed（-tags embed_runtime，分平台）
│   │   ├── embed_linux.go
│   │   ├── embed_common.go
│   │   ├── embed_stub.go       #   默认不内嵌
│   │   ├── progress.go         #   -bsp1 输出解析（退格/回车重绘）
│   │   └── runtime/            #   内嵌二进制暂存（gitignore）
│   │       ├── windows-amd64/7za.exe
│   │       └── linux-amd64/7zz
│   ├── job/                    # 任务引擎
│   │   ├── scanner.go          #   扫描 + content_sig
│   │   └── engine.go           #   worker 池 + 判重 + 取号 + 暂停/续跑 + 事件
│   ├── vault/                  # 密码库（vault.go/crypto.go/kdf.go/password.go）
│   ├── store/                  # SQLite（models.go/repo.go + migrations/）
│   ├── export/                 # xlsx 导出（excelize）
│   ├── legacy/                 # 旧 config.properties + db.sqlite3 导入
│   ├── db/                     # SQLite 打开 + 编号迁移器
│   ├── id/                     # 随机主键
│   └── config/                 # 默认路径
├── cmd/eh/                     # 命令行验证工具（M1~M3）
└── frontend/                   # Vue 3 + TS
    ├── src/App.vue
    ├── src/components/{TasksPanel,ArchivesPanel,VaultPanel}.vue
    └── bindings/               # wails3 生成的 TS 绑定
```

---

## 4. 压缩引擎设计

### 4.1 接口

```go
type Options struct {
    Format        string   // "7z"
    Codec         string   // "zstd" | "lzma2"
    Level         int      // 0..22 (zstd) / 0..9 (lzma2)
    Encrypt       bool
    HeaderEncrypt bool     // -mhe=on
    SplitSize     int64    // 0 = 不分卷
    Password      string   // 仅内存传递
    Threads       int
}

type Progress struct {
    Percent   float64
    Current   string
    BytesOut  int64
    BytesIn   int64
    Speed     int64
    ElapsedMS int64
}

type Archiver interface {
    Compress(ctx context.Context, srcDir, outFile string, opt Options, onProgress func(Progress)) error
    Extract(ctx context.Context, archive, outDir, password string) error
    List(ctx context.Context, archive, password string) ([]Entry, error)
    Verify(ctx context.Context, archive, password string) error
    Capabilities(ctx context.Context) ([]string, error) // 探测可用 codec
}
```

### 4.2 7-Zip 实现要点

> 本节结论已由 M1 在 Linux + 7-Zip ZS 26.02 上实测验证，详见 §10.1。

- **二进制发现**（`internal/archive/resolver.go`，优先级）：
  1. `$EH_SEVENZIP` 显式路径；
  2. 内嵌资源（`-tags embed_runtime`，见下）；
  3. 可执行文件同目录 / 当前目录的 `runtime/<os>-<arch>/` 或根目录；
  4. `$PATH` 中的 `7zz`/`7z`/`7za`/`7zr`（Windows 加 `.exe`）。
- **内嵌与释放**：`//go:embed runtime` 把 `internal/archive/runtime/<os>-<arch>/` 打进二进制，首次运行释放到
  `os.UserCacheDir()/electric-hamster/runtime/<sha256>/`，按内容哈希复用，避免重复释放与损坏。
  默认构建不内嵌（stub），发布构建加 `-tags embed_runtime`。已实测 `7zz` 为独立二进制（4.8MB，不依赖 `7z.so`）。
- **自检**：执行 `7z i`，解析确认 `ZSTD`、`7zAES` 存在；缺失则降级为 `lzma2` 并提示用户。
- **调用**（压缩）：
  ```
  7z a -t7z -m0=zstd -mx=<level> [-mhe=on] -bsp1 -bso0 -sccUTF-8 -y [-v<size>] [-mmt=N] -p<pwd> -- <tmp.7z> <folderName>
  ```
  工作目录设为源目录的父目录，归档内只保留文件夹名本身。
- **密码传递**：实测 `a` 可从 stdin 读密码，但 `x`/`l`/`t` **不读 stdin**（会返回 255 `Break signaled`）。
  为保证一致与可靠，统一使用显式 `-p<password>`，通过 `exec` 直接传参（无 shell，无注入风险）。
  代价是密码短暂出现在进程列表中；后续可选硬化（PTY 或 stdin+校验），见 §9。
- **进度**：`-bsp1` 把进度写到 stdout。7-Zip 用退格/回车原地重绘，`progress.go` 以状态机回放
  （`\b` 移光标、`\r`/`\n` 归位、字符覆写），提取每个可见帧的百分比/阶段/当前条目，并做去重。
  另对临时输出文件做 `stat` 以估算 `BytesOut`。
- **原子性**：先压到 `<out>.tmp`，成功后 `rename`；分卷时批量重命名 `<out>.7z.tmp.NNN` → `<out>.7z.NNN`。
  失败或取消时清理 `.tmp` 及分卷。
- **取消**：`context` 取消 → `exec.CommandContext` 终止 7-Zip 进程，并清理半成品。
- **校验**：可选在压缩后执行 `t`，失败则删除产物并报错（对应 `archives.status = corrupt`）。

### 4.3 降级与扩展

`Archiver` 为接口，后续可加入纯 Go `tar.zst + age` 引擎或「优先系统 7z」引擎，不影响上层。

---

## 5. 数据模型

SQLite，启用 WAL。表结构（迁移脚本管理）：

```sql
-- 任务定义
CREATE TABLE tasks (
  id              TEXT PRIMARY KEY,       -- uuid
  name            TEXT NOT NULL,
  source_dir      TEXT NOT NULL,
  dest_dir        TEXT NOT NULL,
  include_globs   TEXT,                   -- JSON array
  exclude_globs   TEXT,                   -- JSON array
  format          TEXT DEFAULT '7z',
  codec           TEXT DEFAULT 'zstd',
  level           INTEGER DEFAULT 15,
  encrypt         INTEGER DEFAULT 1,
  header_encrypt  INTEGER DEFAULT 1,
  split_size      INTEGER DEFAULT 0,
  password_policy TEXT NOT NULL,          -- JSON
  dedup_rule      TEXT DEFAULT 'content_sig',
  enabled         INTEGER DEFAULT 1,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);

-- 每次运行
CREATE TABLE runs (
  id          TEXT PRIMARY KEY,
  task_id     TEXT NOT NULL REFERENCES tasks(id),
  status      TEXT NOT NULL,              -- pending|running|paused|done|failed|canceled
  total       INTEGER DEFAULT 0,
  done        INTEGER DEFAULT 0,
  bytes_in    INTEGER DEFAULT 0,
  bytes_out   INTEGER DEFAULT 0,
  started_at  TEXT,
  finished_at TEXT,
  error       TEXT
);

-- 归档产物（替代原 t_pwd + 序号表）
CREATE TABLE archives (
  id           TEXT PRIMARY KEY,
  run_id       TEXT REFERENCES runs(id),
  task_id      TEXT REFERENCES tasks(id),
  seq          INTEGER NOT NULL,          -- 展示用序号（事务内原子取号）
  folder_name  TEXT NOT NULL,
  source_path  TEXT NOT NULL,
  archive_path TEXT NOT NULL,
  archive_size INTEGER DEFAULT 0,
  file_count   INTEGER DEFAULT 0,
  content_sig  TEXT NOT NULL,             -- xxhash(排序后的 相对路径+大小+mtime)
  password_id  TEXT,                      -- 指向金库条目；NULL 表示无密码
  status       TEXT DEFAULT 'ok',         -- ok|corrupt|missing
  created_at   TEXT NOT NULL
);
CREATE INDEX idx_archives_sig    ON archives(content_sig);
CREATE UNIQUE INDEX idx_archives_path ON archives(archive_path);

-- 全局计数器（替代 t_zipNO，事务内 UPDATE ... RETURNING 原子取号）
CREATE TABLE counters (
  name  TEXT PRIMARY KEY,
  value INTEGER NOT NULL
);
```

### 5.1 判重策略

- `content_sig` = 对源文件夹内所有文件按相对路径排序后，对 `(relPath, size, mtimeUnixNano)` 做 xxhash64。
- 若已有 `status='ok'` 且签名相同的归档，则跳过（可配置为「按名」或「按内容」）。
- 相比原项目按文件夹名判重：改名、移动目录不会重复压缩。

---

## 6. 密码库设计

### 6.1 金库格式

- **主密码** 经 **Argon2id**（参数写入文件头，便于升级）派生 32 字节主密钥。
- 主密钥加密一个**数据密钥**（DEK，随机 32B），条目用 DEK 做 **AES-256-GCM** 加密；换主密码只需重加密 DEK。
- 金库文件：`%APPDATA%\ElectricHamster\vault.db`（或 JSON + 二进制 blob），含 `kdf` 参数、salt、nonce、verifier。

```
vault_meta: { version, kdf:"argon2id", time, memory, parallelism, salt, verifier }
vault_items:{ id, label, password(enc), note(enc), created_at, updated_at }
```

### 6.2 密码策略（按任务配置）

```json
{ "mode": "random", "length": 20, "charset": "alnum+symbols" }
{ "mode": "fixed",  "value_ref": "vault:<id>" }
{ "mode": "prompt" }
```

- 随机密码用 `crypto/rand`，默认长度 ≥ 16。
- 任何情况下密码不写日志、不写明文文件。

### 6.3 导出

- 从金库 + `archives` 联表导出 `.xlsx`（`excelize`），列为
  `序号 | 文件夹名 | 归档路径 | 密码 | 创建时间 | 大小 | 文件数`。
- 可选：导出文件带主密码保护 / 仅导出本次运行。

---

## 7. 任务引擎

```
Scanner   → 列出源目录下待处理单元（顶层文件夹；可配置深度）
Planner   → 过滤 include/exclude、判重、原子取号、准备密码
Queue     → 持久化待办（runs + 内部 job 表），支持暂停/续跑
Worker    → N 个 worker 调用 Archiver，回报进度
Recorder  → 写 archives，更新 counters/runs
Notifier  → Emit 事件 + （可选）系统通知
```

### 7.1 并发模型

- 顶层文件夹为作业单元，`WorkerPool(n)`，`n` 默认 = `min(NumCPU, 可配)`。
- 每个作业持有 `context`，支持取消；全局暂停 = 停止派发新作业并等待当前作业收尾。
- 单个 7z 进程内部本身多线程（`-mmt`），故默认并发不宜过大，避免磁盘争抢。

### 7.2 事件（Wails）

| 事件名 | 载荷 |
|---|---|
| `job:started` | `{ runId, total }` |
| `job:progress` | `{ runId, seq, folder, percent, speed, bytesOut }` |
| `job:item_done` | `{ runId, seq, folderName, archivePath, ok, error }` |
| `job:finished` | `{ runId, status, done, bytesIn, bytesOut, elapsedMS }` |
| `vault:locked` / `vault:unlocked` | `{}` |
| `app:error` | `{ scope, message }` |

前端用 `@wailsio/runtime` 的 `Events.On(...)` 订阅。

### 7.3 断点续跑

- 作业在开始前写入队列（`pending`），完成后更新状态。
- 应用启动时若有 `running` 残留，标记为 `interrupted`，允许一键「继续」。

---

## 8. GUI 页面

| 页面 | 内容 |
|---|---|
| 仪表盘 | 任务卡片、运行中作业实时进度、累计压缩量统计 |
| 任务 | 任务列表 + 编辑表单：源/目标目录（原生对话框）、codec/等级、加密/文件名加密、分卷、密码策略、包含/排除 |
| 运行 | 实时进度条、速度、当前条目、日志流、暂停/取消 |
| 密码库 | 解锁、搜索、复制/显示、新增/编辑、导出 xlsx |
| 历史 | 运行记录、归档列表、打开目录、解压、校验 |
| 设置 | 语言(zh-CN)、主题、并发数、7z 运行时路径、临时目录、日志级别 |

Wails 绑定示例（目录选择）：

```go
func (s *TaskService) PickFolder() (string, error) {
    return s.app.Dialog.OpenFile().
        SetTitle("选择文件夹").
        CanChooseDirectories(true).
        CanChooseFiles(false).
        PromptForSingleSelection()
}
```

---

## 9. 安全清单

- [ ] 主密码 Argon2id；金库 AES-256-GCM；DEK 间接层支持改密。
- [ ] 密码不落盘、不写日志；7z 经 stdin 传入（**M1 实测 `a` 可行，但 `x/l/t` 不读 stdin**，暂统一用显式 `-p`；见 §10.1）。
- [x] `-mhe=on` 加密归档文件名（M1 已验证无密码无法列出）。
- [ ] 日志脱敏（不打印密码/绝对敏感路径可选）。
- [x] 归档后 `7z t` 完整性校验，失败则删除产物（M1 已实现）。
- [x] 半成品文件使用 `.tmp` 后缀，成功后原子改名（M1 已实现）。
- [ ] 可选：系统钥匙串缓存主密钥（`go-keyring`）。

---

## 10. 里程碑

| 阶段 | 交付 | 验收 |
|---|---|---|
| **M0 ✅** | Wails v3 + Vue3/TS 脚手架，窗口可启动 | 已生成并交叉编译 Windows exe，冒烟启动通过（§10.4） |
| **M1 ✅** | `archive` 接口 + 7z 发现/内嵌/调用/进度解析 + 单测 | 见 §10.1，Linux + 7-Zip ZS 26.02 实测通过 |
| **M2 ✅** | SQLite schema/迁移 + Argon2id 金库 + 解锁 | 见 §10.2，跨进程持久化与改主密码实测通过 |
| **M3 ✅** | 任务引擎：扫描/判重/取号/压缩/记录 + 事件 | 见 §10.3，真实目录跑通全流程，重跑跳过、改动重压 |
| **M4 ✅** | GUI：任务编辑、仪表盘、进度、密码库、xlsx 导出 | GUI MVP 完成（任务/归档/密码库/进度）；xlsx 导出留待 M5 |
| **M5 ✅** | xlsx 导出、暂停/续跑、旧数据导入 | 见 §10.7；设置页/i18n 暂缓 |

### 10.1 M1 验证记录（`internal/archive`, `cmd/eh`）

环境：WSL2 Linux x64，7-Zip ZS `26.02 ZS v1.5.7 R1`。

| 项 | 结论 |
|---|---|
| codec | `7zz i` 报告可用 `ZSTD`、`7zAES`、`LZMA2`、`BROTLI`、`BZip2` ✅ |
| 独立二进制 | `7zz` 单文件可运行，不依赖 `7z.so`（4.8MB）✅ |
| 压缩 | zstd `-mx` + AES-256 + `-mhe=on`，45MB 目录 → 34MB `.7z` ✅ |
| 进度 | 退格/回车重绘状态机正确解析出 scan/compress/header 帧 ✅ |
| 密码 | `a` 支持 stdin；`x/l/t` 不支持 → 统一显式 `-p` ✅ |
| 校验 | 正确密码 `t` 通过，错误密码失败 ✅ |
| 列表 | `-slt` 解析出 242 条（240 文件 + 2 目录），错误/无密码失败（头部加密）✅ |
| 解压 | 240 文件全部还原，内容逐字节一致 ✅ |
| 分卷 | `-v1MB` 生成 7 卷，`.001` 校验通过 ✅ |
| 取消 | `ctx` 取消后无残留产物 ✅ |
| 内嵌 | `-tags embed_runtime` 构建，释放到缓存并以哈希复用，`info` 正常 ✅ |
| 测试 | `go vet` + `go test ./...` 全绿（含集成测试，缺 7z 时自动 skip）✅ |

### 10.2 M2 验证记录（`internal/db`, `internal/store`, `internal/vault`）

技术栈：`modernc.org/sqlite`（纯 Go，免 cgo）、`golang.org/x/crypto/argon2`、AES-256-GCM。

| 项 | 结论 |
|---|---|
| SQLite 驱动 | `modernc.org/sqlite` 打开/迁移/查询正常，WAL + 外键 + busy_timeout ✅ |
| 迁移器 | 编号 `.sql` + `schema_migrations` 记录；可安全处理注释/字符串里的分号；重复打开幂等 ✅ |
| 应用 schema | `tasks`/`runs`/`archives`/`counters` 建表成功 ✅ |
| 金库 KDF | Argon2id（参数存头部，可调/可升级）；`argon2id time/memory/parallelism/salt` ✅ |
| 金库加密 | KEK 包裹 DEK（AES-256-GCM），条目用 DEK 加密；改主密码只重包 DEK ✅ |
| 解锁 | 错误主密码 → `ErrWrongPassword`；未解锁读 → `ErrLocked` ✅ |
| 持久化 | `init → put → 关闭 → 重开 → unlock → get` 跨进程取回一致 ✅ |
| 改主密码 | 旧密码失效、新密码可读，条目不重加密 ✅ |
| 明文检查 | 主密码、条目密码在 `vault.db`/`-wal` 中均不可见 ✅ |
| 密码生成 | `crypto/rand` + big.Int（无模偏差），默认 alnum，可选符号 ✅ |
| 测试 | `go vet` + `go test ./...` 全绿 ✅ |

> 设计说明：`items.label` 目前为**明文**（便于未解锁时列出/搜索）。label 即源文件夹名，与归档输出文件名同级，不算额外泄露；若后续需要隐藏，可改为加密 label + 独立索引。

### 10.3 M3 验证记录（`internal/config`, `internal/store`, `internal/job`）

流水线：`Scan(顶层目录) → ScanTree(content_sig) → 判重 → 原子取号 → 生成/取密码入金库 → 7z 压缩 → 记录归档 → 事件`。

| 项 | 结论 |
|---|---|
| 扫描 | 顶层子目录按名排序；支持 include/exclude glob（exclude 优先）✅ |
| content_sig | 对排序后的 `(相对路径,大小,mtime)` 作 SHA-256 取前 16 字节；改名/新增即失效 ✅ |
| 判重 | 二次运行 3 项全部跳过（done=0 skipped=3），未重复压缩 ✅ |
| 变化检测 | 改动 alpha 后仅重压 alpha（done=1 skipped=2），新序号 10003 ✅ |
| 取号 | `counters` + `INSERT ... ON CONFLICT DO UPDATE ... RETURNING` 原子自增，从 10000 起 ✅ |
| 密码 | 随机 20 位入金库，`archives.password_id` 关联；fixed/none 策略均可 ✅ |
| 记录 | `archives`（seq/路径/大小/文件数/签名/密码id）与 `runs`（状态/计数）正确写入 ✅ |
| 事件 | started / item_started / progress / item_done / item_skipped / finished 齐全 ✅ |
| 并发 | worker 池（默认 NumCPU，上限 16）；单连接 SQLite 串行写入安全 ✅ |
| 端到端 | 用金库密码 `7z t` 校验归档通过，错误密码被拒 ✅ |
| 真实集成测试 | `TestEngineWithRealArchiver`（含中文目录名）通过；无 7z 自动 skip ✅ |
| 测试 | `go vet` + `go test ./...` 全绿 ✅ |

> 待办：任务名默认取源目录名（`src`），GUI 里会改为显式命名；`depth>1`、暂停/续跑、断点恢复留待 M5。

### 10.4 GUI（M0/M4）验证记录

技术栈：Wails v3.0.0-beta.26（Go 后端 + WebView2）+ Vue 3 + TypeScript + Vite 8。开发在 WSL 完成，产物在 Windows 运行。

| 项 | 结论 |
|---|---|
| 脚手架 | `wails3 init -t vue` 模板整合进仓库；`build/` 仅保留 windows/linux 资产 ✅ |
| 绑定生成 | `wails3 generate bindings -ts`：2 services / 19 methods / 6 models ✅ |
| 前端构建 | Node 22 + `npm run build`（vue-tsc + vite）通过，产物 108KB js ✅ |
| 后端服务 | `VaultService`（创建/解锁/锁定/列表/显示/生成）、`TaskService`（增删改查/选目录/运行/取消/归档/校验/解压/引擎状态）、`Runtime` 装配 ✅ |
| 事件 | `job:event`、`vault:changed` 经 `app.Event.Emit` 推送，前端 `@wailsio/runtime` 订阅 ✅ |
| 交叉编译 | `GOOS=windows CGO_ENABLED=0 go build -tags "production,embed_runtime"` → `bin/ElectricHamster.exe`（17MB）✅ |
| 内嵌引擎 | Windows `7za.exe`（1.8MB，含 ZSTD/7zAES）按平台内嵌；首跑释放到缓存 ✅ |
| syso | `wails3 generate syso` 生成图标/清单（24KB）✅ |
| WebView2 | 运行时装 `154.0.4258.48` ✅ |
| 冒烟 | 在 Windows 启动 `bin/ElectricHamster.exe`，进程稳定运行 5s+（窗口正常打开）后关闭 ✅ |
| 内部测试 | `go vet` + `go test ./internal/... ./cmd/...` 全绿（根包为 GUI，Linux 侧不编译）✅ |

### 10.5 构建与运行

开发（Windows 侧，需装 Wails CLI + Node）：`wails3 dev`

从 WSL 交叉编译 Windows 产物：

```bash
# 1. 构建前端（需要 Node >= 20）
cd frontend && npm install && npm run build && cd ..
# 2. 内嵌引擎并编译（需要 internal/archive/runtime/windows-amd64/7za.exe）
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build \
  -tags "production,embed_runtime" -trimpath -buildvcs=false \
  -ldflags="-w -s -H windowsgui" -o bin/ElectricHamster.exe .
```

> 注意：`internal/archive/runtime/` 被 gitignore，克隆后需放置 `7za.exe`（或 `7z.exe`）后再用 `embed_runtime` 构建；不加该标签则回退到系统 7-Zip。
> GUI 根包依赖 WebView2/cgo，**不能在 Linux 上 `go build ./...`**；测试请用 `./internal/... ./cmd/...`。

### 10.6 反馈迭代（试用后修正）

实际用 100GB 游戏目录测试（zstd，压缩到约 75GB）后，按反馈修正三点：

| 问题 | 修正 |
|---|---|
| 多 worker 抢同一个进度条，进度左右横跳 | 引擎新增 `progressTracker`：按每个作业的**输入字节加权**聚合成整体百分比（全为 0 时退化为均权），事件新增 `overallPercent`；前端改为「整体进度条 + 每文件夹独立进度行」 |
| `content_sig` 判重过于敏感（游戏存档一变就重压） | 任务级 `dedup_rule` 可选：`content_sig`（默认）/`folder_name`（只认文件夹名，存档变动也跳过，且**跳过时不做目录遍历**）/`none`（每次都压）；`store.ArchiveByFolderName` 按 task+folder 名查询 |
| 密码库条目不能手动改 | `vault.Update(id,label,password)` + `VaultService.Update`；前端「改密」内联编辑。`archives.password_id` 不变，导出/校验仍按 id 取密码，不受影响 |

验证：新增 `TestEngineFolderNameDedup`（内容改动后仍跳过）、`TestProgressTrackerAggregate`（加权/均权聚合）、`TestUpdateEntry`（改密 + 持久化）；`go vet` + `go test ./internal/... ./cmd/...` 全绿。重新生成绑定（20 methods）、构建前端、交叉编译 exe 并冒烟启动通过。

### 10.7 M5 验证记录（xlsx / 暂停续跑 / 旧数据导入）

| 功能 | 实现 | 验证 |
|---|---|---|
| xlsx 导出 | `internal/export`（excelize）写出 `序号/文件夹/归档路径/密码/大小/文件数/创建时间`；`TaskService.ExportExcel` + 归档页「导出 Excel」+ CLI `eh export` | 导出的 xlsx 内含明文密码与中文标签，`excelize` 读回校验 ✅ |
| 暂停/续跑 | `job.Engine.Pause/Resume`（已派发的文件夹完成收尾后停止派发）；`TaskService.Pause/Resume/IsPaused`；`job:paused`/`job:resumed` 事件；运行面板按钮 | `TestEnginePauseResume`（暂停阻塞、继续放行、取消释放）✅ |
| 旧数据导入 | `internal/legacy`：解析旧 `config.properties` + `t_pwd`/`t_zipNO`，生成任务、导入归档台账、密码写入金库；`TaskService.ImportLegacy` + 任务页「导入旧数据」+ CLI `eh import` | 端到端：3 条旧记录 → 任务 + 3 归档 + 3 密码；旧密码可读、可导出 xlsx；重复导入幂等（skipped=3）；`archive_seq` 推进到 >10001 ✅ |

修复：`archives` 的可选外键（`run_id`/`task_id`/`password_id`）此前用 `""` 写入触发 `FOREIGN KEY` 失败，改为写 `NULL`（`nullIfEmpty`）。

未做（后续可选）：设置页、i18n、任务级时区/调度、xlsx 加密保护。

### 10.8 任务级编号与命名（反馈迭代）

原先 `archives.seq` 取自**全局** `archive_seq` 计数器，编号跨任务连续。按反馈改为**任务级**：

| 项 | 变更 |
|---|---|
| 编号 | 计数器键改为 `task:<taskID>`（`store.SeqCounterName`），每个任务从自己的起始编号独立递增 |
| 命名 | 任务新增 `name_prefix` / `name_suffix` / `seq_start`（迁移 `0002_task_naming.sql`）；输出名 = `前缀+编号+后缀+".7z"`（`Task.ArchiveFileName`） |
| 起始编号 | 创建任务时初始化计数器；编辑任务时用 `EnsureSeqAtLeast` **只增不减**，避免与已产出归档撞号 |
| 导入 | 旧数据导入后把该任务的计数器推进到 `max(旧id)` |

验证：`TestEnginePerTaskNamingAndNumbering`（前缀/后缀/起始编号 + 两任务计数器独立）；CLI 端到端：任务 A `game_{100,101,102}_a.7z`、任务 B `save_{1,2}.7z`，编号互不影响。

### 10.9 打包与仓库清理

- **打包脚本** `scripts/package-windows.sh`：构建前端 → 交叉编译 Windows exe（自动检测 `internal/archive/runtime/windows-amd64/` 决定是否 `embed_runtime`）→ 生成 `.syso` 图标/清单 → 便携 zip → NSIS 安装包（自动在 WSL 中通过 `wslpath` 定位 Windows `makensis`）。
  - `scripts/package-windows.sh [exe|installer|all]`，可用 `SKIP_FRONTEND=1`、`INSTALL_SCOPE=user|machine`。
  - 实测产出：`bin/ElectricHamster.exe`（20MB）、`bin/ElectricHamster-amd64-portable.zip`（8.2MB）、`bin/ElectricHamster-amd64-installer.exe`（9.9MB）。
  - 依赖：Go、Node、`zip`；安装包另需 NSIS（`scoop install nsis`）。
- **移除旧 Java 项目**：删除 `src/`（107 个文件）与 `pom.xml`；README 重写为当前 Go/Wails 项目说明。
- **`.gitignore` 修正**：此前 `eh` 规则会误伤 `cmd/eh/`，`resources/` 会误伤 Wails 资源；已改为精确路径。

---

## 11. 旧数据迁移（M5）

- 读取旧 `config.properties` → 生成一个默认任务。
- 读取旧 `db.sqlite3` 的 `t_pwd` + `t_zipNO` → 导入 `archives`（`archive_path` 按旧命名规则推算，`content_sig` 置空或按现存文件重算），密码条目导入金库。
- 旧 `.zip` 不重新压缩，仅登记。

---

## 12. 风险与待定

| 风险 | 缓解 |
|---|---|
| Windows 发行物命名/单文件差异（`7z.exe`+`7z.dll` vs `7zr.exe`） | Linux 侧已确认 `7zz` 独立可运行；Windows 侧留待拿到产物后 `7z i` 校对；resolver 已兼容多候选名 |
| 密码经 `-p` 暴露在进程命令行 | 仅本机、短暂；后续可改为 stdin（`a`）+ 校验，或 PTY 方案 |
| 内嵌二进制的许可证合规 | 记录各 codec 许可证，发布时附带 |
| zstd 与 LZMA2 压缩率/速度权衡 | 任务级可选 codec 与等级，默认 zstd 15 |
| 大目录长时间运行 | 并发可调、可暂停、断点续跑、日志落盘 |
| 密码库忘记主密码不可恢复 | 显式提示；导出明文台账作为应急备份 |

### 待定问题

1. Windows 上 7-Zip ZS 采用 `7z.exe`+`7z.dll` 还是更大的单文件？拿到产物后在 WSL/Windows 实测。（Linux 侧已闭环）
2. 是否需要「优先系统 7z」作为零体积选项？（暂缓）
3. 旧 `.zip` 是否需要提供「转 `.7z`」工具？（暂缓）
