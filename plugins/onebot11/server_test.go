package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
)

func TestWebSocketAuthenticationActionAndEvent(t *testing.T) {
	config, err := normalizeConfig(defaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	config.Listen = "127.0.0.1:0"
	config.Path = "/onebot/v11/ws"
	config.AccessToken = "test-token"
	config.HeartbeatInterval = 0
	messages := &fakeMessageAbility{}
	g := newGateway(config, messages, fakeIdentityAbility{self: &contact.SelfInfo{
		Username: "wxid_bot", Nickname: "Bot",
	}})
	if err := g.start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = g.stop(ctx)
	})

	address := "ws://" + g.listener.Addr().String() + config.Path
	_, response, err := websocket.DefaultDialer.Dial(address, nil)
	if err == nil {
		t.Fatal("expected unauthorized handshake to fail")
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 response, got %#v", response)
	}
	_ = response.Body.Close()

	header := http.Header{"Authorization": []string{"Bearer test-token"}}
	conn, _, err := websocket.DefaultDialer.Dial(address, header)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var lifecycle lifecycleEvent
	readJSON(t, conn, &lifecycle)
	if lifecycle.PostType != "meta_event" || lifecycle.SubType != "connect" || lifecycle.SelfID != "wxid_bot" {
		t.Fatalf("unexpected lifecycle event: %#v", lifecycle)
	}

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{
		"action":"get_status","params":{},"echo":"status-1"
	}`)); err != nil {
		t.Fatal(err)
	}
	var responseFrame actionResponse
	readJSON(t, conn, &responseFrame)
	if responseFrame.Status != "ok" || string(responseFrame.Echo) != `"status-1"` {
		t.Fatalf("unexpected action response: %#v", responseFrame)
	}

	err = g.publishMessage(&message.Message{
		Id: 42,
		Sender: &contact.Contact{
			Username: "wxid_friend", Nickname: "Friend", Type: contact.ContactType_CONTACT_TYPE_FRIEND,
		},
		Type:    message.TypeText,
		Content: "hello",
		Data:    &message.Message_Text{Text: &message.TextData{Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var event messageEvent
	readJSON(t, conn, &event)
	if event.PostType != "message" || event.UserID != "wxid_friend" || event.MessageID != "42" {
		t.Fatalf("unexpected message event: %#v", event)
	}
}

func readJSON(t *testing.T, conn *websocket.Conn, target any) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode websocket frame: %v: %s", err, data)
	}
}
