package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
)

type gateway struct {
	config  runtimeConfig
	message oneBotMessageAbility
	contact identityAbility
	routes  *routeStore

	mu        sync.RWMutex
	clients   map[*wsClient]struct{}
	server    *http.Server
	listener  net.Listener
	serveDone chan struct{}
	cancel    context.CancelFunc

	identityMu sync.RWMutex
	selfID     string
	nickname   string

	httpClient *http.Client
	upgrader   websocket.Upgrader
}

type oneBotMessageAbility interface {
	Send(msg *message.Message) (*message.Send_Response, error)
	Revoke(receiver string, newMsgID uint64) (*message.Revoke_Response, error)
}

type identityAbility interface {
	GetSelf() *contact.SelfInfo
}

type wsClient struct {
	gateway *gateway
	conn    *websocket.Conn
	send    chan []byte
	done    chan struct{}
	once    sync.Once
}

func newGateway(config runtimeConfig, messageAbility oneBotMessageAbility, contactAbility identityAbility) *gateway {
	return &gateway{
		config:    config,
		message:   messageAbility,
		contact:   contactAbility,
		routes:    newRouteStore(defaultRouteLimit),
		clients:   make(map[*wsClient]struct{}),
		serveDone: make(chan struct{}),
		httpClient: &http.Client{
			Timeout: config.RemoteMediaTimeout,
		},
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(*http.Request) bool {
				// This endpoint is intended for non-browser bot clients. Bearer
				// authentication is the trust boundary when a token is configured.
				return true
			},
		},
	}
}

func (g *gateway) start() error {
	listener, err := net.Listen("tcp", g.config.Listen)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc(g.config.Path, g.serveWebSocket)
	g.listener = listener
	g.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	g.cancel = cancel
	g.refreshIdentity()
	go g.heartbeatLoop(ctx)
	go func() {
		defer close(g.serveDone)
		if err := g.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("[onebot11] WebSocket HTTP 服务异常退出", "err", err)
		}
	}()
	return nil
}

func (g *gateway) stop(ctx context.Context) error {
	if g.cancel != nil {
		g.cancel()
	}

	g.mu.RLock()
	clients := make([]*wsClient, 0, len(g.clients))
	for client := range g.clients {
		clients = append(clients, client)
	}
	g.mu.RUnlock()
	for _, client := range clients {
		client.close(websocket.CloseGoingAway, "plugin stopped")
	}

	if g.server == nil {
		return nil
	}
	return g.server.Shutdown(ctx)
}

func (g *gateway) serveWebSocket(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != g.config.Path {
		http.NotFound(writer, request)
		return
	}
	if !g.authorized(request) {
		writer.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := g.upgrader.Upgrade(writer, request, nil)
	if err != nil {
		slog.Warn("[onebot11] WebSocket 握手失败", "remote", request.RemoteAddr, "err", err)
		return
	}
	conn.SetReadLimit(g.config.MaxFrameBytes)
	client := &wsClient{
		gateway: g,
		conn:    conn,
		send:    make(chan []byte, g.config.ClientBuffer),
		done:    make(chan struct{}),
	}
	g.addClient(client)
	go client.writeLoop()
	client.enqueue(g.mustMarshal(lifecycleEvent{
		Time:          time.Now().Unix(),
		SelfID:        g.identityID(),
		PostType:      "meta_event",
		MetaEventType: "lifecycle",
		SubType:       "connect",
	}))
	slog.Info("[onebot11] WebSocket 客户端已连接", "remote", request.RemoteAddr)

	defer func() {
		client.close(websocket.CloseNormalClosure, "connection closed")
		slog.Info("[onebot11] WebSocket 客户端已断开", "remote", request.RemoteAddr)
	}()
	for {
		messageType, frame, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				slog.Debug("[onebot11] 读取 WebSocket 帧结束", "remote", request.RemoteAddr, "err", err)
			}
			return
		}
		if messageType != websocket.TextMessage {
			continue
		}
		response := g.handleActionFrame(frame)
		if len(response) > 0 && !client.enqueue(response) {
			return
		}
	}
}

func (g *gateway) authorized(request *http.Request) bool {
	if g.config.AccessToken == "" {
		return true
	}
	const prefix = "Bearer "
	authorization := request.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, prefix) {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	return subtle.ConstantTimeCompare([]byte(provided), []byte(g.config.AccessToken)) == 1
}

func (g *gateway) addClient(client *wsClient) {
	g.mu.Lock()
	g.clients[client] = struct{}{}
	g.mu.Unlock()
}

func (g *gateway) removeClient(client *wsClient) {
	g.mu.Lock()
	delete(g.clients, client)
	g.mu.Unlock()
}

func (g *gateway) broadcast(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	g.mu.RLock()
	clients := make([]*wsClient, 0, len(g.clients))
	for client := range g.clients {
		clients = append(clients, client)
	}
	g.mu.RUnlock()
	for _, client := range clients {
		if !client.enqueue(data) {
			client.close(websocket.ClosePolicyViolation, "outbound queue full")
		}
	}
	return nil
}

func (g *gateway) publishMessage(msg *message.Message) error {
	event, ok := buildMessageEvent(msg, g.identityID())
	if !ok {
		return nil
	}
	receiver := msg.GetSender().GetUsername()
	g.routes.put(event.MessageID, receiver)
	return g.broadcast(event)
}

func (g *gateway) heartbeatLoop(ctx context.Context) {
	if g.config.HeartbeatInterval <= 0 {
		return
	}
	ticker := time.NewTicker(g.config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = g.broadcast(heartbeatEvent{
				Time:          time.Now().Unix(),
				SelfID:        g.identityID(),
				PostType:      "meta_event",
				MetaEventType: "heartbeat",
				Status:        heartbeatStatus{Online: true, Good: true},
				Interval:      g.config.HeartbeatInterval.Milliseconds(),
			})
		}
	}
}

func (g *gateway) refreshIdentity() {
	self := g.contact.GetSelf()
	if self == nil {
		return
	}
	g.identityMu.Lock()
	g.selfID = strings.TrimSpace(self.GetUsername())
	g.nickname = firstNonBlank(self.GetNickname(), self.GetAlias(), self.GetUsername())
	g.identityMu.Unlock()
}

func (g *gateway) identityID() string {
	g.identityMu.RLock()
	defer g.identityMu.RUnlock()
	return g.selfID
}

func (g *gateway) identitySnapshot() (string, string) {
	g.refreshIdentity()
	g.identityMu.RLock()
	defer g.identityMu.RUnlock()
	return g.selfID, g.nickname
}

func (g *gateway) mustMarshal(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		slog.Error("[onebot11] JSON 序列化失败", "err", err)
		return []byte(fmt.Sprintf(`{"post_type":"meta_event","error":%q}`, err.Error()))
	}
	return data
}

func (c *wsClient) enqueue(frame []byte) bool {
	select {
	case <-c.done:
		return false
	default:
	}
	select {
	case <-c.done:
		return false
	case c.send <- frame:
		return true
	default:
		return false
	}
}

func (c *wsClient) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case frame := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(c.gateway.config.WriteTimeout)); err != nil {
				c.close(websocket.CloseInternalServerErr, "write deadline failed")
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, frame); err != nil {
				c.close(websocket.CloseAbnormalClosure, "write failed")
				return
			}
		}
	}
}

func (c *wsClient) close(code int, reason string) {
	c.once.Do(func() {
		c.gateway.removeClient(c)
		close(c.done)
		deadline := time.Now().Add(c.gateway.config.WriteTimeout)
		_ = c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), deadline)
		_ = c.conn.Close()
	})
}
