package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
)

const (
	retCodeBadRequest  = 1400
	retCodeNotFound    = 1404
	retCodeUnsupported = 1405
	retCodeInternal    = 1500
)

type actionFailure struct {
	retCode int
	message string
}

func (e *actionFailure) Error() string { return e.message }

type flexibleID string

func (id *flexibleID) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*id = ""
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err == nil {
		*id = flexibleID(strings.TrimSpace(value))
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		*id = flexibleID(number.String())
		return nil
	}
	return errors.New("ID 必须是字符串或数字")
}

type messagePayload []oneBotSegment

func (payload *messagePayload) UnmarshalJSON(data []byte) error {
	var segments []oneBotSegment
	if err := json.Unmarshal(data, &segments); err == nil {
		*payload = segments
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*payload = []oneBotSegment{{Type: "text", Data: map[string]any{"text": text}}}
		return nil
	}
	return errors.New("message 必须是 OneBot 消息段数组或字符串")
}

type sendActionParams struct {
	MessageType string         `json:"message_type"`
	UserID      flexibleID     `json:"user_id"`
	GroupID     flexibleID     `json:"group_id"`
	Message     messagePayload `json:"message"`
	AutoEscape  bool           `json:"auto_escape"`
}

type deleteActionParams struct {
	MessageID flexibleID `json:"message_id"`
}

func (g *gateway) handleActionFrame(frame []byte) []byte {
	var request actionRequest
	if err := json.Unmarshal(frame, &request); err != nil {
		return g.marshalResponse(actionResponse{
			Status: "failed", RetCode: retCodeBadRequest, Data: map[string]any{},
			Message: "无法解析 OneBot Action: " + err.Error(),
			Wording: "无法解析 OneBot Action: " + err.Error(),
		})
	}
	request.Action = strings.TrimSpace(request.Action)
	if request.Action == "" {
		return g.failureResponse(request.Echo, retCodeBadRequest, "action 不能为空")
	}

	data, err := g.executeAction(request)
	if err != nil {
		failure := &actionFailure{retCode: retCodeInternal, message: err.Error()}
		if errors.As(err, &failure) {
			return g.failureResponse(request.Echo, failure.retCode, failure.message)
		}
		return g.failureResponse(request.Echo, retCodeInternal, err.Error())
	}
	return g.marshalResponse(actionResponse{
		Status: "ok", RetCode: 0, Data: data, Echo: request.Echo,
	})
}

func (g *gateway) executeAction(request actionRequest) (any, error) {
	switch request.Action {
	case "send_private_msg":
		return g.handleSend(request.Params, "private")
	case "send_group_msg":
		return g.handleSend(request.Params, "group")
	case "send_msg":
		return g.handleSend(request.Params, "")
	case "delete_msg":
		return g.handleDelete(request.Params)
	case "get_login_info":
		selfID, nickname := g.identitySnapshot()
		return map[string]any{"user_id": selfID, "nickname": nickname}, nil
	case "get_status":
		return map[string]any{"online": true, "good": true}, nil
	case "get_version_info":
		return map[string]any{
			"app_name": "golem-onebot11", "app_version": pluginVersion, "protocol_version": "v11",
		}, nil
	default:
		return nil, &actionFailure{retCode: retCodeUnsupported, message: "不支持的 OneBot Action: " + request.Action}
	}
}

func (g *gateway) handleSend(raw json.RawMessage, forcedType string) (any, error) {
	var params sendActionParams
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &actionFailure{retCode: retCodeBadRequest, message: "send action 参数错误: " + err.Error()}
	}
	messageType := forcedType
	if messageType == "" {
		messageType = strings.TrimSpace(params.MessageType)
	}
	var receiver string
	switch messageType {
	case "private":
		receiver = string(params.UserID)
	case "group":
		receiver = string(params.GroupID)
	default:
		return nil, &actionFailure{retCode: retCodeBadRequest, message: "message_type 必须是 private 或 group"}
	}
	if strings.TrimSpace(receiver) == "" {
		return nil, &actionFailure{retCode: retCodeBadRequest, message: messageType + " ID 不能为空"}
	}
	if len(params.Message) == 0 {
		return nil, &actionFailure{retCode: retCodeBadRequest, message: "message 不能为空"}
	}

	messages, err := g.buildOutboundMessages(receiver, params.Message)
	if err != nil {
		return nil, err
	}
	lastMessageID := "0"
	for _, outbound := range messages {
		response, err := g.message.Send(outbound)
		if err != nil {
			return nil, fmt.Errorf("Golem 发送消息失败: %w", err)
		}
		if response == nil {
			continue
		}
		messageID := strconv.FormatUint(response.GetNewId(), 10)
		if response.GetNewId() != 0 {
			lastMessageID = messageID
			g.routes.put(messageID, receiver)
		}
	}
	return map[string]any{"message_id": lastMessageID}, nil
}

func (g *gateway) buildOutboundMessages(receiver string, segments []oneBotSegment) ([]*message.Message, error) {
	receiverContact := &contact.Contact{Username: receiver}
	result := make([]*message.Message, 0, len(segments))
	var text strings.Builder
	reminds := make([]string, 0)

	flushText := func() {
		content := text.String()
		if strings.TrimSpace(content) == "" && len(reminds) == 0 {
			return
		}
		result = append(result, &message.Message{
			Receiver: receiverContact,
			Type:     message.TypeText,
			Content:  content,
			Data: &message.Message_Text{Text: &message.TextData{
				Content: content, Reminds: append([]string(nil), reminds...),
			}},
		})
		text.Reset()
		reminds = reminds[:0]
	}

	for _, segment := range segments {
		switch strings.ToLower(strings.TrimSpace(segment.Type)) {
		case "text":
			text.WriteString(valueString(segment.Data["text"]))
		case "at":
			target := strings.TrimSpace(valueString(segment.Data["qq"]))
			if target == "" {
				continue
			}
			remindTarget := target
			display := firstNonBlank(valueString(segment.Data["name"]), target)
			if target == "all" {
				remindTarget = "notify@all"
				display = "所有人"
			}
			text.WriteString("@" + display + " ")
			reminds = append(reminds, remindTarget)
		case "image":
			flushText()
			file := firstNonBlank(valueString(segment.Data["file"]), valueString(segment.Data["url"]))
			media, err := g.loadMedia(file)
			if err != nil {
				return nil, &actionFailure{retCode: retCodeBadRequest, message: "读取图片失败: " + err.Error()}
			}
			result = append(result, &message.Message{
				Receiver: receiverContact,
				Type:     message.TypeImage,
				Data: &message.Message_Image{Image: &message.ImageData{
					Media: &message.Media{Data: media, Size: uint32(len(media))},
				}},
			})
		default:
			return nil, &actionFailure{
				retCode: retCodeUnsupported,
				message: "暂不支持发送消息段: " + segment.Type,
			}
		}
	}
	flushText()
	if len(result) == 0 {
		return nil, &actionFailure{retCode: retCodeBadRequest, message: "没有可发送的消息段"}
	}
	return result, nil
}

func (g *gateway) handleDelete(raw json.RawMessage) (any, error) {
	var params deleteActionParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &actionFailure{retCode: retCodeBadRequest, message: "delete_msg 参数错误: " + err.Error()}
	}
	messageID := strings.TrimSpace(string(params.MessageID))
	if messageID == "" {
		return nil, &actionFailure{retCode: retCodeBadRequest, message: "message_id 不能为空"}
	}
	receiver, ok := g.routes.get(messageID)
	if !ok {
		return nil, &actionFailure{
			retCode: retCodeNotFound,
			message: "找不到 message_id 对应的 Golem 会话；插件重启前必须见过或发送过该消息",
		}
	}
	numericID, err := strconv.ParseUint(messageID, 10, 64)
	if err != nil {
		return nil, &actionFailure{retCode: retCodeBadRequest, message: "Golem message_id 必须是无符号整数: " + err.Error()}
	}
	if _, err := g.message.Revoke(receiver, numericID); err != nil {
		return nil, fmt.Errorf("Golem 撤回消息失败: %w", err)
	}
	return map[string]any{}, nil
}

func (g *gateway) loadMedia(reference string) ([]byte, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, errors.New("file 不能为空")
	}
	if encoded, ok := strings.CutPrefix(reference, "base64://"); ok {
		return g.decodeMediaBase64(encoded)
	}
	if strings.HasPrefix(reference, "data:") {
		comma := strings.IndexByte(reference, ',')
		if comma < 0 || !strings.Contains(reference[:comma], ";base64") {
			return nil, errors.New("仅支持 base64 data URL")
		}
		return g.decodeMediaBase64(reference[comma+1:])
	}

	parsed, err := url.Parse(reference)
	if err != nil {
		return nil, err
	}
	switch parsed.Scheme {
	case "http", "https":
		return g.downloadMedia(parsed.String())
	case "file":
		if !g.config.AllowLocalFiles {
			return nil, errors.New("本地文件读取已禁用")
		}
		path, err := url.PathUnescape(parsed.Path)
		if err != nil {
			return nil, err
		}
		return readLimitedFile(path, g.config.MaxMediaBytes)
	case "":
		if !g.config.AllowLocalFiles {
			return nil, errors.New("只允许 http(s)、base64 或 data URL")
		}
		return readLimitedFile(reference, g.config.MaxMediaBytes)
	default:
		return nil, errors.New("不支持的媒体地址协议: " + parsed.Scheme)
	}
}

func (g *gateway) decodeMediaBase64(encoded string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > g.config.MaxMediaBytes {
		return nil, fmt.Errorf("媒体超过最大限制 %d 字节", g.config.MaxMediaBytes)
	}
	return data, nil
}

func (g *gateway) downloadMedia(address string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), g.config.RemoteMediaTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	response, err := g.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("远程媒体响应状态为 %d", response.StatusCode)
	}
	return readLimited(response.Body, g.config.MaxMediaBytes)
}

func readLimitedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return readLimited(file, limit)
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("媒体超过最大限制 %d 字节", limit)
	}
	return data, nil
}

func valueString(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

func (g *gateway) failureResponse(echo json.RawMessage, retCode int, message string) []byte {
	return g.marshalResponse(actionResponse{
		Status: "failed", RetCode: retCode, Data: map[string]any{},
		Message: message, Wording: message, Echo: echo,
	})
}

func (g *gateway) marshalResponse(response actionResponse) []byte {
	data, err := json.Marshal(response)
	if err != nil {
		return []byte(`{"status":"failed","retcode":1500,"data":{},"message":"response marshal failed"}`)
	}
	return data
}
