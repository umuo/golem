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
