# Codex Deck

Codex Deck 是使用 Go 和 Wails 构建的跨平台桌面程序，用于管理 Codex CLI 账号、订阅额度和重置卡。

项目目前处于开发交接阶段，尚未实现。完整需求与验收条件见 [handoff.md](handoff.md)，开发约定见 [AGENTS.md](AGENTS.md)。

## 功能范围

- 添加、查看和删除账号，修改账号别名。
- 切换 Codex CLI 当前账号。
- 查看套餐、5H 和 Weekly 剩余额度及恢复时间。
- 查看重置卡数量及可用详情，手动使用重置卡。
- 通过 WebDAV 自动备份，手动备份和恢复。

## 技术方案

后端使用 Go 和 Wails v2。前端使用 React 19、TypeScript、HeroUI v3 和 Tailwind CSS v4，包管理器使用 pnpm。

登录、令牌刷新、用量查询、重置卡和 WebDAV 操作由 Go 直接通过 HTTP 完成。账号切换通过写入 Codex CLI 认证文件完成。

独立开发，不依赖 codex-auth，不兼容其配置或数据格式。程序不调用 codex CLI 或 app-server 子进程。

## 数据存储

Codex Deck 自身所有持久数据以明文 JSON 保存到 `~/.codex-deck.json`。其中包括账号认证、别名、用量缓存、界面设置和 WebDAV 配置。

不使用数据库、加密或系统凭据库。WebDAV 远端只保留一个 `codex-deck.json` 文件。

Codex CLI 继续使用自身的 `auth.json`。第一版只支持 CLI 文件凭据存储，要求 `cli_auth_credentials_store = "file"`。默认认证路径为 `~/.codex/auth.json`，设置 `CODEX_HOME` 时使用对应目录。

## 界面

界面采用极简、克制风格。主窗口包含顶部操作和账号表格，设置通过对话框打开。使用中性色、小圆角和少量强调色，主题跟随系统。

不添加欢迎语、功能介绍、统计卡片、侧边栏或常驻操作说明。仅在需要时显示进度、错误和确认。

## 实现依据

- [Wails 文档](https://wails.io/docs/introduction/)
- [HeroUI 文档](https://heroui.com/en/docs/react/getting-started)
- [Codex 认证文档](https://learn.chatgpt.com/docs/auth)
- [Codex 官方源码](https://github.com/openai/codex)

HTTP 接口以官方源码和实际响应为依据。官方客户端内部接口可能变化，不能将源码中出现的接口视为稳定的第三方 API 契约。
