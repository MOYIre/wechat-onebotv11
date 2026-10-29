package onebot

import (
	"context"
	"encoding/json"
	"io"
	"testing"
)

func TestEncodeIDStableAndPositive(t *testing.T) {
	first := EncodeID("@alice")
	if first <= 0 {
		t.Fatalf("EncodeID() = %d, want positive", first)
	}
	if got := EncodeID("@alice"); got != first {
		t.Fatalf("EncodeID() changed: got %d, want %d", got, first)
	}
	if first == EncodeID("@@group") {
		t.Fatal("sample user and group IDs collided")
	}
}

func TestIDMapperRoundTrip(t *testing.T) {
	mapper := NewIDMapper()
	id := mapper.Bind("@alice")
	if got, ok := mapper.Username(id); !ok || got != "@alice" {
		t.Fatalf("Username(%d) = %q, %v", id, got, ok)
	}
	if mapper.Size() != 1 {
		t.Fatalf("Size() = %d, want 1", mapper.Size())
	}
}

func TestParseCQCodes(t *testing.T) {
	segments, err := parseMessageParam(json.RawMessage(`"hello[CQ:at,qq=123]![CQ:image,file=base64://aGVsbG8=]"`), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 4 {
		t.Fatalf("got %d segments, want 4", len(segments))
	}
	if got := SegmentsText(segments); got != "hello!" {
		t.Fatalf("SegmentsText() = %q, want %q", got, "hello!")
	}
	if got := intValue(segments[1].Data["qq"]); got != 123 {
		t.Fatalf("at qq = %d, want 123", got)
	}
}

func TestParseMessageArrayAndAutoEscape(t *testing.T) {
	segments, err := parseMessageParam(json.RawMessage(`[{"type":"text","data":{"text":"[CQ:at,qq=1]"}}]`), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := SegmentsText(segments); got != "[CQ:at,qq=1]" {
		t.Fatalf("auto escaped text = %q", got)
	}
}

func TestAPIUnknownAction(t *testing.T) {
	api := NewAPI(nil, NewIDMapper(), NewMessageStore(), nil)
	response := api.Handle(context.Background(), Request{Action: "unsupported_action", Echo: "test"})
	if response.Status != "failed" || response.Retcode != 1404 || response.Echo != "test" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestSendMsgRequiresMessageType(t *testing.T) {
	api := NewAPI(nil, NewIDMapper(), NewMessageStore(), nil)
	_, err := api.Call(context.Background(), "send_msg", json.RawMessage(`{"user_id":123,"message":"hello"}`))
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != 1400 {
		t.Fatalf("Call() error = %v, want APIError 1400", err)
	}
}

func TestMessageStoreIDs(t *testing.T) {
	store := NewMessageStore()
	first := store.NextID()
	second := store.NextID()
	if second != first+1 {
		t.Fatalf("IDs = %d, %d; want consecutive values", first, second)
	}
}

func TestConfigValidation(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.WSURL = "http://127.0.0.1:6199/ws"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid WebSocket scheme")
	}
}

func TestOpenImageBase64(t *testing.T) {
	file, err := openImage(context.Background(), "base64://aGVsbG8=")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("decoded image = %q, want hello", data)
	}
}
