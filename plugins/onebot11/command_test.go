package main

import (
	"testing"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/plugin"
)

func TestQQCommandIsRegistered(t *testing.T) {
	p := newPlugin()
	registry := plugin.NewCommandRegistry()
	if err := plugin.RegisterCommandTo(registry, p.handleQQCommand); err != nil {
		t.Fatalf("register command: %v", err)
	}
	commands := registry.Commands()
	if len(commands) != 1 || commands[0] != "qq" {
		t.Fatalf("commands = %#v, want [qq]", commands)
	}
	schemas := registry.Schemas()
	if len(schemas) != 1 || schemas[0].GetUsage() != "/qq <内容>" {
		t.Fatalf("schemas = %#v", schemas)
	}
}

func TestQQCommandRequiresRunningGateway(t *testing.T) {
	p := newPlugin()
	_, err := p.handleQQCommand(qqCommand{
		Content: "hello",
		Command: &plugin.Command{Sender: &contact.Contact{Username: "wxid_friend"}},
	})
	if err == nil || err.Error() != "OneBot WebSocket 服务未启动" {
		t.Fatalf("error = %v", err)
	}
}

func TestQQGroupCommandMentionsSelfForDownstreamTrigger(t *testing.T) {
	msg := qqCommandMessage(qqCommand{
		Content: "hello",
		Command: &plugin.Command{Sender: &contact.Contact{
			Username: "room@chatroom",
			Type:     contact.ContactType_CONTACT_TYPE_CHATROOM,
		}},
	}, "wxid_bot")

	event, ok := buildMessageEvent(msg, "wxid_bot")
	if !ok {
		t.Fatal("expected group event")
	}
	if event.MessageType != "group" || event.GroupID != "room@chatroom" {
		t.Fatalf("unexpected group event: %#v", event)
	}
	if len(event.Message) != 2 || event.Message[0].Type != "at" || event.Message[1].Type != "text" {
		t.Fatalf("segments = %#v", event.Message)
	}
	if got := valueString(event.Message[0].Data["qq"]); got != "wxid_bot" {
		t.Fatalf("mention target = %q, want wxid_bot", got)
	}
}
