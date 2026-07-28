<p align="center">
  <img src="assets/logo.png" alt="PS5 FTP Manager" width="128" height="128"/>
</p>

<p align="center">
  <b>PS5 FTP Manager</b><br/>
  在 NAS 与 PS5 FTP 服务之间传输和管理文件的飞牛 fnOS 图形化工具
</p>

<p align="center">
  <b>简体中文</b>
  ·
  <a href="README.md">English</a>
</p>

<p align="center">
  <b><a href="#特点">特点</a></b>
  ·
  <b><a href="#安装">安装</a></b>
  ·
  <b><a href="#使用">使用</a></b>
  ·
  <b><a href="#截图">截图</a></b>
  ·
  <b><a href="#开发">开发</a></b>
  ·
  <b><a href="#致谢">致谢</a></b>
</p>

---

<p align="center">
  <a href="https://github.com/aydencharles/ps5-ftp-fnOS/releases"><img src="https://img.shields.io/github/v/release/aydencharles/ps5-ftp-fnOS" alt="release"/></a>
  <a href="https://github.com/aydencharles/ps5-ftp-fnOS/stargazers"><img src="https://img.shields.io/github/stars/aydencharles/ps5-ftp-fnOS?style=flat" alt="stars"/></a>
  <a href="https://github.com/aydencharles/ps5-ftp-fnOS/network/members"><img src="https://img.shields.io/github/forks/aydencharles/ps5-ftp-fnOS" alt="forks"/></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-GPLv3-blue.svg" alt="license"/></a>
  <img src="https://img.shields.io/badge/-fnOS-0ea5e9?style=flat" alt="fnOS"/>
  <img src="https://img.shields.io/badge/-PS5-003791?style=flat&logo=PlayStation" alt="PS5"/>
  <img src="https://img.shields.io/badge/Go-00ADD8?style=flat&logo=go&logoColor=white" alt="Go"/>
</p>

<p align="center">
  面向 <b>飞牛 fnOS</b> 的第三方应用（<code>.fpk</code>），可将目录与游戏镜像上传到 PS5、<br/>
  将 PS5 文件下载至 fnOS，并通过 <b>zftpd</b> 或 <b>ftpsrv</b> 管理远端文件。
</p>

<br>

# 截图

<div align="center">
  <a href="assets/screenshots_01.png">
    <img src="assets/screenshots_01.png" alt="fnOS 存储浏览" width="31%" style="padding: 4px; background: #f6f8fa; border: 1px solid #d0d7de; border-radius: 12px;" />
  </a>
  <a href="assets/screenshots_02.png">
    <img src="assets/screenshots_02.png" alt="传输任务中心" width="31%" style="padding: 4px; background: #f6f8fa; border: 1px solid #d0d7de; border-radius: 12px;" />
  </a>
  <a href="assets/screenshots_03.png">
    <img src="assets/screenshots_03.png" alt="PS5 文件管理器" width="31%" style="padding: 4px; background: #f6f8fa; border: 1px solid #d0d7de; border-radius: 12px;" />
  </a>
</div>

<p align="center"><sub>fnOS 存储浏览 &nbsp;·&nbsp; 实时传输任务 &nbsp;·&nbsp; PS5 文件管理器</sub></p>

<br>

# 特点

为 NAS 场景设计的 PS5 文件传输管理器，无需挂载 FTP，也不依赖外部传输工具。

- **双向文件传输** — 支持 fnOS → PS5 上传，以及 PS5 → fnOS Library Root 下载
- **友好的本地浏览器** — 以存储位置展示 fnOS 文件，隐藏 `/vol2/1000/...` 一类内部路径
- **游戏内容识别** — 识别含 `eboot.bin` 的目录及 `.exfat`、`.ffpfs`、`.ffpfsc`、`.phu` 镜像，同时允许选择任意文件和目录
- **持久化任务队列** — 基于 SQLite，支持取消、重试、删除历史记录、真实字节进度、平滑速度与 ETA
- **可控并发** — 每台 PS5 同时运行一个任务，不同 Profile 可并行，单任务支持 1–4 路文件连接
- **安全替换** — 临时文件、尺寸校验、可回滚替换，以及单次任务内 REST/STOR 或 REST/RETR 断点续传
- **远端文件管理** — 浏览、搜索、新建目录、重命名、移动、下载和永久删除 PS5 文件
- **原生 fnOS 界面** — Vue 3、TypeScript、Pinia、LESS、TDesign Vue Next 与 Lucide，适配 fnOS iframe

<br>

# 安装

### 运行环境

1. **飞牛 fnOS**（x86_64）
2. PS5 上已启动且可连接的 FTP 服务：
   - **zftpd**（主机端服务通常使用端口 `2120`）
   - **ftpsrv**（通常使用端口 `2121`）
3. 可信局域网；推荐 PS5 与 NAS 使用有线连接

> [!WARNING]
> 应用以 root 运行，以便访问 fnOS 存储卷；其 HTTP 服务有意不提供登录验证。  
> 只能在可信局域网中使用，禁止将端口 **8100** 暴露到互联网。

### 安装包

1. 从 [Releases](https://github.com/aydencharles/ps5-ftp-fnOS/releases) 下载最新 `.fpk`  
   （或自行构建，见 [开发](#开发)）。
2. 在飞牛 **应用中心** 中安装第三方应用包。
3. 打开 **PS5 FTP Manager**。

应用监听端口 **8100**（`manifest` 中的 `service_port`）。

<br>

# 使用

### 连接 PS5

1. 在 PS5 上启动 zftpd 或 ftpsrv，记录 IP 地址和端口。
2. 打开 **设置**，添加一个 PS5 Profile。
3. 选择对应预设，填写主机、凭据和允许访问的基础目录后保存。
4. 新建传输前先执行连接测试。

FTP 使用明文被动模式。旧版 PS5 固件写入内部 `/data` 时可能受 PFS 加密驱动影响而速度较慢，外置 USB 存储通常更快。

### 传输文件

1. 打开 **新建传输**。
2. 从 fnOS Library Root 中选择一个或多个文件或目录。
3. 选择 PS5 Profile、目标目录和冲突策略。
4. 创建任务，在 **任务** 页面查看进度、当前文件、速度与 ETA。
5. 如需反向复制，在 **PS5 文件** 中选择内容，点击 **下载**，再选择 fnOS 目标位置。

排队或运行中的任务均可取消；终态任务可以重试或删除历史记录。删除任务记录不会删除已经传输的实际文件。

### 冲突策略

| 策略 | 行为 |
| --- | --- |
| 智能处理 | 同尺寸文件跳过，不同尺寸文件安全替换；默认策略 |
| 全部覆盖 | 所有同名文件重新传输并安全替换 |
| 遇到冲突停止 | 发现同名文件后立即使任务失败 |

目录始终非破坏性合并，目标端的额外文件不会被删除。上传与下载都会先写入同目录临时文件，校验尺寸后再替换为正式文件。

服务重启后，运行中的任务会变为 **已中断**，不会自动续传；原本排队但未开始的任务仍保留。重试会创建新的尝试并重新扫描源文件。

<br>

# 开发

```shell
git clone https://github.com/aydencharles/ps5-ftp-fnOS.git
cd ps5-ftp-fnOS
```

### 依赖

| 依赖 | 用途 |
| --- | --- |
| Go 1.26 | 编译后端；`github.com/jlaffaye/ftp v0.2.1` 要求此版本 |
| Node.js 22+ 与 pnpm 10+ | 检查并构建 Vue 前端 |
| `fnpack` / 飞牛打包工具链 | 生成可安装的 `.fpk` |

本机 Go 版本较旧时，可通过 `GOTOOLCHAIN=auto` 自动取得所需工具链。

### 测试

```shell
# 后端单元测试
go test ./src/backend/... -count=1

# fnOS start / status / stop 生命周期测试
./scripts/test-lifecycle.sh

# 前端检查
pnpm --dir src/frontend install --frozen-lockfile
pnpm --dir src/frontend lint
pnpm --dir src/frontend typecheck
pnpm --dir src/frontend test
```

### 构建

```shell
# 推荐：运行全部检查，构建前端与 linux/amd64 后端、暂存应用，然后打包 → dist/
./scripts/build.sh

# 本机没有 fnpack 时，仅构建并暂存
SKIP_FNPACK=1 ./scripts/build.sh

# 跳过检查（不推荐；前端生产构建仍会执行）
SKIP_TEST=1 ./scripts/build.sh
```

常用环境变量：

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `DIST_DIR` | `$PROJECT_ROOT/dist` | 构建产物目录 |
| `PACKAGE_DIR` | `$PROJECT_ROOT/packaging` | fnpack 工程目录 |
| `GOOS` / `GOARCH` | `linux` / `amd64` | Go 编译目标 |
| `FNPACK` | `fnpack` | fnpack 可执行文件 |
| `SKIP_FNPACK` | `0` | 设为 `1` 时跳过 `.fpk` 打包 |
| `SKIP_TEST` | `0` | 设为 `1` 时跳过后端、生命周期及前端检查 |

### 产物

| 路径 | 说明 |
| --- | --- |
| `dist/server` | 面向所选 Go 目标的静态后端程序 |
| `dist/*.fpk` | 飞牛可安装包 |
| `packaging/app/` | 构建生成的运行时暂存目录 |

> [!TIP]
> `dist/` 与 `packaging/app/` 是构建产物，请通过 [Releases](https://github.com/aydencharles/ps5-ftp-fnOS/releases) 分发发布版本。

### 本地开发

```shell
# 先安装依赖并构建前端
pnpm --dir src/frontend install
pnpm --dir src/frontend build

# 使用 .dev-data/ 启动后端：http://localhost:8100
./scripts/dev.sh
```

| 方法 | 接口 | 用途 |
| --- | --- | --- |
| `GET` | `/healthz` | 进程与 SQLite 健康状态 |
| `GET` | `/api/v1/bootstrap` | UI 启动快照 |
| `GET/POST/PUT/DELETE` | `/api/v1/profiles` | PS5 Profile 管理 |
| `POST` | `/api/v1/profiles/{id}/test` | 连接与列表能力测试 |
| `GET` | `/api/v1/library/roots` | fnOS 友好存储位置 |
| `GET` | `/api/v1/library/entries` | 本地只读浏览 |
| `GET` | `/api/v1/ps5/{id}/entries` | PS5 目录浏览 |
| `POST` | `/api/v1/ps5/{id}/operations` | 新建、重命名、移动或删除远端内容 |
| `GET/POST` | `/api/v1/tasks` | 查询或创建传输任务 |
| `GET/DELETE` | `/api/v1/tasks/{id}` | 查询单个任务或删除其终态记录 |
| `POST` | `/api/v1/tasks/{id}/cancel` | 取消任务 |
| `POST` | `/api/v1/tasks/{id}/retry` | 重试终态任务 |
| `GET` | `/api/v1/events` | SSE 任务快照流 |
| `GET/PUT` | `/api/v1/settings` | 查询或更新传输并发数 |

Library API 只接受 `{root_id, path}` 定位信息，不接受客户端提供的 fnOS 绝对路径。后端会解析符号链接并拒绝越过 Library Root 的路径；PS5 端操作同样受每个 Profile 的基础目录约束。

### 运行时环境变量

路径与端口优先读取 fnOS 注入变量，开发时可使用对应覆盖项。

| 变量 | 用途 |
| --- | --- |
| `TRIM_APPDEST` / `APP_DIR` | 应用目录 |
| `TRIM_PKGVAR` / `DATA_DIR` | SQLite、密钥、PID 与日志的持久化目录 |
| `TRIM_PKGTMP` / `TEMP_DIR` | 临时数据目录 |
| `TRIM_SERVICE_PORT` / `PORT` / `SERVICE_PORT` | 监听端口（默认 `8100`） |
| `UI_DIR` | 已构建的前端目录 |
| `LISTEN_ADDR` / `BIND_ADDR` | 绑定地址（默认 `0.0.0.0`） |

`packaging/cmd/main` 会导出 fnOS 运行时变量并管理服务进程。

### 仓库结构

```text
.
├── src/
│   ├── backend/             Go HTTP API、FTP Adapter、SQLite 与任务队列
│   └── frontend/            Vue 3 / TypeScript / TDesign 前端
├── packaging/               fnpack 元数据、权限、UI 配置与生命周期脚本
├── scripts/                 开发、生命周期测试与一键构建脚本
├── docs/adr/                架构决策记录
├── CONTEXT.md               领域术语
├── THIRD_PARTY_NOTICES.md   第三方许可说明
├── go.mod
└── README.md
```

<br>

# 反馈问题前

1. 确认 PS5 FTP 服务正在运行，且 fnOS 能访问其 IP 地址与端口。
2. 检查 Profile 的服务预设、凭据与基础目录是否匹配 FTP 服务。
3. 确保 PS5 与 NAS 位于同一可信局域网；传输大型游戏镜像时优先使用有线网络。
4. 查看任务错误以及 `${TRIM_PKGVAR}/server.log` 中的后端日志。
5. 先在 [Issues](https://github.com/aydencharles/ps5-ftp-fnOS/issues) 搜索类似报告；新建问题时请附上 fnOS 版本、PS5 FTP 服务、应用版本、日志与复现步骤。

<br>

# 贡献

- 较大功能请先开 issue 讨论，避免重复劳动。
- 改动尽量聚焦，风格与现有代码保持一致。
- 提交前请使用 `./scripts/build.sh`（或 `SKIP_FNPACK=1 ./scripts/build.sh`）验证。
- 欢迎向当前开发分支发起 Pull Request。

<br>

# 致谢

本项目基于开源组件与飞牛 fnOS 第三方应用框架构建。

- [jlaffaye/ftp](https://github.com/jlaffaye/ftp) — FTP 客户端与传输能力
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — 纯 Go 的持久化任务与 Profile 存储
- [Vue](https://vuejs.org/)、[TDesign Vue Next](https://github.com/Tencent/tdesign-vue-next) 与 [Lucide](https://lucide.dev/) — Web UI
- 飞牛 **fnOS** 第三方应用框架

依赖许可详见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

<br>

# 许可证

本项目采用 [GNU General Public License v3.0](LICENSE)。

第三方组件遵循各自许可证，详见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

> 本项目为非官方工具，与 Sony Interactive Entertainment **无任何关联**。  
> “PlayStation”“PS5”等为相应权利人的商标。  
> 仅用于合法持有内容的个人备份与管理。风险自担，不提供任何担保。
