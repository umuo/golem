package main

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
)

type actionRequest struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
	Echo   json.RawMessage `json:"echo,omitempty"`
}

type actionResponse struct {
	Status  string          `json:"status"`
	RetCode int             `json:"retcode"`
	Data    any             `json:"data"`
	Message string          `json:"message,omitempty"`
	Wording string          `json:"wording,omitempty"`
	Echo    json.RawMessage `json:"echo,omitempty"`
}

type oneBotSegment struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

type senderInfo struct {
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname,omitempty"`
	Card     string `json:"card,omitempty"`
	Role     string `json:"role,omitempty"`
}

type messageEvent struct {
	Time        int64           `json:"time"`
	SelfID      string          `json:"self_id"`
	PostType    string          `json:"post_type"`
	MessageType string          `json:"message_type"`
	SubType     string          `json:"sub_type"`
	MessageID   string          `json:"message_id"`
	UserID      string          `json:"user_id"`
	GroupID     string          `json:"group_id,omitempty"`
	Message     []oneBotSegment `json:"message"`
	RawMessage  string          `json:"raw_message"`
	Font        int             `json:"font"`
	Sender      senderInfo      `json:"sender"`
}

type lifecycleEvent struct {
	Time          int64  `json:"time"`
	SelfID        string `json:"self_id"`
	PostType      string `json:"post_type"`
	MetaEventType string `json:"meta_event_type"`
	SubType       string `json:"sub_type"`
}

type heartbeatStatus struct {
	Online bool `json:"online"`
	Good   bool `json:"good"`
}

type heartbeatEvent struct {
	Time          int64           `json:"time"`
	SelfID        string          `json:"self_id"`
	PostType      string          `json:"post_type"`
	MetaEventType string          `json:"meta_event_type"`
	Status        heartbeatStatus `json:"status"`
	Interval      int64           `json:"interval"`
}

func buildMessageEvent(msg *message.Message, selfID string) (messageEvent, bool) {
	if msg == nil || msg.GetSender() == nil || strings.TrimSpace(msg.GetSender().GetUsername()) == "" {
		return messageEvent{}, false
	}

	sender := msg.GetSender()
	isGroup := sender.GetType() == contact.ContactType_CONTACT_TYPE_CHATROOM || strings.HasSuffix(sender.GetUsername(), "@chatroom")
	userID := sender.GetUsername()
	nickname := displayContact(sender)
	card := ""
	groupID := ""
	messageType := "private"
	subType := "friend"
	role := ""
	if isGroup {
		messageType = "group"
		subType = "normal"
		groupID = sender.GetUsername()
		if member := msg.GetMember(); member != nil && strings.TrimSpace(member.GetUsername()) != "" {
			userID = member.GetUsername()
			nickname = strings.TrimSpace(member.GetNickname())
			card = strings.TrimSpace(member.GetDisplayName())
			if nickname == "" {
				nickname = firstNonBlank(card, userID)
			}
		}
	}

	timestamp := int64(msg.GetTimestamp())
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	return messageEvent{
		Time:        timestamp,
		SelfID:      selfID,
		PostType:    "message",
		MessageType: messageType,
		SubType:     subType,
		MessageID:   strconv.FormatInt(msg.GetId(), 10),
		UserID:      userID,
		GroupID:     groupID,
		Message:     toOneBotSegments(msg),
		RawMessage:  msg.GetContent(),
		Font:        0,
		Sender: senderInfo{
			UserID: userID, Nickname: nickname, Card: card, Role: role,
		},
	}, true
}

func toOneBotSegments(msg *message.Message) []oneBotSegment {
	if msg == nil {
		return nil
	}
	if text := msg.GetText(); text != nil {
		segments := make([]oneBotSegment, 0, len(text.GetReminds())+1)
		for _, target := range text.GetReminds() {
			target = strings.TrimSpace(target)
			if target != "" {
				segments = append(segments, oneBotSegment{Type: "at", Data: map[string]any{"qq": target}})
			}
		}
		content := firstNonBlank(text.GetContent(), msg.GetContent())
		if content != "" {
			segments = append(segments, oneBotSegment{Type: "text", Data: map[string]any{"text": content}})
		}
		return segments
	}
	if image := msg.GetImage(); image != nil {
		return []oneBotSegment{mediaSegment("image", image.GetMedia())}
	}
	if voice := msg.GetVoice(); voice != nil {
		segment := mediaSegment("record", voice.GetMedia())
		segment.Data["duration"] = voice.GetDuration()
		return []oneBotSegment{segment}
	}
	if video := msg.GetVideo(); video != nil {
		segment := mediaSegment("video", video.GetMedia())
		segment.Data["duration"] = video.GetDuration()
		return []oneBotSegment{segment}
	}
	if emoji := msg.GetEmoji(); emoji != nil {
		return []oneBotSegment{mediaSegment("image", emoji.GetMedia())}
	}
	if location := msg.GetLocation(); location != nil {
		return []oneBotSegment{{Type: "location", Data: map[string]any{
			"lat": location.GetLatitude(), "lon": location.GetLongitude(),
			"title": location.GetPoiName(), "content": location.GetLabel(),
		}}}
	}
	if app := msg.GetApp(); app != nil {
		content := strings.TrimSpace(strings.Join(nonEmpty(app.GetTitle(), app.GetDesc(), app.GetUrl()), "\n"))
		if content != "" {
			return []oneBotSegment{{Type: "text", Data: map[string]any{"text": content}}}
		}
	}
	if content := msg.GetContent(); content != "" {
		return []oneBotSegment{{Type: "text", Data: map[string]any{"text": content}}}
	}
	return nil
}

func mediaSegment(typ string, media *message.Media) oneBotSegment {
	data := map[string]any{}
	if media != nil {
		file := strings.TrimSpace(media.GetUrl())
		if file == "" && len(media.GetData()) > 0 {
			file = "base64://" + base64.StdEncoding.EncodeToString(media.GetData())
		}
		data["file"] = file
		data["url"] = media.GetUrl()
		data["file_size"] = media.GetSize()
		data["md5"] = media.GetMd5()
	}
	return oneBotSegment{Type: typ, Data: data}
}

func displayContact(value *contact.Contact) string {
	if value == nil {
		return ""
	}
	return firstNonBlank(value.GetRemark(), value.GetNickname(), value.GetAlias(), value.GetUsername())
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	return result
}
