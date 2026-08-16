package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wujunwei928/parse-video/parser"
)

type recordItem struct {
	Kind      string // text | image
	Name      string
	Content   string
	Avatar    string
	Time      string
	DataURL   string
	DataKey   string
	FullMD5   string
	DataSize  uint32
	DataFmt   string
	ThumbURL  string
	ThumbKey  string
	ThumbMD5  string
	ThumbSize uint32
}

const (
	maxImagesPerRecord = 9
	downloadTimeout    = 8 * time.Second
)

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

func md5Hex(data []byte) string {
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}

func (v *VideoParserPlugin) buildImageRecordItems(info *parser.VideoParseInfo) []recordItem {
	var items []recordItem

	authorName := info.Author.Name
	if authorName == "" {
		authorName = "作者"
	}
	avatar := info.Author.Avatar

	// 1. 首条放入标题和简介
	descText := info.Title
	if len(info.Images) > maxImagesPerRecord {
		descText = fmt.Sprintf("%s\n(图集共 %d 张，展示前 %d 张)", info.Title, len(info.Images), maxImagesPerRecord)
	}
	items = append(items, recordItem{
		Kind:    "text",
		Name:    authorName,
		Content: descText,
		Avatar:  avatar,
	})

	imagesToFetch := info.Images
	if len(imagesToFetch) > maxImagesPerRecord {
		imagesToFetch = imagesToFetch[:maxImagesPerRecord]
	}

	type downloadResult struct {
		index int
		data  []byte
		err   error
	}

	results := make([]downloadResult, len(imagesToFetch))
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
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

	for i, res := range results {
		imgUrl := imagesToFetch[i].Url
		if res.err != nil || len(res.data) == 0 {
			slog.Warn("下载图片失败，降级为直链", "idx", i, "url", imgUrl, "err", res.err)
			items = append(items, recordItem{
				Kind:    "text",
				Name:    authorName,
				Content: fmt.Sprintf("[图片 %d] %s", i+1, imgUrl),
				Avatar:  avatar,
			})
			continue
		}

		if v.cdn != nil {
			uploadResp, err := v.cdn.UploadImage("filehelper", bytes.NewReader(res.data))
			if err == nil && uploadResp != nil && uploadResp.GetFileId() != "" && uploadResp.GetAesKey() != "" {
				fileId := uploadResp.GetFileId()
				aesKey := uploadResp.GetAesKey()
				fileMd5 := uploadResp.GetFileMd5()
				if fileMd5 == "" {
					fileMd5 = md5Hex(res.data)
				}
				fileSize := uploadResp.GetFileSize()
				if fileSize == 0 {
					fileSize = uint32(len(res.data))
				}

				thumbUrl := uploadResp.GetFileId()
				thumbKey := uploadResp.GetAesKey()
				thumbMd5 := uploadResp.GetThumbMd5()
				if thumbMd5 == "" {
					thumbMd5 = fileMd5
				}
				thumbSize := uploadResp.GetThumbSize()
				if thumbSize == 0 {
					thumbSize = fileSize
				}

				items = append(items, recordItem{
					Kind:      "image",
					Name:      authorName,
					Content:   "[图片]",
					Avatar:    avatar,
					DataURL:   fileId,
					DataKey:   aesKey,
					FullMD5:   fileMd5,
					DataSize:  fileSize,
					DataFmt:   "jpg",
					ThumbURL:  thumbUrl,
					ThumbKey:  thumbKey,
					ThumbMD5:  thumbMd5,
					ThumbSize: thumbSize,
				})
				continue
			} else {
				slog.Warn("上传图片到微信 CDN 失败，降级为直链", "idx", i, "err", err)
			}
		}

		// 降级为文本链接
		items = append(items, recordItem{
			Kind:    "text",
			Name:    authorName,
			Content: fmt.Sprintf("[图片 %d] %s", i+1, imgUrl),
			Avatar:  avatar,
		})
	}

	return items
}

func buildChatRecordXML(title, desc string, items []recordItem, defaultAvatar string) string {
	title = strings.TrimSpace(title)
	desc = strings.TrimSpace(desc)
	if title == "" {
		title = "图文记录"
	}
	if desc == "" {
		desc = fmt.Sprintf("共%d条消息", len(items))
	}

	var inner strings.Builder
	inner.WriteString("<![CDATA[<recordinfo>\n")
	inner.WriteString(fmt.Sprintf("<title>%s</title>\n", escapeXML(title)))
	inner.WriteString(fmt.Sprintf("<desc>%s</desc>\n", escapeXML(desc)))
	inner.WriteString(fmt.Sprintf("<datalist count=\"%d\">\n", len(items)))

	now := time.Now()
	for i, item := range items {
		createAt := now.Add(time.Duration(i) * time.Second)
		inner.WriteString(buildRecordDataItem(item, defaultAvatar, createAt, i))
		inner.WriteString("\n")
	}
	inner.WriteString("</datalist></recordinfo>]]>")

	return fmt.Sprintf(
		`<appmsg appid="" sdkver="0">`+
			`<title>%s</title>`+
			`<des>%s</des>`+
			`<action>view</action>`+
			`<type>19</type>`+
			`<url>https://support.weixin.qq.com/cgi-bin/mmsupport-bin/readtemplate?t=page/favorite_record__w_unsupport&amp;from=singlemessage&amp;isappinstalled=0</url>`+
			`<recorditem>%s</recorditem>`+
			`</appmsg>`,
		escapeXML(title),
		escapeXML(desc),
		inner.String(),
	)
}

func buildRecordDataItem(item recordItem, defaultAvatar string, createAt time.Time, idx int) string {
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = "消息"
	}
	avatar := strings.TrimSpace(item.Avatar)
	if avatar == "" {
		avatar = defaultAvatar
	}
	timeStr := strings.TrimSpace(item.Time)
	if timeStr == "" {
		timeStr = createAt.Format("2006-01-02 15:04:05")
	}
	dataID := fmt.Sprintf("d_%d_%d", createAt.Unix(), idx)
	kind := item.Kind
	if kind == "" {
		kind = "text"
	}

	var b strings.Builder
	if kind == "image" {
		desc := strings.TrimSpace(item.Content)
		if desc == "" {
			desc = "[图片]"
		}
		fmtStr := item.DataFmt
		if fmtStr == "" {
			fmtStr = "jpg"
		}
		thumbURL := strings.TrimSpace(item.ThumbURL)
		if thumbURL == "" {
			thumbURL = strings.TrimSpace(item.DataURL)
		}
		thumbKey := strings.TrimSpace(item.ThumbKey)
		if thumbKey == "" {
			thumbKey = strings.TrimSpace(item.DataKey)
		}
		thumbMD5 := strings.TrimSpace(item.ThumbMD5)
		if thumbMD5 == "" {
			thumbMD5 = strings.TrimSpace(item.FullMD5)
		}
		thumbSize := item.ThumbSize
		if thumbSize == 0 {
			thumbSize = item.DataSize
		}
		b.WriteString(fmt.Sprintf(
			`<dataitem datatype="2" dataid="%s">`+
				`<datadesc>%s</datadesc>`+
				`<cdnthumburl>%s</cdnthumburl>`+
				`<cdnthumbkey>%s</cdnthumbkey>`+
				`<thumbfullmd5>%s</thumbfullmd5>`+
				`<thumbsize>%d</thumbsize>`+
				`<cdndataurl>%s</cdndataurl>`+
				`<cdndatakey>%s</cdndatakey>`+
				`<fullmd5>%s</fullmd5>`+
				`<datasize>%d</datasize>`+
				`<datafmt>%s</datafmt>`+
				`<sourcename>%s</sourcename>`+
				`<sourceheadurl>%s</sourceheadurl>`+
				`<sourcetime>%s</sourcetime>`+
				`<srcMsgCreateTime>%d</srcMsgCreateTime>`+
				`<fromnewmsgid>%d</fromnewmsgid>`+
				`<thumbfiletype>1</thumbfiletype>`+
				`<filetype>1</filetype>`+
				`</dataitem>`,
			escapeXML(dataID),
			escapeXML(desc),
			escapeXML(thumbURL),
			escapeXML(thumbKey),
			escapeXML(thumbMD5),
			thumbSize,
			escapeXML(strings.TrimSpace(item.DataURL)),
			escapeXML(strings.TrimSpace(item.DataKey)),
			escapeXML(strings.TrimSpace(item.FullMD5)),
			item.DataSize,
			escapeXML(fmtStr),
			escapeXML(name),
			escapeXML(avatar),
			escapeXML(timeStr),
			createAt.Unix(),
			createAt.UnixNano(),
		))
		return b.String()
	}

	// 文本
	content := strings.TrimSpace(item.Content)
	b.WriteString(fmt.Sprintf(
		`<dataitem datatype="1" dataid="%s" htmlid="">`+
			`<datadesc>%s</datadesc>`+
			`<sourcename>%s</sourcename>`+
			`<sourceheadurl>%s</sourceheadurl>`+
			`<sourcetime>%s</sourcetime>`+
			`<srcMsgCreateTime>%d</srcMsgCreateTime>`+
			`<fromnewmsgid>%d</fromnewmsgid>`+
			`</dataitem>`,
		escapeXML(dataID),
		escapeXML(content),
		escapeXML(name),
		escapeXML(avatar),
		escapeXML(timeStr),
		createAt.Unix(),
		createAt.UnixNano(),
	))
	return b.String()
}

func escapeXML(value string) string {
	var buffer bytes.Buffer
	if err := xml.EscapeText(&buffer, []byte(value)); err != nil {
		return value
	}
	return buffer.String()
}
