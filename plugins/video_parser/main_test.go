package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/sbgayhub/golem/sdk/cdn"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
	"github.com/wujunwei928/parse-video/parser"
)

type mockMessageAbility struct {
	sentMessages []*message.Message
}

func (m *mockMessageAbility) Send(msg *message.Message) (*message.Send_Response, error) {
	m.sentMessages = append(m.sentMessages, msg)
	fmt.Printf("\n=== [Mock Send 接收到发送结果] ===\n")
	if msg.Receiver != nil {
		fmt.Printf("Receiver : %s (%s)\n", msg.Receiver.Username, msg.Receiver.Nickname)
	}
	fmt.Printf("Type     : %v\n", msg.Type)
	fmt.Printf("Content  : %s\n", msg.Content)
	if appData, ok := msg.Data.(*message.Message_App); ok && appData.App != nil {
		fmt.Printf("App.SubType: %d\n", appData.App.SubType)
		fmt.Printf("App.Title  : %s\n", appData.App.Title)
		fmt.Printf("App.Desc   : %s\n", appData.App.Desc)
		fmt.Printf("App.Xml    :\n%s\n", appData.App.Xml)
	}
	return &message.Send_Response{NewId: 1}, nil
}

func (m *mockMessageAbility) Forward(*message.Message, string) (*message.Forward_Response, error) {
	return &message.Forward_Response{}, nil
}

func (m *mockMessageAbility) Revoke(string, uint64) (*message.Revoke_Response, error) {
	return &message.Revoke_Response{}, nil
}

func (m *mockMessageAbility) Download(*message.Message) (io.ReadCloser, error) {
	return io.NopCloser(nil), nil
}

type mockCDNAbility struct{}

func (c *mockCDNAbility) UploadImage(receiver string, reader io.Reader) (*cdn.UploadImage_Response, error) {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(reader)
	data := buf.Bytes()
	fileId := fmt.Sprintf("mock_cdn_file_%d", time.Now().UnixNano())
	aesKey := "0123456789abcdef0123456789abcdef"
	fileMd5 := md5Hex(data)
	fileSize := uint32(len(data))

	return &cdn.UploadImage_Response{
		FileId:    &fileId,
		AesKey:    &aesKey,
		FileMd5:   &fileMd5,
		FileSize:  &fileSize,
		ThumbSize: &fileSize,
		ThumbMd5:  &fileMd5,
	}, nil
}

func (c *mockCDNAbility) UploadVideo(string, []byte, io.Reader, uint32) (*cdn.UploadVideo_Response, error) {
	return nil, nil
}
func (c *mockCDNAbility) DownloadImage(string, string) (io.ReadCloser, error) {
	return io.NopCloser(nil), nil
}
func (c *mockCDNAbility) DownloadVideo(string, string) (io.ReadCloser, error) {
	return io.NopCloser(nil), nil
}
func (c *mockCDNAbility) UploadMomentsImage([]byte) (*cdn.UploadMomentsImage_Response, error) {
	return nil, nil
}
func (c *mockCDNAbility) UploadMomentsVideo([]byte, []byte) (*cdn.UploadMomentsVideo_Response, error) {
	return nil, nil
}
func (c *mockCDNAbility) DownloadVideoCover(string, string) ([]byte, error) {
	return nil, nil
}
func (c *mockCDNAbility) DownloadMomentsVideo(string, uint64) ([]byte, error) {
	return nil, nil
}

func TestImageRecordBuilding(t *testing.T) {
	mockInfo := &parser.VideoParseInfo{
		Title: "小红书高清多图穿搭分享",
		Images: []parser.ImgInfo{
			{Url: "https://sns-webpic-qc.xhscdn.com/202608161031/85bfd3ee1aedb563383061cc5880acbc/1040g008323qa2alp7k6g5psevsj3i17kg43b7l0!nd_dft_wlteh_jpg_3"},
		},
	}
	mockInfo.Author.Name = "时尚达人"
	mockInfo.Author.Avatar = "https://example.com/avatar.jpg"

	mockMsg := &mockMessageAbility{}
	mockCDN := &mockCDNAbility{}
	p := &VideoParserPlugin{
		message:    mockMsg,
		cdn:        mockCDN,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	handled, err := p.sendImageRecord(&contact.Contact{Username: "test_chatroom@chatroom", Nickname: "测试群"}, mockInfo)
	if err != nil {
		t.Fatalf("sendImageRecord 失败: %v", err)
	}
	if !handled {
		t.Fatalf("handled should be true")
	}

	if len(mockMsg.sentMessages) == 0 {
		t.Fatalf("未发送消息")
	}

	sent := mockMsg.sentMessages[0]
	appData := sent.Data.(*message.Message_App).App
	if appData.SubType != 19 {
		t.Errorf("期望 SubType 19 (合并转发), 实际: %d", appData.SubType)
	}
}

func TestOnEventDouyinVideo(t *testing.T) {
	rawText := `6.41 11/28 a@A.GI aAt:/ :8pm 复制打开抖音极速版，看看【張震嶽的作品】谢谢昨晚的悉尼，很开心。 下一站墨尔本，明天晚上 ... https://v.douyin.com/hXn8sT_DJL4/`

	mockMsg := &mockMessageAbility{}
	p := &VideoParserPlugin{
		message:    mockMsg,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	event := &plugin.Event{
		Payload: &plugin.Event_Message{
			Message: &message.Message{
				Sender:  &contact.Contact{Username: "group@chatroom", Nickname: "群聊"},
				Type:    message.TypeText,
				Content: rawText,
			},
		},
	}

	handled, err := p.OnEvent(event)
	if err != nil {
		t.Fatalf("OnEvent 失败: %v", err)
	}
	if !handled {
		t.Fatalf("OnEvent 应返回 true")
	}
	t.Log("抖音视频流程测试成功")
}

func TestGetRedirectVideoURL(t *testing.T) {
	p := &VideoParserPlugin{}

	rawURL := "https://www.iesdouyin.com/aweme/v1/play/?video_id=v2800fgi0000d9r7uofog65ju1flrgp0&ratio=1080p&line=0"

	// 1. 无配置
	if got := p.getRedirectVideoURL(rawURL); got != rawURL {
		t.Errorf("期望无配置返回原始链接，实际: %s", got)
	}

	// 2. 配置了带等号的 redirect_url: https://next-url-redirector.pages.dev/go?url=
	p.Config.RedirectURL = "https://next-url-redirector.pages.dev/go?url="
	expected := "https://next-url-redirector.pages.dev/go?url=https%3A%2F%2Fwww.iesdouyin.com%2Faweme%2Fv1%2Fplay%2F%3Fvideo_id%3Dv2800fgi0000d9r7uofog65ju1flrgp0%26ratio%3D1080p%26line%3D0"
	if got := p.getRedirectVideoURL(rawURL); got != expected {
		t.Errorf("期望重定向格式:\n%s\n实际:\n%s", expected, got)
	}

	// 3. 配置了带模板的 redirect_url: https://example.com/play?target={url}&v=1
	p.Config.RedirectURL = "https://example.com/play?target={url}&v=1"
	expectedTemplate := "https://example.com/play?target=https%3A%2F%2Fwww.iesdouyin.com%2Faweme%2Fv1%2Fplay%2F%3Fvideo_id%3Dv2800fgi0000d9r7uofog65ju1flrgp0%26ratio%3D1080p%26line%3D0&v=1"
	if got := p.getRedirectVideoURL(rawURL); got != expectedTemplate {
		t.Errorf("期望模板替换格式:\n%s\n实际:\n%s", expectedTemplate, got)
	}
}

func TestFallbackVideoText(t *testing.T) {
	mockMsg := &mockMessageAbility{}
	p := &VideoParserPlugin{
		message: mockMsg,
	}

	info := &parser.VideoParseInfo{
		Title:    "张震岳悉尼演唱会",
		VideoUrl: "https://www.iesdouyin.com/aweme/v1/play/?video_id=v2800fgi0000d9r7uofog65ju1flrgp0&ratio=1080p&line=0",
	}
	info.Author.Name = "張震嶽"

	ok, err := p.sendFallbackVideoText(&contact.Contact{Username: "group@chatroom", Nickname: "测试群"}, info)
	if err != nil || !ok {
		t.Fatalf("sendFallbackVideoText 失败: %v", err)
	}

	if len(mockMsg.sentMessages) == 0 {
		t.Fatalf("未发送降级文本消息")
	}

	lastMsg := mockMsg.sentMessages[len(mockMsg.sentMessages)-1]
	if lastMsg.Type != message.TypeText {
		t.Errorf("期望降级消息类型为 TypeText, 实际: %v", lastMsg.Type)
	}
	t.Logf("降级文本内容:\n%s", lastMsg.Content)
}
