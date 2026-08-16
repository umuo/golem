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

// getFinalRedirectURL 跟踪 302 重定向获取真实播放直链
func (v *VideoParserPlugin) getFinalRedirectURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // 仅获取 302 Location，不下载数据流
		},
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return rawURL
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1")
	req.Header.Set("Range", "bytes=0-0")

	resp, err := client.Do(req)
	if err != nil || resp == nil {
		return rawURL
	}
	defer resp.Body.Close()

	if loc := resp.Header.Get("Location"); loc != "" {
		return loc
	}
	return rawURL
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
		slog.Warn("卡片发送失败，触发降级文本发送", "err", err)
		return v.sendFallbackVideoText(msg.Sender, info)
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
		slog.Warn("图文合并转发发送失败，触发降级文本发送", "err", err)
		return v.sendFallbackImageText(receiver, info)
	}

	return true, nil
}

// sendFallbackVideoText 视频卡片发送失败时的文本降级
func (v *VideoParserPlugin) sendFallbackVideoText(receiver *contact.Contact, info *parser.VideoParseInfo) (bool, error) {
	// 获取 302 重定向后的真实直链
	finalURL := v.getFinalRedirectURL(info.VideoUrl)
	if finalURL == "" {
		finalURL = info.VideoUrl
	}

	targetURL := v.getRedirectVideoURL(finalURL)

	authorName := info.Author.Name
	if authorName == "" {
		authorName = "作者"
	}

	content := fmt.Sprintf("🎬 %s\n👤 %s\n🔗 视频直链：\n%s", info.Title, authorName, targetURL)

	_, err := v.message.Send(&message.Message{
		Receiver: receiver,
		Type:     message.TypeText,
		Content:  content,
	})
	if err != nil {
		slog.Error("降级发送纯文本依然失败", "err", err)
		return false, err
	}
	return true, nil
}

// sendFallbackImageText 图文转发发送失败时的文本降级
func (v *VideoParserPlugin) sendFallbackImageText(receiver *contact.Contact, info *parser.VideoParseInfo) (bool, error) {
	authorName := info.Author.Name
	if authorName == "" {
		authorName = "作者"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📖 %s\n👤 %s\n🖼️ 图片直链（共 %d 张）：\n", info.Title, authorName, len(info.Images)))
	for i, img := range info.Images {
		if i >= 9 {
			sb.WriteString(fmt.Sprintf("... 其余 %d 张图请查看原链接\n", len(info.Images)-9))
			break
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, img.Url))
	}

	_, err := v.message.Send(&message.Message{
		Receiver: receiver,
		Type:     message.TypeText,
		Content:  strings.TrimSpace(sb.String()),
	})
	if err != nil {
		slog.Error("降级发送纯文本依然失败", "err", err)
		return false, err
	}
	return true, nil
}

func main() {
	plugin.Start(&VideoParserPlugin{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	})
}
