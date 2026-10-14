# 开发与发布

本文提供可执行的开发、验证和发布入口。具体版本与参数以仓库配置为准，架构边界见 [架构与行为契约](architecture.md)。修改工作流或运行方式时，只更新受影响的命令与说明。

## 工具链与配置来源

| 内容 | 事实来源 |
| --- | --- |
| Go 版本与依赖 | [`go.mod`](../go.mod)、`go.sum` |
| React 等前端依赖、pnpm 版本、前端命令 | [`frontend/package.json`](../frontend/package.json)、`pnpm-lock.yaml` |
| 发布使用的 Node 版本、Wails CLI、平台和系统依赖 | [发布工作流](../.github/workflows/release.yml) |
| Wails 开发与构建入口 | [`wails.json`](../wails.json) |
| 程序版本与系统版本信息同步 | [`scripts/version.mjs`](../scripts/version.mjs)、[`version.go`](../version.go) |

安装 Go、Node、配置指定 pnpm，以及对应平台原生依赖。平台要求见 [Wails 安装文档](https://wails.io/docs/gettingstarted/installation/)。macOS 构建需要 Xcode Command Line Tools；Linux 需要 C 工具链、GTK 与匹配的 WebKitGTK；Windows 运行需要 WebView2。

在仓库根目录安装与项目依赖一致的 Wails CLI，然后安装前端依赖。以下安装命令适用于 Bash 和 PowerShell：

```sh
go install "github.com/wailsapp/wails/v2/cmd/wails@$(go list -m -f '{{.Version}}' github.com/wailsapp/wails/v2)"
pnpm --dir frontend install --frozen-lockfile
```

## 启动与构建

```sh
wails dev
wails build -clean
```

Wails 构建命令安装依赖并构建前端。产物位于 `build/bin`，macOS 产物为完整 `.app`。单独运行 Vite 不提供 Go 绑定，不能代替完整桌面验证。

Linux 使用 WebKitGTK 4.1 时，开发、构建及涉及桌面依赖的 Go 检查均需携带 `webkit2_41` 标签，保持编译与运行库一致。例如：

```sh
wails dev -tags webkit2_41
wails build -clean -tags webkit2_41
go test -tags webkit2_41 ./...
```

本地构建默认标记为开发版，不能自动安装更新。需要验证正式版更新流程时显式加入发布标记；系统依赖仍沿用当前平台对应标签。

```sh
wails build -clean -ldflags "-X main.releaseBuild=true"
```

这只是构建操作，不会发布。正式 CI 另注入实际提交号用于排错。

## 验证与隔离

前端命令在 `frontend/package.json` 定义。常用检查如下，按实际改动选择，不要求每次全跑：

```sh
pnpm --dir frontend typecheck
pnpm --dir frontend test
pnpm --dir frontend build
go test ./...
```

Go 入口嵌入前端构建产物，干净检出首次运行根包 Go 检查前需构建前端。`-race` 仅在需要并发检查时使用，要求启用 CGO 并提供可用 C 编译器；平台标签同样适用。

Go 测试使用临时目录、本地签发令牌和本地 HTTP 服务；前端测试使用替身 Wails 绑定。正常启动完整应用会访问真实用户配置和 CLI 目录，不能把开发启动当作隔离测试。涉及账号保存、CLI 写入、备份恢复与更新替换的验证应使用隔离数据，不能覆盖用户实际状态。

会话解析和检索测试使用临时目录中的合成 rollout JSONL，前端会话测试使用替身 Wails 绑定。针对会话功能可运行：

```sh
go test ./internal/sessions
go test . -run TestSessionBindingsIndependentOfAccounts
pnpm --dir frontend test src/sessions/SessionBrowser.test.tsx src/app/App.test.tsx src/app/useKeyboardShortcuts.test.tsx
```

原型入口已移除，正式会话页面通过主窗口顶部切换访问。桌面验证需同时隔离用户目录和 `CODEX_HOME`，只放入合成会话，不提供真实令牌或 WebDAV 配置。浏览器替身可以检查布局和竞态，但不证明真实 Wails/WebView 文件读取已通过。协议核对的提交与支持范围见 [本地会话浏览](usage.md#本地会话浏览)。

[真实 WebDAV 测试](../internal/deck/webdav_integration_test.go) 只有配置测试服务环境变量后才启用，并会操作远端测试文件。获得明确授权后才可运行，应使用独立空目录；具体变量以测试入口为准。已有文件时跳过不代表集成验证通过。

编译、模拟测试与真实验证应分开记录。用测试替身不能证明 OpenAI 授权或卡片消费成功，CI 原生构建不能证明桌面托盘、通知权限、原生文件对话框和自动升级实际可用。只改文档或注释时检查内容、链接与最终 diff，无需重跑编译或测试。

## 版本与标签发布

`frontend/package.json` 的 `version` 是程序版本源。bump 命令先检查对应更新日志存在且非空，再同步 `wails.json` 中的系统产品版本，不自动提交或打标签。缺少或空白日志时，两个版本文件保持原值。应用状态 JSON 的格式版本属于数据模型，不能随发布 bump 修改。

每个版本的日志保存在 `docs/releases/<version>.md`。根据上个发布标签以来的用户可见变化编写简体中文说明，有内容才保留“新增、优化、修复”等分组。日志写结果与必要操作，不直接堆放 commit message。版本文件和对应日志须放入同一个 bump 提交。

以下使用一个示例正式版本，执行时替换为待发布版本。先创建并编写 `docs/releases/1.0.1.md`，再执行：

```sh
pnpm --dir frontend bump 1.0.1
node scripts/version.mjs check v1.0.1
```

`check` 同时检查标签、两个版本文件与对应日志。bump 脚本的隔离测试可运行 `node --test scripts/version.test.mjs`。检查版本 diff 与更新日志后，将版本文件和日志一并提交，确保待发布提交已进入 `main`。只有获发布授权时创建并推送标签：

```sh
git tag -a v1.0.1 -m "发布 v1.0.1"
git push origin main
git push origin v1.0.1
```

标签应指向已经检查的发布提交。已有公开标签与附件不作为日常修改对象，后续变化通过新版本发布。bump 工具当前只接受正式三段数字版本，预发布流程不属于其已有能力。

推送版本标签触发工作流。工作流验证标签与产品版本一致，在目标系统测试、构建并打包；全部成功后上传附件并公开完整 Release。当前矩阵、附件规则、权限与依赖以工作流为准，不另维护发布平台清单。更改附件命名或格式时，同时核对 [`internal/update`](../internal/update) 的匹配与解包契约。

### macOS 签名与公证

工作流使用以下 GitHub Secrets；具体读取位置以工作流为准，凭据不得写入仓库或日志。

| Secrets | 用途 |
| --- | --- |
| `MACOS_CERTIFICATE` | Developer ID p12 证书的 Base64 内容 |
| `MACOS_CERTIFICATE_PASSWORD` | 证书密码与临时 keychain 配置 |
| `MACOS_SIGNING_IDENTITY` | 签名身份 |
| `APPLE_ID`、`APPLE_TEAM_ID`、`APPLE_APP_PASSWORD` | 公证认证 |

配置证书后执行签名、公证并附加票据。未提供证书时，当前工作流明确输出未签名状态并继续打包。因此“macOS 签名与公证”步骤返回成功不一定代表已签名，需查看实际分支与日志。安装位置权限、系统拦截和真实重启仍需目标系统验证。

## 验证证据与外部资料

[v1.0.0 构建与发布记录](https://github.com/Sanjeever/codex-deck/actions/runs/38344875071) 证明该标签的 CI 测试、三平台构建和发布完成，不代表后来源码已发布，也不代表系统权限、签名或升级验收完成。其他版本以对应 Actions 与 Release 记录为准，不在文档维护滚动“已通过”清单。

仓库中的测试说明其覆盖的契约；外部服务或真实桌面的结果只适用于相应环境与版本。既有 WebDAV 兼容性经验见使用指南，需要重验时读取对应协议代码和测试，不把历史观察当作服务商永久契约。

- [Wails 文档](https://wails.io/docs/introduction/)
- [HeroUI 文档](https://heroui.com/en/docs/react/getting-started)
- [官方 Codex 源码](https://github.com/openai/codex)：认证、文件凭据、后台客户端和重置卡协议的依据。
