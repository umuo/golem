package main

import (
	"errors"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

type qqCommand struct {
	_       struct{} `cmd:"qq" help:"将文本转发给 OneBot 11" usage:"/qq <内容>" example:"/qq 看见你拉屎"`
	Content string   `arg:"content" help:"要转发的文本" required:"true" variadic:"true"`
	Command *plugin.Command
}

func registerCommands(p *OneBotPlugin) error {
	return plugin.RegisterCommand(p.handleQQCommand)
}

func (p *OneBotPlugin) GetCommands() []string {
	return plugin.CommandCommands()
}

func (p *OneBotPlugin) GetCommandSchemas() []*plugin.CommandSchema {
	return plugin.CommandSchemas()
}

func (p *OneBotPlugin) OnCommand(command *plugin.Command) (string, error) {
	return plugin.DispatchCommand(command)
}

func (p *OneBotPlugin) handleQQCommand(command qqCommand) (string, error) {
	content := strings.TrimSpace(command.Content)
	if content == "" {
		return "", errors.New("转发内容不能为空")
	}
	if command.Command == nil || command.Command.GetSender() == nil {
		return "", errors.New("命令发送者为空")
	}

	p.mu.Lock()
	g := p.gateway
	p.mu.Unlock()
	if g == nil {
		return "", errors.New("OneBot WebSocket 服务未启动")
	}

	msg := qqCommandMessage(command, g.identityID())
	if err := g.publishMessage(msg); err != nil {
		return "", err
	}
	return "", nil
}

func qqCommandMessage(command qqCommand, selfID string) *message.Message {
	sender := command.Command.GetSender()
	reminds := []string(nil)
	if sender.GetType() == contact.ContactType_CONTACT_TYPE_CHATROOM || strings.HasSuffix(sender.GetUsername(), "@chatroom") {
		if selfID = strings.TrimSpace(selfID); selfID != "" {
			reminds = []string{selfID}
		}
	}
	content := strings.TrimSpace(command.Content)
	return &message.Message{
		Timestamp: uint32(time.Now().Unix()),
		Sender:    sender,
		Type:      message.TypeText,
		Content:   content,
		Data: &message.Message_Text{Text: &message.TextData{
			Content: content,
			Reminds: reminds,
		}},
	}
}
