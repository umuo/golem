package main

import (
	"log/slog"
	"sync"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

const pluginVersion = "0.2.1"

// Config controls the OneBot 11 compatible WebSocket endpoint.
type Config struct {
	Listen                    string `toml:"listen" comment:"WebSocket 监听地址"`
	Path                      string `toml:"path" comment:"WebSocket 路径"`
	AccessToken               string `toml:"access_token" comment:"Bearer 鉴权令牌，为空时关闭鉴权"`
	Trigger                   string `toml:"trigger" comment:"消息转发触发前缀"`
	HeartbeatIntervalSeconds  int    `toml:"heartbeat_interval_seconds" comment:"心跳间隔，0 表示关闭"`
	WriteTimeoutSeconds       int    `toml:"write_timeout_seconds" comment:"单帧写入超时"`
	MaxFrameBytes             int64  `toml:"max_frame_bytes" comment:"客户端请求帧最大字节数"`
	ClientBuffer              int    `toml:"client_buffer" comment:"每个客户端的发送缓冲帧数"`
	RemoteMediaTimeoutSeconds int    `toml:"remote_media_timeout_seconds" comment:"下载远程媒体超时"`
	MaxMediaBytes             int64  `toml:"max_media_bytes" comment:"发送媒体最大字节数"`
	AllowLocalFiles           bool   `toml:"allow_local_files" comment:"是否允许 action 读取本地文件"`
}

func defaultConfig() Config {
	return Config{
		Listen:                    "127.0.0.1:3001",
		Path:                      "/",
		AccessToken:               "change-me",
		Trigger:                   "/qq",
		HeartbeatIntervalSeconds:  30,
		WriteTimeoutSeconds:       5,
		MaxFrameBytes:             1 << 20,
		ClientBuffer:              128,
		RemoteMediaTimeoutSeconds: 30,
		MaxMediaBytes:             20 << 20,
		AllowLocalFiles:           false,
	}
}

// OneBotPlugin exposes Golem messages through a focused OneBot 11 compatible
// WebSocket surface. IDs intentionally remain strings because Golem uses wxid
// and @chatroom identifiers rather than numeric QQ identifiers.
type OneBotPlugin struct {
	plugin.ConfigAbility[Config]

	message message.Ability
	contact contact.Ability

	mu      sync.Mutex
	gateway *gateway
}

func newPlugin() *OneBotPlugin {
	return &OneBotPlugin{
		ConfigAbility: plugin.ConfigAbility[Config]{Config: defaultConfig()},
	}
}

func main() {
	slog.Info("[onebot11] 插件启动中")
	p := newPlugin()
	if err := registerCommands(p); err != nil {
		slog.Error("[onebot11] 注册命令失败", "err", err)
		return
	}
	plugin.Start(p)
}
