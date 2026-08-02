# OneBot 11 兼容网关插件

`onebot11` 将 Golem 的微信消息能力转换成 OneBot 11 风格的 WebSocket
事件和 Action，主要用于让现有的 OneBot 客户端接入 Golem。

消息采用命令触发式转发：只有 `/qq <内容>` 命令会进入 OneBot，
例如 `/qq hello` 在 OneBot 侧收到的内容是 `hello`。命令与内容之间需要空格。
群聊命令会自动附加一个指向当前机器人的 `at` 消息段，以兼容只响应群聊 @ 的
OneBot 客户端。

由于微信使用 `wxid_xxx`、`xxx@chatroom` 等字符串标识，本插件有意将
`self_id`、`user_id`、`group_id` 和 `message_id` 输出为字符串。客户端应当
使用字符串字段接收这些 ID。

## 已支持的协议范围

### 事件

- 私聊和群聊 `post_type=message` 事件
- `text`、`at`、`image`、`record`、`video`、`location` 消息段
- WebSocket 连接生命周期事件
- 可选心跳事件

其中 Golem 的应用消息会降级为 `text`；表情会映射为 `image`。客户端可以
忽略暂不认识的消息段。

### Action

| Action | 说明 |
|---|---|
| `send_private_msg` | 发送私聊消息 |
| `send_group_msg` | 发送群聊消息 |
| `send_msg` | 根据 `message_type` 发送消息 |
| `delete_msg` | 撤回消息 |
| `get_login_info` | 获取当前微信账号标识与昵称 |
| `get_status` | 获取网关状态 |
| `get_version_info` | 获取插件版本 |

发送消息目前支持 `text`、`at` 和 `image`。图片来源支持：

- `http://` 或 `https://` URL
- `base64://...`
- `data:image/...;base64,...`
- 本地路径或 `file://`（需要显式开启 `allow_local_files`）

包含图片的混合消息会拆成多条微信消息，Action 回执返回最后一条消息的
`message_id`。

## 构建与部署

在仓库根目录执行：

```bash
task build:onebot11
```

或者在插件目录构建：

```bash
cd plugins/onebot11
go build -o golem_plugin_onebot11 .
```

将可执行文件放到 Host 的 `plugins` 目录后，在 Host 工作目录下的
`plugins/config.toml` 中配置：

```toml
[onebot11]
enable = true
mode = "blacklist"
limits = []

[onebot11.config]
# Java 服务和 Golem 位于不同机器时改成 0.0.0.0:3001，并配置防火墙。
listen = "127.0.0.1:3001"
path = "/"
access_token = "replace-with-a-strong-token"
trigger = "/qq"
heartbeat_interval_seconds = 30
write_timeout_seconds = 5
max_frame_bytes = 1048576
client_buffer = 128
remote_media_timeout_seconds = 30
max_media_bytes = 20971520
allow_local_files = false
```

首次加载时 Host 也会生成默认配置。修改监听地址、路径或 Token 后，应执行：

```text
/pm reload onebot11
```

## Java 客户端配置

对于 `boot-qq-rebot` 中的 `OneBotWebSocketListener`，配置示例为：

```yaml
config:
  robot:
    web-socket-url: ws://127.0.0.1:3001/
    access-token: replace-with-a-strong-token
    api-response-timeout-millis: 30000
```

Java DTO 中至少需要将以下字段从 `Long` 改为 `String`：

- `self_id`
- `user_id`
- `group_id`
- `message_id`
- Action 参数中的对应 ID
- Action 回执中的 `data.message_id`

## 撤回限制

Golem 撤回消息时还需要会话接收者，而 OneBot `delete_msg` 只有
`message_id`。插件会缓存最近 10,000 条已接收或由网关发送的消息路由。
插件重启后缓存会清空，因此无法通过 `delete_msg` 撤回重启前的消息。

## 安全建议

- 不要在局域网或公网监听时留空 `access_token`。
- 对外监听时使用防火墙限制客户端来源。
- `allow_local_files` 默认关闭，避免远端客户端读取 Golem 所在机器的文件。
- 该插件是面向 Golem/微信语义的 OneBot 11 兼容子集，不承诺实现全部 QQ
  管理类 Action。
