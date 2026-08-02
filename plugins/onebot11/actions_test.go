package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
)

type fakeMessageAbility struct {
	nextID         uint64
	sent           []*message.Message
	revokeReceiver string
	revokeID       uint64
}

func (f *fakeMessageAbility) Send(msg *message.Message) (*message.Send_Response, error) {
	f.sent = append(f.sent, msg)
	f.nextID++
	return &message.Send_Response{NewId: f.nextID}, nil
}

func (f *fakeMessageAbility) Revoke(receiver string, newMsgID uint64) (*message.Revoke_Response, error) {
	f.revokeReceiver = receiver
	f.revokeID = newMsgID
	return &message.Revoke_Response{}, nil
}

type fakeIdentityAbility struct {
	self *contact.SelfInfo
}

func (f fakeIdentityAbility) GetSelf() *contact.SelfInfo { return f.self }

func testGateway(t *testing.T, messages *fakeMessageAbility) *gateway {
	t.Helper()
	config, err := normalizeConfig(defaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	return newGateway(config, messages, fakeIdentityAbility{self: &contact.SelfInfo{
		Username: "wxid_bot", Nickname: "Bot",
	}})
}

func TestSendGroupMessageAndDeleteWithEcho(t *testing.T) {
	messages := &fakeMessageAbility{nextID: 100}
	g := testGateway(t, messages)
	request := []byte(`{
		"action":"send_group_msg",
		"params":{"group_id":"room@chatroom","message":[
			{"type":"at","data":{"qq":"wxid_member","name":"Member"}},
			{"type":"text","data":{"text":" hello"}}
		]},
		"echo":"request-1"
	}`)

	response := decodeActionResponse(t, g.handleActionFrame(request))
	if response.Status != "ok" || response.RetCode != 0 || string(response.Echo) != `"request-1"` {
		t.Fatalf("unexpected response: %#v", response)
	}
	data, ok := response.Data.(map[string]any)
	if !ok || data["message_id"] != "101" {
		t.Fatalf("unexpected response data: %#v", response.Data)
	}
	if len(messages.sent) != 1 {
		t.Fatalf("expected one Golem message, got %d", len(messages.sent))
	}
	sent := messages.sent[0]
	if sent.GetReceiver().GetUsername() != "room@chatroom" || sent.GetText().GetContent() != "@Member  hello" {
		t.Fatalf("unexpected outbound message: %#v", sent)
	}
	if got := sent.GetText().GetReminds(); len(got) != 1 || got[0] != "wxid_member" {
		t.Fatalf("unexpected reminds: %#v", got)
	}

	deleteResponse := decodeActionResponse(t, g.handleActionFrame([]byte(`{
		"action":"delete_msg","params":{"message_id":"101"},"echo":"request-2"
	}`)))
	if deleteResponse.Status != "ok" || messages.revokeReceiver != "room@chatroom" || messages.revokeID != 101 {
		t.Fatalf("unexpected delete result: response=%#v receiver=%q id=%d",
			deleteResponse, messages.revokeReceiver, messages.revokeID)
	}
}

func TestSendBase64Image(t *testing.T) {
	messages := &fakeMessageAbility{}
	g := testGateway(t, messages)
	response := decodeActionResponse(t, g.handleActionFrame([]byte(`{
		"action":"send_private_msg",
		"params":{"user_id":"wxid_friend","message":[
			{"type":"image","data":{"file":"base64://aGVsbG8="}}
		]},
		"echo":"image-1"
	}`)))
	if response.Status != "ok" || len(messages.sent) != 1 {
		t.Fatalf("unexpected image response: %#v, sent=%d", response, len(messages.sent))
	}
	if got := string(messages.sent[0].GetImage().GetMedia().GetData()); got != "hello" {
		t.Fatalf("unexpected image bytes: %q", got)
	}
}

func TestUnknownActionReturnsFailureWithEcho(t *testing.T) {
	g := testGateway(t, &fakeMessageAbility{})
	response := decodeActionResponse(t, g.handleActionFrame([]byte(`{
		"action":"unknown_action","params":{},"echo":"unknown-1"
	}`)))
	if response.Status != "failed" || response.RetCode != retCodeUnsupported || string(response.Echo) != `"unknown-1"` {
		t.Fatalf("unexpected failure response: %#v", response)
	}
}

func decodeActionResponse(t *testing.T, data []byte) actionResponse {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var response actionResponse
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode response: %v: %s", err, data)
	}
	return response
}
