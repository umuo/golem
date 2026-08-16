package main

import (
	"bytes"
	"context"
	"encoding/xml"
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

	// 如果包含图集/图文，发送首张封面大图 + 合并转发聊天记录
	if len(info.Images) > 0 {
		return v.sendCoverAndImageRecord(msg.Sender, info)
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

type recordItem struct {
	Name      string
	Content   string
	AvatarURL string
}

// sendCoverAndImageRecord 先发一张首图大图，再发一条合并聊天记录卡片
func (v *VideoParserPlugin) sendCoverAndImageRecord(receiver *contact.Contact, info *parser.VideoParseInfo) (bool, error) {
	authorName := info.Author.Name
	if authorName == "" {
		authorName = "作者"
	}
	title := info.Title
	if title == "" {
		title = fmt.Sprintf("%s 的图文作品", authorName)
	}

	// 1. 发送第 1 张高清封面大图
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

	// 2. 构造合并转发聊天记录（每条包含原图直链）
	var records []recordItem

	// 记录首条：标题和作者
	records = append(records, recordItem{
		Name:      authorName,
		Content:   fmt.Sprintf("📖 %s", title),
		AvatarURL: info.Author.Avatar,
	})

	// 记录后续：每张高清原图直链
	for i, img := range info.Images {
		text := fmt.Sprintf("[图片 %d] %s", i+1, img.Url)
		if img.LivePhotoUrl != "" {
			text += fmt.Sprintf("\n[实况动图] %s", img.LivePhotoUrl)
		}
		records = append(records, recordItem{
			Name:      authorName,
			Content:   text,
			AvatarURL: info.Author.Avatar,
		})
	}

	desc := fmt.Sprintf("%s: 共 %d 张图片", authorName, len(info.Images))
	xmlContent := buildChatRecordXML(title, desc, records)

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
		slog.Warn("发送图文合并转发失败，触发降级文本发送", "err", err)
		return v.sendFallbackImageText(receiver, info)
	}

	return true, nil
}

func buildChatRecordXML(title, desc string, records []recordItem) string {
	return fmt.Sprintf(`<appmsg appid="" sdkver="0">`+
		`<title>%s</title>`+
		`<des>%s</des>`+
		`<action>view</action>`+
		`<type>19</type>`+
		`<url>https://support.weixin.qq.com/cgi-bin/mmsupport-bin/readtemplate?t=page/favorite_record__w_unsupport&amp;from=singlemessage&amp;isappinstalled=0</url>`+
		`<recorditem>%s</recorditem>`+
		`</appmsg>`, escapeXML(title), escapeXML(desc), buildRecordItemXML(title, desc, records))
}

func buildRecordItemXML(title, desc string, records []recordItem) string {
	var builder strings.Builder
	builder.WriteString("<![CDATA[<recordinfo>\n")
	builder.WriteString(fmt.Sprintf("<title>%s</title>\n", escapeXML(title)))
	builder.WriteString(fmt.Sprintf("<desc>%s</desc>\n", escapeXML(desc)))
	builder.WriteString(fmt.Sprintf("<datalist count=\"%d\">\n", len(records)))

	base := time.Now()
	for i, record := range records {
		t := base.Add(time.Duration(i) * time.Second)
		timeStr := t.Format("2006-01-02 15:04:05")

		builder.WriteString(fmt.Sprintf(
			"<dataitem datatype=\"1\" dataid=\"d_%d_%d\" htmlid=\"\">\n"+
				"\t<datadesc>%s</datadesc>\n"+
				"\t<sourcename>%s</sourcename>\n"+
				"\t<sourceheadurl>%s</sourceheadurl>\n"+
				"\t<sourcetime>%s</sourcetime>\n"+
				"\t<srcMsgCreateTime>%d</srcMsgCreateTime>\n"+
				"\t<fromnewmsgid>%d</fromnewmsgid>\n"+
				"</dataitem>\n",
			t.Unix(),
			i,
			escapeXML(record.Content),
			escapeXML(record.Name),
			escapeXML(record.AvatarURL),
			escapeXML(timeStr),
			t.Unix(),
			t.UnixNano(),
		))
	}

	builder.WriteString("</datalist></recordinfo>]]>")
	return builder.String()
}

func escapeXML(value string) string {
	var buffer bytes.Buffer
	if err := xml.EscapeText(&buffer, []byte(value)); err != nil {
		return value
	}
	return buffer.String()
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
