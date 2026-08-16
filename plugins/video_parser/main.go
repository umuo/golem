package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
	"github.com/wujunwei928/parse-video/parser"
)

// Config 视频解析插件配置
type Config struct {
	RedirectURL string `toml:"redirect_url" comment:"视频直链重定向 API 地址，例如：https://next-url-redirector.pages.dev/go?url="`
}

type VideoParserPlugin struct {
	plugin.ConfigAbility[Config]
	message    message.Ability
	httpClient *http.Client
}

func (v *VideoParserPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "video_parser",
		Author:      "ovo",
		Version:     "v1.0.0",
		Description: "视频/图文在线解析插件",
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

func downloadImage(ctx context.Context, client *http.Client, imgUrl string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", imgUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if strings.Contains(imgUrl, "xhscdn.com") || strings.Contains(imgUrl, "xiaohongshu.com") {
		req.Header.Set("Referer", "https://www.xiaohongshu.com/")
	} else if strings.Contains(imgUrl, "douyinpic.com") || strings.Contains(imgUrl, "byteimg.com") {
		req.Header.Set("Referer", "https://www.douyin.com/")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
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

	// 如果包含图集/图文，发送首张封面大图 + 编号直链清单（防刷屏）
	if len(info.Images) > 0 {
		return v.sendCoverAndImageLinks(msg.Sender, info)
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

// sendCoverAndImageLinks 发送首张高清封面大图 + 编号直链排版文本（防刷屏最佳实践）
func (v *VideoParserPlugin) sendCoverAndImageLinks(receiver *contact.Contact, info *parser.VideoParseInfo) (bool, error) {
	authorName := info.Author.Name
	if authorName == "" {
		authorName = "作者"
	}

	// 1. 尝试下载并发送首张大图（封面）
	firstImageURL := info.CoverUrl
	if firstImageURL == "" && len(info.Images) > 0 {
		firstImageURL = info.Images[0].Url
	}

	if firstImageURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		data, err := downloadImage(ctx, v.httpClient, firstImageURL)
		cancel()

		if err == nil && len(data) > 0 {
			_, err = v.message.Send(&message.Message{
				Receiver: receiver,
				Type:     message.TypeImage,
				Content:  "[图集封面]",
				Data: &message.Message_Image{
					Image: &message.ImageData{
						Media: &message.Media{
							Data: data,
						},
					},
				},
			})
			if err != nil {
				slog.Warn("发送首张封面图失败", "err", err)
			}
		} else {
			slog.Warn("下载首张封面图失败", "err", err)
		}
	}

	// 2. 发送排版优雅的标题、作者与原图直链清单
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📖 %s\n👤 %s\n\n🖼️ 图集（共 %d 张原图直链）：\n", info.Title, authorName, len(info.Images)))
	for i, img := range info.Images {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, img.Url))
		if img.LivePhotoUrl != "" {
			sb.WriteString(fmt.Sprintf("   🎬 实况动图: %s\n", img.LivePhotoUrl))
		}
	}

	_, err := v.message.Send(&message.Message{
		Receiver: receiver,
		Type:     message.TypeText,
		Content:  strings.TrimSpace(sb.String()),
	})
	if err != nil {
		slog.Error("发送图文直链清单失败", "err", err)
		return false, err
	}

	return true, nil
}

// sendFallbackVideoText 视频卡片发送失败时的文本降级
func (v *VideoParserPlugin) sendFallbackVideoText(receiver *contact.Contact, info *parser.VideoParseInfo) (bool, error) {
	authorName := info.Author.Name
	if authorName == "" {
		authorName = "作者"
	}

	targetURL := v.getRedirectVideoURL(info.VideoUrl)
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

func main() {
	plugin.Start(&VideoParserPlugin{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	})
}
