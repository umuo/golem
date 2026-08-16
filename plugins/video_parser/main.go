package main

import (
	"fmt"
	"log/slog"

	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
	"github.com/wujunwei928/parse-video/parser"
)

type VideoParserPlugin struct {
	message message.Ability
}

func (v *VideoParserPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "video_parser",
		Author:      "ovo",
		Version:     "v1.0.0",
		Description: "视频在线解析插件",
		Priority:    100,
		Next:        false,
		AlwaysRun:   false,
	}
}

func (v *VideoParserPlugin) GetSubscriptions() []string {
	return []string{message.TypeText.Topic}
}

func (v *VideoParserPlugin) OnEvent(event *plugin.Event) (bool, error) {
	msg := event.Payload.(*plugin.Event_Message).Message
	if msg == nil || msg.Type.Code != message.TypeText.Code {
		return false, nil
	}

	info, err := parser.ParseVideoShareUrlByRegexp(msg.Content)
	if err != nil {
		if err.Error() == "str not have url" {
			return false, nil
		}
		slog.Warn("视频解析失败", "err", err)
		return false, err
	}

	targetUrl := info.VideoUrl
	if targetUrl == "" && len(info.Images) > 0 {
		targetUrl = info.CoverUrl
		if targetUrl == "" {
			targetUrl = info.Images[0].Url
		}
	}

	_, err = v.message.Send(&message.Message{
		Receiver: msg.Sender,
		Type:     message.TypeAppLink,
		Content:  fmt.Sprintf("[%s] %s", info.Title, targetUrl),
		Data: &message.Message_App{App: &message.AppData{
			SubType: 5,
			Title:   info.Title,
			Desc:    info.Author.Name,
			Url:     targetUrl,
			Xml:     info.CoverUrl,
		}},
	})

	if err != nil {
		slog.Warn("发送解析结果失败", "err", err)
		return false, nil
	}

	return true, nil
}

func main() {
	plugin.Start(&VideoParserPlugin{})
}
