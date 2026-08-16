package main

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

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
		fmt.Printf("App.Url    : %s\n", appData.App.Url)
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

func TestSendCoverAndImageRecord(t *testing.T) {
	mockInfo := &parser.VideoParseInfo{
		Title: "小红书高清多图穿搭分享",
		Images: []parser.ImgInfo{
			{Url: "https://sns-webpic-qc.xhscdn.com/202608161031/85bfd3ee1aedb563383061cc5880acbc/1040g008323qa2alp7k6g5psevsj3i17kg43b7l0!nd_dft_wlteh_jpg_3"},
		},
	}
	mockInfo.Author.Name = "时尚达人"
	mockInfo.Author.Avatar = "https://example.com/avatar.jpg"

	mockMsg := &mockMessageAbility{}
	p := &VideoParserPlugin{
		message:    mockMsg,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	handled, err := p.sendCoverAndImageRecord(&contact.Contact{Username: "test_chatroom@chatroom", Nickname: "测试群"}, mockInfo)
	if err != nil {
		t.Fatalf("sendCoverAndImageRecord 失败: %v", err)
	}
	if !handled {
		t.Fatalf("handled should be true")
	}

	if len(mockMsg.sentMessages) < 2 {
		t.Fatalf("期望发送 2 条消息 (1张封面图 + 1条合并转发卡片)，实际发送了 %d 条", len(mockMsg.sentMessages))
	}

	// 验证第 1 条是 TypeImage 封面大图
	firstMsg := mockMsg.sentMessages[0]
	if firstMsg.Type != message.TypeImage {
		t.Errorf("期望第 1 条为 TypeImage (封面大图), 实际为: %v", firstMsg.Type)
	}

	// 验证第 2 条是 TypeApplication (SubType 19 合并转发)
	secondMsg := mockMsg.sentMessages[1]
	if secondMsg.Type != message.TypeApplication {
		t.Errorf("期望第 2 条为 TypeApplication (合并转发), 实际为: %v", secondMsg.Type)
	}
	appData := secondMsg.Data.(*message.Message_App).App
	if appData.SubType != 19 {
		t.Errorf("期望 SubType 为 19, 实际为: %d", appData.SubType)
	}
	t.Logf("合并转发 XML:\n%s", appData.Xml)
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

func TestFallbackVideoText(t *testing.T) {
	mockMsg := &mockMessageAbility{}
	p := &VideoParserPlugin{
		message: mockMsg,
	}

	info := &parser.VideoParseInfo{
		Title:    "张震岳悉尼演唱会",
		VideoUrl: "https://v5-se-ws-cold.douyinvod.com/example/video.mp4",
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

func TestGetRedirectVideoURL(t *testing.T) {
	p := &VideoParserPlugin{}

	rawURL := "https://v5-dy-ov-experiment.zjcdn.com/9593c276a6589399acf6e8853d87ce0f/6a816707/video/tos/cn/tos-cn-ve-15c000-ce/oEe6hnd4DI1uk2n9A7Fgov9hhBfmqXEQE1wLAd/?a=6383"

	// 1. 无配置
	if got := p.getRedirectVideoURL(rawURL); got != rawURL {
		t.Errorf("期望无配置返回原始链接，实际: %s", got)
	}

	// 2. 配置了带等号的 redirect_url
	p.Config.RedirectURL = "https://next-url-redirector.pages.dev/go?url="
	expected := "https://next-url-redirector.pages.dev/go?url=https%3A%2F%2Fv5-dy-ov-experiment.zjcdn.com%2F9593c276a6589399acf6e8853d87ce0f%2F6a816707%2Fvideo%2Ftos%2Fcn%2Ftos-cn-ve-15c000-ce%2FoEe6hnd4DI1uk2n9A7Fgov9hhBfmqXEQE1wLAd%2F%3Fa%3D6383"
	if got := p.getRedirectVideoURL(rawURL); got != expected {
		t.Errorf("期望重定向格式:\n%s\n实际:\n%s", expected, got)
	}

	// 3. 配置了带模板的 redirect_url
	p.Config.RedirectURL = "https://example.com/play?v={url}"
	expectedTemplate := "https://example.com/play?v=https%3A%2F%2Fv5-dy-ov-experiment.zjcdn.com%2F9593c276a6589399acf6e8853d87ce0f%2F6a816707%2Fvideo%2Ftos%2Fcn%2Ftos-cn-ve-15c000-ce%2FoEe6hnd4DI1uk2n9A7Fgov9hhBfmqXEQE1wLAd%2F%3Fa%3D6383"
	if got := p.getRedirectVideoURL(rawURL); got != expectedTemplate {
		t.Errorf("期望模板替换格式:\n%s\n实际:\n%s", expectedTemplate, got)
	}
}
