package onebot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait                = 10 * time.Second
	pingInterval             = 20 * time.Second
	pongWait                 = 60 * time.Second
	maxFrameBytes            = 16 << 20
	outgoingBuffer           = 256
	eventBuffer              = 4
	maxConcurrentAPIRequests = 8
)

type outboundFrame struct {
	value any
	ack   chan error
}

// Server 是连接 AstrBot 的 OneBot v11 反向 WebSocket 客户端
type Server struct {
	cfg       Config
	api       *API
	selfID    int64
	logger    *log.Logger
	out       chan outboundFrame
	events    chan any
	dialer    websocket.Dialer
	connected bool
	stateMu   sync.RWMutex
}

func NewServer(cfg Config, api *API, selfID int64, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	return &Server{
		cfg:    cfg,
		api:    api,
		selfID: selfID,
		logger: logger,
		events: make(chan any, eventBuffer),
		dialer: websocket.Dialer{HandshakeTimeout: 10 * time.Second},
	}
}

// SendEvent 非阻塞地排队发送事件, 队列满时丢弃并记录日志
func (s *Server) SendEvent(event any) bool {
	select {
	case s.events <- event:
		return true
	default:
		s.logger.Printf("onebot: outgoing event queue is full; event dropped")
		return false
	}
}

// Run 持续连接 AstrBot, 断线后按指数退避重连, 直到 ctx 取消
func (s *Server) Run(ctx context.Context) error {
	if s.cfg.WSURL == "" {
		return fmt.Errorf("onebot websocket URL is empty")
	}
	maxDelay := s.cfg.reconnectInterval()
	delay := time.Second
	for ctx.Err() == nil {
		if err := s.serveConnection(ctx); err != nil && ctx.Err() == nil {
			s.logger.Printf("onebot: websocket disconnected: %v; reconnecting in %s", err, delay)
		}
		if ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
		if delay < maxDelay {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
	return nil
}

func (s *Server) serveConnection(ctx context.Context) error {
	header := http.Header{}
	header.Set("X-Self-ID", strconv.FormatInt(s.selfID, 10))
	header.Set("X-Client-Role", "Universal")
	header.Set("User-Agent", "openwechat-onebot/"+Version)
	if s.cfg.AccessToken != "" {
		header.Set("Authorization", "Bearer "+s.cfg.AccessToken)
	}
	conn, response, err := s.dialer.DialContext(ctx, s.cfg.WSURL, header)
	if err != nil {
		if response != nil {
			return fmt.Errorf("websocket handshake (HTTP %s): %w", response.Status, err)
		}
		return err
	}
	s.logger.Printf("onebot: connected to %s", s.cfg.WSURL)
	out := make(chan outboundFrame, outgoingBuffer)
	s.stateMu.Lock()
	s.out = out
	s.connected = true
	s.stateMu.Unlock()
	defer func() {
		s.stateMu.Lock()
		if s.out == out {
			s.connected = false
			s.out = nil
		}
		s.stateMu.Unlock()
	}()

	connCtx, cancel := context.WithCancel(ctx)
	var closeOnce sync.Once
	closeConn := func() {
		closeOnce.Do(func() {
			cancel()
			_ = conn.Close()
		})
	}
	defer closeConn()
	go func() {
		select {
		case <-ctx.Done():
			closeConn()
		case <-connCtx.Done():
		}
	}()

	conn.SetReadLimit(maxFrameBytes)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	if err := s.writeDirect(conn, LifecycleEvent(s.selfID, time.Now().Unix())); err != nil {
		return err
	}
	go s.writePump(connCtx, conn, out, closeConn)
	apiSlots := make(chan struct{}, maxConcurrentAPIRequests)
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		var req Request
		if err := json.Unmarshal(payload, &req); err != nil {
			s.logger.Printf("onebot: invalid websocket request: %v", err)
			continue
		}
		select {
		case apiSlots <- struct{}{}:
		default:
			busy := Response{Status: "failed", Retcode: 1, Wording: "too many concurrent API requests", Echo: req.Echo}
			if err := s.writeFrame(connCtx, out, busy); err != nil {
				return err
			}
			continue
		}
		go func(request Request) {
			defer func() { <-apiSlots }()
			response := s.api.Handle(connCtx, request)
			if err := s.writeFrame(connCtx, out, response); err != nil && connCtx.Err() == nil {
				s.logger.Printf("onebot: write API response failed: %v", err)
			}
		}(req)
	}
}

func (s *Server) writePump(ctx context.Context, conn *websocket.Conn, out <-chan outboundFrame, closeConn func()) {
	heartbeat := time.NewTicker(30 * time.Second)
	ping := time.NewTicker(pingInterval)
	defer heartbeat.Stop()
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-out:
			if err := s.writeDirect(conn, frame.value); err != nil {
				if frame.ack != nil {
					frame.ack <- err
				}
				closeConn()
				return
			}
			if frame.ack != nil {
				frame.ack <- nil
			}
		case event := <-s.events:
			if err := s.writeDirect(conn, event); err != nil {
				closeConn()
				return
			}
		case <-heartbeat.C:
			status := map[string]any{"online": true, "good": true}
			if err := s.writeDirect(conn, HeartbeatEvent(s.selfID, time.Now().Unix(), int64(heartbeatInterval/time.Millisecond), status)); err != nil {
				closeConn()
				return
			}
		case <-ping.C:
			_ = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait))
		}
	}
}

const heartbeatInterval = 30000 * time.Millisecond

func (s *Server) writeFrame(ctx context.Context, out chan<- outboundFrame, value any) error {
	ack := make(chan error, 1)
	select {
	case out <- outboundFrame{value: value, ack: ack}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-ack:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) writeDirect(conn *websocket.Conn, value any) error {
	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
	return conn.WriteJSON(value)
}
