# Codex Deck

Codex Deck 是管理 Codex CLI 多账号、套餐额度和重置卡的桌面应用。后端使用 Go 和 Wails v2，前端使用 React 19、TypeScript、HeroUI v3 和 Tailwind CSS v4。

![Codex Deck 账号列表与操作菜单](docs/screenshots/accounts.png)

## 功能

- 浏览器 OAuth 登录、取消登录和重新登录，按用户与工作区身份区分账号。
- 启动或刷新时同步本地 Codex CLI 当前 ChatGPT 账号，自动导入新账号并更新已有账号凭据。
- 查看账号、编辑别名和删除记录。
- 切换 Codex CLI 账号，按实际认证文件标记当前账号。
- 查看套餐、5H 和 Weekly 剩余额度及本地时区恢复时间，查询失败时保留缓存。
- 查询重置卡数量、状态和到期时间，确认后使用；结果未确定时沿用原请求标识重试。
- WebDAV 自动备份、手动备份和整体恢复。

主窗口采用账号表格，设置通过对话框打开，主题跟随系统。所有业务 HTTP 请求由 Go 发起，前端通过 Wails 调用后端。

## 开发与构建

开发环境使用 Go 1.23、Node.js 26、pnpm 11.20.0 和 Wails CLI v2.12.0。macOS 需要 Xcode Command Line Tools，其他平台依赖见 [Wails 安装文档](https://wails.io/docs/gettingstarted/installation/)。

在仓库根目录启动开发环境。

```sh
wails dev
```

构建桌面应用。

```sh
wails build -clean
```

Wails 会通过 pnpm 安装依赖，执行 TypeScript 检查并构建前端。macOS 构建产物为 `build/bin/codex-deck.app`，可双击打开或执行下面命令。

```sh
open build/bin/codex-deck.app
```

单独构建前端与运行检查。

```sh
pnpm --dir frontend build
go test -race ./...
pnpm --dir frontend typecheck
```

Go 入口嵌入 `frontend/dist`，首次单独运行 Go 测试前需先构建前端。完整应用通过 Wails 启动，单独运行 Vite 不包含 Go 后端。

## 数据与账号切换

应用自身所有持久数据保存到 `~/.codex-deck.json`，包括账号令牌、别名、用量缓存、消费请求标识和 WebDAV 配置。文件使用明文 JSON，包含敏感凭据。

账号切换通过写入 Codex CLI 的 `auth.json` 完成。CLI 目录由 `CODEX_HOME` 决定，未设置时使用 `~/.codex`。仅支持文件凭据存储。

```toml
cli_auth_credentials_store = "file"
```

CLI 使用其他存储方式时，应用会在切换前请求确认，再修改对应配置项。切换后需重新启动 Codex CLI。添加或删除账号不会修改 CLI 当前凭据。

启动或刷新时读取本地文件凭据，按用户与工作区身份导入新账号或更新已有账号，保留别名和用量缓存。手动在 CLI 中 logout 再 login 后，点击刷新即可同步。logout 后清除当前账号标记，已管理账号仍保留。凭据同步本身不改写 CLI 认证，数据变化后按设置自动备份；凭据未变化时不重复保存或备份。

凭据无效时显示错误，不覆盖已保存认证，仍可通过浏览器添加账号。已有账号保留比 CLI 文件更新的认证，避免覆盖尚未切换到 CLI 的重新登录结果。

令牌刷新前读取并核对 CLI 身份与凭据，写回前再次检查。CLI 同时更新认证时，应用保留其文件并显示错误。刷新令牌过期、被重复使用或失效时需重新登录。

## WebDAV 备份

在设置中填写远端目录地址、用户名和密码，远端文件名固定为 `codex-deck.json`。备份包含完整配置与账号认证，使用明文 JSON，不保留历史版本。

坚果云的 `https://dav.jianguoyun.com/dav/` 是同步目录列表，不能直接存放文件。填写这个根地址时，应用自动使用 `codex-deck/codex-deck.json`，首次备份会创建 `codex-deck` 目录。填写具体目录地址或使用其他 WebDAV 服务时，文件仍直接保存在所填目录中。

账号添加、重新登录、删除、别名修改、成功切换、令牌更新和设置修改后自动备份。单纯刷新用量不触发上传，下一次备份会包含最新缓存。上传串行执行，失败不会撤销本地变更，可手动重试。

恢复先下载并校验备份，确认后整体替换本地账号、认证和设置。取消恢复不修改本地数据。恢复不会切换 CLI 账号，也不会自动上传回远端。

## 兼容性与验证

本项目已通过 Go 测试与竞争检测、前端类型检查和生产构建，并在 macOS arm64 验证桌面构建与启动。Windows 和 Linux 桌面构建尚未验证。

认证、用量、重置卡和 WebDAV 请求通过隔离配置与本地 HTTP 服务测试。WebDAV 另经坚果云真实服务验证上传、读取与备份解析，测试文件在验证后删除。真实 OpenAI 授权、服务端用量和重置卡消费尚未完成集成验证。

OpenAI 接口依据官方 Codex 源码实现，属于客户端内部接口，可能变化或限制第三方客户端使用。接口错误会直接显示。当前不支持 FedRAMP 工作区。

## 开发参考

开发约定见 [AGENTS.md](AGENTS.md)。

- [Wails 文档](https://wails.io/docs/introduction/)
- [HeroUI 文档](https://heroui.com/en/docs/react/getting-started)
- [Codex 认证文档](https://learn.chatgpt.com/docs/auth)
- [浏览器登录与 PKCE](https://github.com/openai/codex/blob/main/codex-rs/login/src/server.rs)
- [令牌刷新](https://github.com/openai/codex/blob/main/codex-rs/login/src/auth/manager.rs)
- [CLI 认证存储](https://github.com/openai/codex/blob/main/codex-rs/login/src/auth/storage.rs)
- [后端客户端](https://github.com/openai/codex/blob/main/codex-rs/backend-client/src/client.rs)
- [用量与重置卡请求](https://github.com/openai/codex/blob/main/codex-rs/backend-client/src/client/rate_limit_resets.rs)
- [重置卡响应类型](https://github.com/openai/codex/blob/main/codex-rs/backend-client/src/types.rs)
