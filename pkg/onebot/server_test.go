package onebot

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestReverseWebSocketHandshakeAndAPI(t *testing.T) {
	upgrade := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	serverErr := make(chan error, 1)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Self-ID"); got != "12345" {
			serverErr <- &testError{message: "unexpected X-Self-ID: " + got}
			return
		}
		if got := r.Header.Get("X-Client-Role"); got != "Universal" {
			serverErr <- &testError{message: "unexpected X-Client-Role: " + got}
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			serverErr <- &testError{message: "unexpected Authorization header"}
			return
		}
		conn, err := upgrade.Upgrade(w, r, nil)
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		var lifecycle MetaEvent
		if err := conn.ReadJSON(&lifecycle); err != nil {
			serverErr <- err
			return
		}
		if lifecycle.MetaEventType != "lifecycle" || lifecycle.SubType != "connect" {
			serverErr <- &testError{message: "lifecycle connect event was not sent"}
			return
		}
		if err := conn.WriteJSON(Request{Action: "get_version_info", Echo: "echo-1"}); err != nil {
			serverErr <- err
			return
		}
		var response Response
		if err := conn.ReadJSON(&response); err != nil {
			serverErr <- err
			return
		}
		if response.Status != "ok" || response.Retcode != 0 || response.Echo != "echo-1" {
			serverErr <- &testError{message: "invalid API response"}
			return
		}
		serverErr <- nil
	}))
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	cfg := DefaultConfig()
	cfg.WSURL = wsURL
	cfg.AccessToken = "secret"
	api := NewAPI(nil, NewIDMapper(), NewMessageStore(), log.New(io.Discard, "", 0))
	client := NewServer(cfg, api, 12345, log.New(io.Discard, "", 0))
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(ctx) }()

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for reverse websocket exchange")
	}
	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after context cancellation")
	}
}

type testError struct{ message string }

func (e *testError) Error() string { return e.message }

func TestHandleResponseEchoOnFailure(t *testing.T) {
	api := NewAPI(nil, NewIDMapper(), NewMessageStore(), log.New(io.Discard, "", 0))
	response := api.Handle(context.Background(), Request{Action: "invalid", Echo: json.Number("17")})
	if response.Status != "failed" || response.Retcode != 1404 || response.Echo != json.Number("17") {
		t.Fatalf("unexpected response: %+v", response)
	}
}
