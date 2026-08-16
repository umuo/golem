package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/cdn"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
	"github.com/wujunwei928/parse-video/parser"
)

// Config 视频解析插件配置
type Config struct {
	RedirectURL string `toml:"redirect_url" comment:"重定向 API 地址，例如：https://next-url-redirector.pages.dev/go?url="`
}

type VideoParserPlugin struct {
	plugin.ConfigAbility[Config]
	message    message.Ability
	cdn        cdn.Ability
	httpClient *http.Client
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

func (v *VideoParserPlugin) getRedirectVideoURL(rawVideoURL string) string {
	redirectURL := strings.TrimSpace(v.Config.RedirectURL)
	if redirectURL == "" || rawVideoURL == "" {
		return rawVideoURL
	}

	if strings.Contains(redirectURL, "{url}") {
		return strings.ReplaceAll(redirectURL, "{url}", url.QueryEscape(rawVideoURL))
	}

	if strings.HasSuffix(redirectURL, "=") {
		return redirectURL + url.QueryEscape(rawVideoURL)
	}

	if strings.Contains(redirectURL, "?") {
		return redirectURL + "&url=" + url.QueryEscape(rawVideoURL)
	}

	return redirectURL + "?url=" + url.QueryEscape(rawVideoURL)
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

	if v.httpClient == nil {
		v.httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	// 如果包含图集/图文，发送合并转发聊天记录
	if len(info.Images) > 0 {
		return v.sendImageRecord(msg.Sender, info)
	}

	videoURL := v.getRedirectVideoURL(info.VideoUrl)

	// 视频消息：发送链接卡片
	_, err = v.message.Send(&message.Message{
		Receiver: msg.Sender,
		Type:     message.TypeAppLink,
		Content:  fmt.Sprintf("[%s] %s", info.Title, videoURL),
		Data: &message.Message_App{App: &message.AppData{
			SubType: 5,
			Title:   info.Title,
			Desc:    info.Author.Name,
			Url:     videoURL,
			Xml:     info.CoverUrl,
		}},
	})

	if err != nil {
		slog.Warn("发送解析结果失败", "err", err)
		return false, nil
	}

	return true, nil
}

func (v *VideoParserPlugin) sendImageRecord(receiver *contact.Contact, info *parser.VideoParseInfo) (bool, error) {
	authorName := info.Author.Name
	if authorName == "" {
		authorName = "作者"
	}
	title := info.Title
	if title == "" {
		title = fmt.Sprintf("%s 的图文作品", authorName)
	}
	desc := fmt.Sprintf("%s: 共 %d 张图片", authorName, len(info.Images))

	items := v.buildImageRecordItems(info)
	xmlContent := buildChatRecordXML(title, desc, items, info.Author.Avatar)

	_, err := v.message.Send(&message.Message{
		Receiver: receiver,
		Type:     message.TypeApplication,
		Content:  fmt.Sprintf("[%s] %s", title, desc),
		Data: &message.Message_App{App: &message.AppData{
			SubType: 19,
			Title:   title,
			Desc:    desc,
			Xml:     xmlContent,
		}},
	})

	if err != nil {
		slog.Warn("发送图文合并转发失败", "err", err)
		return false, nil
	}

	return true, nil
}

func main() {
	plugin.Start(&VideoParserPlugin{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	})
}
