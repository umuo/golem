package main

import (
	"testing"

	"github.com/sbgayhub/golem/sdk/chatroom"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
)

func TestBuildPrivateMessageEventUsesStringIDs(t *testing.T) {
	msg := &message.Message{
		Id:        123,
		Timestamp: 456,
		Sender: &contact.Contact{
			Username: "wxid_friend", Nickname: "Friend", Type: contact.ContactType_CONTACT_TYPE_FRIEND,
		},
		Type:    message.TypeText,
		Content: "hello",
		Data: &message.Message_Text{Text: &message.TextData{
			Content: "hello", Reminds: []string{"wxid_bot"},
		}},
	}

	event, ok := buildMessageEvent(msg, "wxid_bot")
	if !ok {
		t.Fatal("expected message event")
	}
	if event.MessageType != "private" || event.UserID != "wxid_friend" || event.MessageID != "123" {
		t.Fatalf("unexpected event identity: %#v", event)
	}
	if len(event.Message) != 2 || event.Message[0].Type != "at" || event.Message[1].Type != "text" {
		t.Fatalf("unexpected segments: %#v", event.Message)
	}
}

func TestBuildGroupMessageEventUsesMemberAsUser(t *testing.T) {
	msg := &message.Message{
		Id: 789,
		Sender: &contact.Contact{
			Username: "room@chatroom", Nickname: "Room", Type: contact.ContactType_CONTACT_TYPE_CHATROOM,
		},
		Member:  &chatroom.Member{Username: "wxid_member", Nickname: "Member", DisplayName: "Card"},
		Type:    message.TypeImage,
		Content: "image",
		Data: &message.Message_Image{Image: &message.ImageData{
			Media: &message.Media{Url: "https://example.test/image.jpg", Size: 12},
		}},
	}

	event, ok := buildMessageEvent(msg, "wxid_bot")
	if !ok {
		t.Fatal("expected message event")
	}
	if event.MessageType != "group" || event.GroupID != "room@chatroom" || event.UserID != "wxid_member" {
		t.Fatalf("unexpected group identity: %#v", event)
	}
	if event.Sender.Card != "Card" || len(event.Message) != 1 || event.Message[0].Type != "image" {
		t.Fatalf("unexpected sender or segments: %#v", event)
	}
	if got := valueString(event.Message[0].Data["file"]); got != "https://example.test/image.jpg" {
		t.Fatalf("unexpected image file: %q", got)
	}
}

func TestTriggeredMessageStripsPrefixWithoutMutatingOriginal(t *testing.T) {
	msg := &message.Message{
		Content: " /qq   hello ",
		Type:    message.TypeText,
		Data: &message.Message_Text{Text: &message.TextData{
			Content: " /qq   hello ", Reminds: []string{"wxid_bot"},
		}},
	}

	triggered, ok := triggeredMessage(msg, "/qq")
	if !ok {
		t.Fatal("expected message to trigger")
	}
	if got := triggered.GetText().GetContent(); got != "hello" {
		t.Fatalf("triggered content = %q, want hello", got)
	}
	if got := msg.GetText().GetContent(); got != " /qq   hello " {
		t.Fatalf("original message was mutated: %q", got)
	}
	if got := triggered.GetText().GetReminds(); len(got) != 1 || got[0] != "wxid_bot" {
		t.Fatalf("reminds = %#v", got)
	}
}

func TestTriggeredMessageRequiresPrefixBoundary(t *testing.T) {
	for _, content := range []string{"hello /qq", "/qqbot hello", "hello"} {
		msg := &message.Message{
			Type: message.TypeText,
			Data: &message.Message_Text{Text: &message.TextData{Content: content}},
		}
		if _, ok := triggeredMessage(msg, "/qq"); ok {
			t.Fatalf("content %q unexpectedly triggered", content)
		}
	}
}

func TestTriggeredMessageAllowsTriggerOnly(t *testing.T) {
	msg := &message.Message{
		Type: message.TypeText,
		Data: &message.Message_Text{Text: &message.TextData{Content: "/qq"}},
	}
	triggered, ok := triggeredMessage(msg, "/qq")
	if !ok || triggered.GetText().GetContent() != "" {
		t.Fatalf("triggered = %#v, ok = %v", triggered, ok)
	}
}
