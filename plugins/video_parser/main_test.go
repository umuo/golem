package main

import (
	"fmt"
	"io"
	"testing"

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
		fmt.Printf("App.Title: %s\n", appData.App.Title)
		fmt.Printf("App.Desc : %s\n", appData.App.Desc)
		fmt.Printf("App.Url  : %s\n", appData.App.Url)
		fmt.Printf("App.Xml  : %s\n", appData.App.Xml)
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

func TestRedBookParse(t *testing.T) {
	rawText := `人要有一颗独立思考的脑子！！！ https://xhslink.cn/o/3XqcY90fWih 来【小红书】逛逛这篇笔记吧~`

	info, err := parser.ParseVideoShareUrlByRegexp(rawText)
	if err != nil {
		t.Fatalf("小红书解析失败: %v", err)
	}

	fmt.Printf("\n=== [小红书笔记解析结果] ===\n")
	fmt.Printf("标题: %s\n", info.Title)
	fmt.Printf("作者: %s (ID: %s)\n", info.Author.Name, info.Author.Uid)
	fmt.Printf("视频直链: %s\n", info.VideoUrl)
	fmt.Printf("封面图: %s\n", info.CoverUrl)
}

func TestDouyinParse(t *testing.T) {
	rawText := `0.53 ULJ:/ 01/13 m@q.re :5pm 复制打开抖音极速版，看看【K侦探的作品】21岁空姐深夜发微信给同事说"司机是个变态，说想亲... https://v.douyin.com/jz7V8Tf_-rc/`
	info, err := parser.ParseVideoShareUrlByRegexp(rawText)
	if err != nil {
		t.Fatalf("抖音解析失败: %v", err)
	}
	fmt.Printf("\n=== [抖音视频解析结果] ===\n")
	fmt.Printf("标题: %s\n", info.Title)
	fmt.Printf("作者: %s\n", info.Author.Name)
	fmt.Printf("视频直链: %s\n", info.VideoUrl)
	fmt.Printf("封面图: %s\n", info.CoverUrl)
}

func TestVideoParserPlugin_OnEvent(t *testing.T) {
	rawText := `人要有一颗独立思考的脑子！！！ https://xhslink.cn/o/3XqcY90fWih 来【小红书】逛逛这篇笔记吧~`

	mockMsg := &mockMessageAbility{}
	p := &VideoParserPlugin{
		message: mockMsg,
	}

	event := &plugin.Event{
		Payload: &plugin.Event_Message{
			Message: &message.Message{
				Sender: &contact.Contact{
					Username: "wxid_test_user",
					Nickname: "测试用户",
				},
				Type:    message.TypeText,
				Content: rawText,
			},
		},
	}

	handled, err := p.OnEvent(event)
	if err != nil {
		t.Fatalf("OnEvent 处理失败: %v", err)
	}

	if !handled {
		t.Fatalf("OnEvent 返回 false，消息未被成功处理")
	}

	if len(mockMsg.sentMessages) == 0 {
		t.Fatalf("未发送任何消息")
	}

	t.Log("OnEvent 完整插件流程测试通过！")
}
