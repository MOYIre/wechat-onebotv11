# AstrBot 对接

本仓库新增了 `openwechat-onebot`，将 openwechat 的微信消息转换为 OneBot v11，并通过反向 WebSocket 连接 AstrBot 的 `aiocqhttp` 适配器。

## 构建

```bash
gofmt -w pkg/onebot cmd/openwechat-onebot
go build ./cmd/openwechat-onebot
```

生成的可执行文件为 `openwechat-onebot`。

## 配置

复制 `onebot.example.json` 为 `onebot.json`，按 AstrBot 的 aiocqhttp 适配器端口修改：

```json
{
  "ws_url": "ws://127.0.0.1:6199/ws",
  "access_token": "",
  "hot_reload_file": "hotreload.json",
  "login_mode": "desktop",
  "max_reconnect_interval_seconds": 30
}
```

AstrBot 侧需要启用 OneBot v11 / aiocqhttp 的反向 WebSocket。AstrBot 默认监听配置为主机 `0.0.0.0`、端口 `6199`、路径 `/ws`；客户端的 `ws_url` 则填写 AstrBot 从 openwechat 所在环境可访问的地址。若 AstrBot 在另一台设备或容器中运行，将 `127.0.0.1` 改为 AstrBot 所在主机可访问的地址。

如果 AstrBot 配置了反向 WebSocket token，`access_token` 必须设置为相同值。`hotreload.json` 包含 Cookie、登录信息和 SyncKey，已经加入 `.gitignore`，不要提交或打印它的内容。

## 运行

```bash
./openwechat-onebot -config onebot.json
```

也可以用参数临时覆盖配置：

```bash
./openwechat-onebot \
  -ws ws://127.0.0.1:6199/ws \
  -token 'your-token' \
  -hot-reload hotreload.json \
  -mode desktop
```

首次运行会在终端输出二维码，扫码成功后自动保存热登录凭据。后续运行优先尝试热登录，热登录失效时回退到扫码登录。终端二维码是默认行为，不会自动打开外部浏览器。

## 支持范围

已实现 OneBot v11 反向 WebSocket 的：

- 生命周期连接事件和心跳事件
- 文本消息、图片消息上报
- 文本和图片发送
- `get_login_info`
- `get_friend_list`
- `get_group_list`
- `get_group_info`
- `get_group_member_list`
- `get_group_member_info`
- `get_msg`
- `delete_msg`（仅限机器人自己发送且仍在微信可撤回时间内的消息）
- `get_status`
- `get_version_info`
- `can_send_image`
- `can_send_record`

微信字符串 UserName 会通过稳定哈希映射为 OneBot 的 `int64` ID。相同 UserName 在重启后仍会得到相同 ID；发送消息时如果本地还没有反向映射，会扫描联系人列表补全。

OneBot 的混合文本/图片消息会拆分为多条微信消息，因为微信发送接口不支持单条消息混合消息段。入站图片上限为 4 MiB，超出时改为 `[图片]` 文本占位；语音、视频、文件、表情、群管理等消息类型暂不转换。

## 验证

```bash
GOPROXY=https://goproxy.cn,direct go test ./...
go vet ./...
go build ./...
```

集成测试和真实登录仍需要微信账号及扫码，默认的 `go test ./...` 不会执行真实微信登录。
