# QWEN.md

本文件为 Qwen Code 在本仓库工作时的上下文说明。

## 项目概述

openwechat 是一个 Go 语言实现的个人微信机器人 SDK（非可执行程序），基于 Web 微信协议封装登录、消息收发、联系人管理、群管理等能力。

- 模块路径：`github.com/eatmoreapple/openwechat`
- 包路径：仓库根目录下只有一个库包 `pkg/openwechat`，包名为 `openwechat`
- Go 版本：`1.23`（CI 使用 `1.23.x`）
- 唯一直接依赖：`github.com/skip2/go-qrcode`（终端二维码渲染）

使用方通过 `go get github.com/eatmoreapple/openwechat/pkg/openwechat` 引入，通常作为库嵌入自己的机器人程序。

## 目录结构

```
.
├── pkg/openwechat/          # 全部库代码（单一包，无子包）
│   ├── bot.go               # Bot 生命周期、登录、消息循环、退出
│   ├── bot_login.go         # 扫码/热登录/免扫码登录、登录状态码、BotPreparer 选项
│   ├── caller.go            # 业务层请求封装与响应解析（Caller）
│   ├── client.go            # 原始 HTTP 请求层（Client）、HttpHook、分块上传
│   ├── mode.go              # normal / desktop 两种请求构造模式
│   ├── url.go               # 微信接口地址常量
│   ├── user.go              # User / Self / Members，发送与批量发送
│   ├── relations.go         # Friend / Group / Mp 及其集合
│   ├── message.go           # Message、各类消息判断与回复
│   ├── message_handle.go    # MessageMatchDispatcher 路由分发
│   ├── message_processor.go # 消息拉取与解析
│   ├── storage.go           # HotReloadStorage（热登录凭据持久化）
│   ├── session.go           # Session：BaseRequest / LoginInfo / WebInitResponse
│   ├── sync_check.go        # 长轮询状态与 Selector 常量
│   ├── parser.go / base_response.go / entity.go / errors.go / emoji.go ...
│   └── stringer.go          # go generate 生成的枚举 String() 方法
├── docs/                    # Sphinx 文档（bot.md / user.md / message.md）
├── Makefile                 # test / lint / vet / tidy 快捷命令
└── .github/workflows/go.yml # CI：test + vet + golangci-lint
```

## 架构分层

请求方向自上而下，新增接口时应保持分层：

1. **Bot**（`bot.go`）：对外入口，负责登录流程、维护 `Session` 上下文、驱动消息长轮询、触发回调。
2. **Caller**（`caller.go`）：业务编排层，处理上传/发送的组合逻辑、JSON/XML 解析、错误映射。
3. **Client**（`client.go`）：纯 HTTP 层，构造具体 URL、Cookie jar、重试（`MaxRetryTimes`）、`HttpHooks`。
4. **Mode**（`mode.go`）：按 `normal`（网页版）或 `desktop`（UOS electron 套壳）组装登录相关请求头与参数。

关键运行时结构：

- `Session`（`Storage` 字段）持有 `Request *BaseRequest`、`LoginInfo`、`Response *WebInitResponse`（含 `SyncKey`、`ContactList`）。
- `Bot.runMessageLoop` → `syncCheck`（长轮询）→ 有消息时 `syncMessage` → `handleMessages` → `MessageHandler`。
- 登录方式：`Bot.Login()`（扫码）、`Bot.HotLogin(storage)`、`Bot.PushLogin(storage)`；`HotReloadStorage` 定义为 `io.ReadWriter`，默认实现 `NewFileHotReloadStorage(filename)`。
- 模式通过 `openwechat.DefaultBot(openwechat.Desktop)` 或 `openwechat.Normal` 传入（`WithUUIDOption` 等为登录选项）。

## 常用 API 形态

```go
bot := openwechat.DefaultBot(openwechat.Desktop)
bot.UUIDCallback = openwechat.PrintlnQrcodeUrl
bot.MessageHandler = func(msg *openwechat.Message) {
    if msg.IsText() && msg.Content == "ping" {
        _, _ = msg.ReplyText("pong")
    }
}
if err := bot.Login(); err != nil { /* handle */ }
self, _ := bot.GetCurrentUser()
friends, _ := self.Friends()
_ = bot.Block()
```

- 路由分发：`dispatcher := openwechat.NewMessageMatchDispatcher()` + `OnText/OnGroup/OnFriend...`，最终赋值 `bot.MessageHandler = dispatcher.AsMessageHandler()`；`SetAsync(true)` 可异步执行。
- 发送消息统一走 `Self.SendXxxToFriend/Group/Friends/Groups`，媒体参数为 `io.Reader`。
- 底层网络方法（`Client` / `Caller`）均以 `context.Context` 作为首个参数；上层实体方法（如 `Friend.SendText`）不接收 context，而是复用 `Bot` 持有的可取消 context。

## 构建、运行与测试

```bash
go build ./...                      # 编译检查
go test ./...                       # 全部测试（集成测试默认跳过）
make test-unit                      # 仅纯单元测试（网络/逻辑）
OPENWECHAT_INTEGRATION=1 go test ./pkg/openwechat   # 集成测试，需真实扫码登录
go vet ./...
golangci-lint run ./...
go mod tidy
```

- 集成测试位于 `bot_test.go`，全部通过 `skipIntegrationTest(t)` 由环境变量 `OPENWECHAT_INTEGRATION=1` 控制；未设置时不应视为失败。
- CI（`.github/workflows/go.yml`）在 push/PR 到 `master` 时运行 `go test ./...`、`go vet ./...` 与 `golangci-lint`。
- 修改 `MessageType`、`Ret`、`AppMessageType` 枚举后需执行 `go generate ./pkg/openwechat` 重新生成 `stringer.go`（依赖 `stringer` 工具）。
- 若 `proxy.golang.org` 不可达（国内网络常见），可改用 `GOPROXY=https://goproxy.cn,direct go build ./...`。

## 开发约定

- 注释与文档使用中文；导出符号需有中文 doc 注释。对外错误值集中定义在 `errors.go`，使用 `errors.Is/As` 判断。
- 新增微信接口遵循“Client 方法 + `ClientXxxOptions` 结构体 → Caller 方法 + `CallerXxxOptions` → 上层实体方法”的三段式，保持与现有命名一致。
- 错误包装优先使用 `fmt.Errorf("...: %w", err)`；网络错误通过 `NetworkErr` / `IsNetworkError` 区分。
- 按钮式的可选行为用接口选项实现（`BotPreparer` / `BotLoginOption` / `BotOptionGroup`），不要随意给 `Bot` 增加布尔字段。
- 媒体文件上传通过 `readerToFile` 落临时文件；非 `*os.File` 的 reader 会被完整写入临时文件（受 `TMPDIR` 影响）。
- 已标记 `Deprecated` 的 API（如 `SetRemarkNameToFriend`、`RenameGroup`、`RemoveMemberFromGroup`、`NewJsonFileHotReloadStorage`）保留签名，不要直接删除。
- 本仓库为公共库：不要修改导出 API 的语义或签名而不评估兼容性；新增字段注意零值可用。
- 提交信息使用一句简洁的中文概括。

## 常见陷阱

- **消息处理是串行的**：`MessageHandler` 在消息循环 goroutine 中依次调用，耗时操作会阻塞后续消息同步；需要并发时自行 `go` 或使用 `dispatcher.SetAsync(true)`。
- **凭据敏感**：`DumpTo` / `DumpHotReloadStorage` 会写出 Cookie、`LoginInfo`、`SyncKey` 等登录态，切勿提交到仓库或打印到日志。
- **联系人缓存无锁**：`Self.members/friends/groups/mps` 为惰性缓存，`Friends(true)` 等参数用于强制刷新；并发访问同一 `Bot` 时需自行串行化。
- **协议易变**：登录与同步依赖微信 Web 协议及正则解析（`global.go` 中的 `uuidRegexp`、`syncCheckRegexp` 等），接口字段变化会导致解析失败，错误信息通常表现为 `uuid does not match` / `parse sync key failed`。
- **上传限制**：`chunkSize = 512KB`，超过 `needCheckSize`（25MB）会先做秒传检查；`maxFileUploadSize`/`maxImageUploadSize` 常量当前标记为 `unused`，不要假设已有强校验。
- **临时目录**：`readerToFile` 使用 `os.CreateTemp("", "*")`，在 `TMPDIR` 不可写的环境下（如部分 Android/Termux 配置）会失败，需先设置可写的 `TMPDIR`。
- **`Bot.Block()` 前置条件**：必须在登录成功后调用，否则返回错误；主动退出用 `Exit()` / `ExitWith(err)`，退出原因通过 `CrashReason()` 获取。

## 文档

- 在线文档：https://openwechat.readthedocs.io/zh/latest/
- 本地 Sphinx 源文件位于 `docs/`（`bot.md`、`user.md`、`message.md`），修改公共 API 时同步更新对应文档。
