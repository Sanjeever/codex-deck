# Codex Deck

Codex Deck 是使用 Go 和 Wails 构建的桌面程序，用于管理 Codex CLI 账号、订阅额度和重置卡。已实现完整业务流程与桌面界面。真实 OpenAI 授权、服务端接口及用户 WebDAV 服务仍需用户验证。

完整需求与验收条件见 [handoff.md](handoff.md)，开发约定见 [AGENTS.md](AGENTS.md)。

## 功能范围

- 通过系统浏览器进行 OAuth 授权码与 PKCE 登录，支持取消和重新登录。按用户与工作区身份去重，重新登录身份不匹配时拒绝更新。
- 查看、删除账号和编辑别名。添加与删除不修改 CLI 认证。
- 切换 Codex CLI 当前账号，按实际认证文件标记当前账号。非文件存储需确认后再修改对应 TOML 配置项，保留其他内容。
- 启动时及手动查询套餐、5H 和 Weekly 剩余额度与本地时区恢复时间。失败时保留缓存，缺失窗口显示未知。
- 查询重置卡数量、状态与到期时间，确认后使用。请求标识持久保存，未确定结果时沿用原请求重试；消费后重新查询用量和重置卡。
- 账号与设置变化后自动串行备份到 WebDAV，也可手动备份。失败保留本地变更并显示错误。
- 恢复前下载并校验备份，确认后整体替换本地数据。取消不修改数据，恢复不写 CLI 认证，也不上传回远端。

## 技术方案

后端使用 Go 和 Wails v2。前端使用 React 19、TypeScript、HeroUI v3 和 Tailwind CSS v4，包管理器使用 pnpm。

登录、令牌刷新、用量查询、重置卡和 WebDAV 操作由 Go 直接通过 HTTP 完成。账号切换通过写入 Codex CLI 认证文件完成。

独立开发，不依赖 codex-auth，不兼容其配置或数据格式。程序不调用 codex CLI 或 app-server 子进程。

## 数据存储

Codex Deck 自身所有持久数据以明文 JSON 保存到 `~/.codex-deck.json`。其中包括账号认证、别名、用量缓存、界面设置和 WebDAV 配置。

不使用数据库、加密或系统凭据库。WebDAV 远端只保留一个 `codex-deck.json` 文件。

Codex CLI 继续使用自身的 `auth.json`。第一版只支持 CLI 文件凭据存储，要求 `cli_auth_credentials_store = "file"`。默认认证路径为 `~/.codex/auth.json`，设置 `CODEX_HOME` 时使用对应目录。

刷新前读取并核对 CLI 当前身份与凭据，采用更新后的 CLI 令牌。刷新结果先保存到本地；写回前再次核对 CLI 文件，若 CLI 同时更新则明确报错并保留其文件。刷新令牌过期、被重复使用或失效时提示重新登录。

## 界面

界面采用极简、克制风格。主窗口包含顶部操作和账号表格，设置通过对话框打开。使用中性色、小圆角和少量强调色，主题跟随系统。

不添加欢迎语、功能介绍、统计卡片、侧边栏或常驻操作说明。仅在需要时显示进度、错误和确认。

## 开发与构建

本机已验证 Go 1.23.12、Node.js 26.11.0、pnpm 11.20.0、Wails CLI v2.12.0。前端依赖版本由 `frontend/pnpm-lock.yaml` 锁定。其他平台依赖见 [Wails 安装文档](https://wails.io/docs/gettingstarted/installation/)。macOS 需要 Xcode Command Line Tools。

在仓库根目录启动开发环境。

```sh
wails dev
```

构建桌面程序。Wails 会调用 pnpm 安装依赖、执行 TypeScript 检查并构建前端。

```sh
wails build -clean
```

macOS 构建产物为 `build/bin/codex-deck.app`。可以双击打开，或执行下面命令。

```sh
open build/bin/codex-deck.app
```

单独构建前端及运行检查。

```sh
pnpm --dir frontend build
go test -race ./...
pnpm --dir frontend typecheck
```

Go 入口嵌入 `frontend/dist`，首次单独运行 Go 测试前需要先构建前端。直接在浏览器运行 Vite 只能加载页面；完整程序通过 Wails 调用 Go。

## 验证状态

- Go 测试及竞争检测通过。覆盖串行保存、损坏配置、身份去重、CLI 格式与配置保留、令牌更新冲突、401 后刷新、缓存保留、消费去重和恢复边界。
- OAuth、用量、重置卡与 WebDAV 使用本地 HTTP 服务验证请求和响应处理。测试令牌只在隔离环境使用，不代表真实 OpenAI 服务验证。
- 前端 TypeScript 检查和生产构建通过。
- 本机 macOS arm64 Wails 桌面构建通过，原生窗口在隔离配置下启动。通过真实 Wails 通道验证设置、别名、删除、隔离 CLI 切换、WebDAV 上传、取消恢复和确认恢复；检查浅色与深色主题。
- 未执行 Windows 或 Linux 桌面构建。

用户仍需验证浏览器真实登录与重新登录、真实账号套餐与用量、重置卡详情与实际消费，以及自己的 WebDAV 服务。账号切换后需重新启动 Codex CLI，再检查其实际账号。开发验证没有读写用户实际配置、使用真实令牌发请求或消费真实重置卡。

## 实现依据

- [Wails 文档](https://wails.io/docs/introduction/)
- [HeroUI 文档](https://heroui.com/en/docs/react/getting-started)
- [Codex 认证文档](https://learn.chatgpt.com/docs/auth)
- [Codex 官方源码](https://github.com/openai/codex)
- [浏览器登录与 PKCE](https://github.com/openai/codex/blob/main/codex-rs/login/src/server.rs)
- [令牌刷新](https://github.com/openai/codex/blob/main/codex-rs/login/src/auth/manager.rs)
- [CLI 认证存储](https://github.com/openai/codex/blob/main/codex-rs/login/src/auth/storage.rs)
- [请求头与后端客户端](https://github.com/openai/codex/blob/main/codex-rs/backend-client/src/client.rs)
- [用量与重置卡 HTTP 请求](https://github.com/openai/codex/blob/main/codex-rs/backend-client/src/client/rate_limit_resets.rs)
- [重置卡响应类型](https://github.com/openai/codex/blob/main/codex-rs/backend-client/src/types.rs)

实现依据为 2026-10-12 阅读的官方源码。登录沿用源码中的客户端标识 `app_EMoamEEZ73f0CkXaXp7hrann` 与回调 `http://127.0.0.1:1455/auth/callback`，监听端口被占用时明确报错。使用 Bearer 认证与 `ChatGPT-Account-Id` 请求头；重置卡消费请求包含 `redeem_request_id` 和可选的 `credit_id`。

官方客户端内部 HTTP 接口可能变化，也可能限制第三方客户端使用。源码核对与本地测试不能证明真实服务已接受本应用；授权拒绝、HTTP 错误或未知消费结果会明确显示，不改用 CLI 请求。当前版本不支持 FedRAMP 工作区，识别到对应令牌时明确报错。
