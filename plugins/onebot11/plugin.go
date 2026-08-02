package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
	"google.golang.org/protobuf/proto"
)

func (p *OneBotPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "onebot11",
		Author:      "Golem Team",
		Version:     pluginVersion,
		Description: "通过 WebSocket 暴露 OneBot 11 兼容消息事件与 Action",
		Priority:    0,
		Next:        true,
		AlwaysRun:   false,
	}
}

func (p *OneBotPlugin) GetSubscriptions() []string {
	return []string{message.TypeText.Topic}
}

func (p *OneBotPlugin) OnEvent(event *plugin.Event) (bool, error) {
	p.mu.Lock()
	g := p.gateway
	p.mu.Unlock()
	if g == nil {
		return false, nil
	}

	payload, ok := event.GetPayload().(*plugin.Event_Message)
	if !ok || payload.Message == nil {
		return false, nil
	}
	triggered, ok := triggeredMessage(payload.Message, g.config.Trigger)
	if !ok {
		return false, nil
	}
	if err := g.publishMessage(triggered); err != nil {
		return false, err
	}
	return false, nil
}

func triggeredMessage(msg *message.Message, trigger string) (*message.Message, bool) {
	if msg == nil || msg.GetText() == nil {
		return nil, false
	}
	content := msg.GetText().GetContent()
	if strings.TrimSpace(content) == "" {
		content = msg.GetContent()
	}
	content = strings.TrimSpace(content)
	trigger = strings.TrimSpace(trigger)
	if content == "" || trigger == "" || !strings.HasPrefix(content, trigger) {
		return nil, false
	}

	remainder := strings.TrimPrefix(content, trigger)
	if remainder != "" {
		first, _ := utf8.DecodeRuneInString(remainder)
		if !unicode.IsSpace(first) {
			return nil, false
		}
	}
	remainder = strings.TrimSpace(remainder)

	cloned, ok := proto.Clone(msg).(*message.Message)
	if !ok || cloned.GetText() == nil {
		return nil, false
	}
	cloned.Content = remainder
	cloned.GetText().Content = remainder
	return cloned, true
}

func (p *OneBotPlugin) OnLoad() error {
	return p.startGateway()
}

func (p *OneBotPlugin) OnUnload() error {
	return p.stopGateway()
}

func (p *OneBotPlugin) OnEnable() error {
	return p.startGateway()
}

func (p *OneBotPlugin) OnDisable() error {
	return p.stopGateway()
}

func (p *OneBotPlugin) startGateway() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.gateway != nil {
		return nil
	}
	if p.message == nil {
		return errors.New("message ability is not injected")
	}
	if p.contact == nil {
		return errors.New("contact ability is not injected")
	}

	config, err := normalizeConfig(p.Config)
	if err != nil {
		return err
	}
	g := newGateway(config, p.message, p.contact)
	if err := g.start(); err != nil {
		return fmt.Errorf("启动 OneBot WebSocket 服务失败: %w", err)
	}
	p.gateway = g
	slog.Info("[onebot11] WebSocket 服务已启动", "listen", config.Listen, "path", config.Path, "trigger", config.Trigger)
	return nil
}

func (p *OneBotPlugin) stopGateway() error {
	p.mu.Lock()
	g := p.gateway
	p.gateway = nil
	p.mu.Unlock()
	if g == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := g.stop(ctx); err != nil {
		return fmt.Errorf("停止 OneBot WebSocket 服务失败: %w", err)
	}
	slog.Info("[onebot11] WebSocket 服务已停止")
	return nil
}
