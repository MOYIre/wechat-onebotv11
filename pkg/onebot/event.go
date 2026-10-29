package onebot

import "encoding/json"

// Segment OneBot v11 消息段
type Segment struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data,omitempty"`
}

// TextSegment 构造文本消息段
func TextSegment(text string) Segment {
	return Segment{Type: "text", Data: map[string]any{"text": text}}
}

// AtSegment 构造 @ 消息段
func AtSegment(qq int64) Segment {
	return Segment{Type: "at", Data: map[string]any{"qq": qq}}
}

// ReplySegment 构造回复消息段
func ReplySegment(id int64) Segment {
	return Segment{Type: "reply", Data: map[string]any{"id": id}}
}

// ImageSegment 构造图片消息段, file 支持 base64:// / http(s):// / 本地路径
func ImageSegment(file string, summary string) Segment {
	data := map[string]any{"file": file}
	if summary != "" {
		data["summary"] = summary
	}
	return Segment{Type: "image", Data: data}
}

// SenderInfo OneBot v11 事件中的发送者信息
type SenderInfo struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Card     string `json:"card,omitempty"`
	Sex      string `json:"sex,omitempty"`
	Age      int    `json:"age"`
	Area     string `json:"area,omitempty"`
	Level    string `json:"level,omitempty"`
	Role     string `json:"role,omitempty"`
	Title    string `json:"title,omitempty"`
}

// MessageEvent OneBot v11 上报消息事件
type MessageEvent struct {
	Time        int64      `json:"time"`
	SelfID      int64      `json:"self_id"`
	PostType    string     `json:"post_type"`
	MessageType string     `json:"message_type"`
	SubType     string     `json:"sub_type"`
	MessageID   int64      `json:"message_id"`
	UserID      int64      `json:"user_id"`
	GroupID     int64      `json:"group_id,omitempty"`
	Message     []Segment  `json:"message"`
	RawMessage  string     `json:"raw_message"`
	Font        int        `json:"font"`
	Sender      SenderInfo `json:"sender"`
}

// MetaEvent OneBot v11 元事件 (生命周期 / 心跳)
type MetaEvent struct {
	Time          int64  `json:"time"`
	SelfID        int64  `json:"self_id"`
	PostType      string `json:"post_type"`
	MetaEventType string `json:"meta_event_type"`
	SubType       string `json:"sub_type,omitempty"`
	Status        any    `json:"status,omitempty"`
	Interval      int64  `json:"interval,omitempty"`
}

// LifecycleEvent 构造连接成功的生命周期事件
func LifecycleEvent(selfID int64, time int64) MetaEvent {
	return MetaEvent{
		Time:          time,
		SelfID:        selfID,
		PostType:      "meta_event",
		MetaEventType: "lifecycle",
		SubType:       "connect",
	}
}

// HeartbeatEvent 构造心跳事件
func HeartbeatEvent(selfID int64, time int64, interval int64, status any) MetaEvent {
	return MetaEvent{
		Time:          time,
		SelfID:        selfID,
		PostType:      "meta_event",
		MetaEventType: "heartbeat",
		Status:        status,
		Interval:      interval,
	}
}

// EncodeJSON 编码为紧凑 JSON, 便于测试断言
func EncodeJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}
