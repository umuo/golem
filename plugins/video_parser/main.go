package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
	"github.com/wujunwei928/parse-video/parser"
)

// Config 视频解析插件配置
type Config struct {
	RedirectURL string `toml:"redirect_url" comment:"视频重定向 API 地址，例如：https://next-url-redirector.pages.dev/go?url="`
	MaxImages   int    `toml:"max_images" comment:"图集最大发送张数，默认为 9 张"`
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

	// 如果包含图集/图文，直接在会话中发送原生高清图片
	if len(info.Images) > 0 {
		return v.sendDirectImages(msg.Sender, info)
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

// sendDirectImages 并发下载并逐张发送原生微信图片
func (v *VideoParserPlugin) sendDirectImages(receiver *contact.Contact, info *parser.VideoParseInfo) (bool, error) {
	maxImages := v.Config.MaxImages
	if maxImages <= 0 {
		maxImages = 9
	}

	imagesToFetch := info.Images
	if len(imagesToFetch) > maxImages {
		imagesToFetch = imagesToFetch[:maxImages]
	}

	type downloadResult struct {
		index int
		data  []byte
		err   error
	}

	results := make([]downloadResult, len(imagesToFetch))
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for i, img := range imagesToFetch {
		wg.Add(1)
		go func(idx int, targetUrl string) {
			defer wg.Done()
			data, err := downloadImage(ctx, v.httpClient, targetUrl)
			results[idx] = downloadResult{index: idx, data: data, err: err}
		}(i, img.Url)
	}
	wg.Wait()

	authorName := info.Author.Name
	if authorName == "" {
		authorName = "作者"
	}

	sentCount := 0
	for _, res := range results {
		if res.err != nil || len(res.data) == 0 {
			slog.Warn("下载图片失败，跳过", "idx", res.index, "err", res.err)
			continue
		}

		_, err := v.message.Send(&message.Message{
			Receiver: receiver,
			Type:     message.TypeImage,
			Content:  fmt.Sprintf("[%d/%d]", res.index+1, len(imagesToFetch)),
			Data: &message.Message_Image{
				Image: &message.ImageData{
					Media: &message.Media{
						Data: res.data,
					},
				},
			},
		})
		if err != nil {
			slog.Warn("发送图片失败", "idx", res.index, "err", err)
			continue
		}
		sentCount++
	}

	// 发送图文说明摘要
	summary := fmt.Sprintf("📖 %s\n👤 %s", info.Title, authorName)
	if len(info.Images) > len(imagesToFetch) {
		summary += fmt.Sprintf("\n(图集共 %d 张，已发送前 %d 张高清原图)", len(info.Images), len(imagesToFetch))
	}

	_, _ = v.message.Send(&message.Message{
		Receiver: receiver,
		Type:     message.TypeText,
		Content:  summary,
	})

	if sentCount == 0 && len(info.Images) > 0 {
		// 如果一张图片都没成功发送，降级为直链文本
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
