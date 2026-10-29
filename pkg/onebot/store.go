package onebot

import (
	"sync"
	"sync/atomic"

	"github.com/eatmoreapple/openwechat/pkg/openwechat"
)

// storeLimit 缓存的消息上限, 超出后丢弃最早存入的一条
const storeLimit = 1024

// MessageStore 缓存最近的消息, 用于支持 get_msg / delete_msg 等 API。
//
// 微信的 MsgId 是字符串且与 OneBot 的 int 消息 ID 无法对应,
// 所以自己分配 ID 并保存映射。
type MessageStore struct {
	mu       sync.Mutex
	nextID   atomic.Int64
	events   map[int64]*MessageEvent
	sent     map[int64]*openwechat.SentMessage
	keyOrder []int64
}

func NewMessageStore() *MessageStore {
	return &MessageStore{
		events: make(map[int64]*MessageEvent),
		sent:   make(map[int64]*openwechat.SentMessage),
	}
}

// NextID 分配进程内唯一的 OneBot 消息 ID
func (s *MessageStore) NextID() int64 {
	return s.nextID.Add(1)
}

// PutEvent 保存一条接收到的消息事件
func (s *MessageStore) PutEvent(id int64, e *MessageEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[id] = compactEvent(e)
	s.track(id)
}

// PutSent 保存自己发出的消息, 用于撤回
func (s *MessageStore) PutSent(id int64, msg *openwechat.SentMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent[id] = msg
	s.track(id)
}

// PutSentEvent 同时保存自己发出的消息和对应的 OneBot 查询事件。
func (s *MessageStore) PutSentEvent(id int64, msg *openwechat.SentMessage, event *MessageEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent[id] = msg
	s.events[id] = compactEvent(event)
	s.track(id)
}

func compactEvent(event *MessageEvent) *MessageEvent {
	if event == nil {
		return nil
	}
	copyEvent := *event
	copyEvent.Message = make([]Segment, len(event.Message))
	for i, segment := range event.Message {
		copySegment := Segment{Type: segment.Type, Data: make(map[string]any, len(segment.Data))}
		for key, value := range segment.Data {
			copySegment.Data[key] = value
		}
		if segment.Type == segImage {
			copySegment.Data["file"] = "[图片]"
		}
		copyEvent.Message[i] = copySegment
	}
	return &copyEvent
}

func (s *MessageStore) track(id int64) {
	s.keyOrder = append(s.keyOrder, id)
	if len(s.keyOrder) > storeLimit {
		drop := s.keyOrder[0]
		s.keyOrder = s.keyOrder[1:]
		delete(s.events, drop)
		delete(s.sent, drop)
	}
}

// Event 取出消息事件
func (s *MessageStore) Event(id int64) (*MessageEvent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.events[id]
	return e, ok
}

// Sent 取出已发送消息
func (s *MessageStore) Sent(id int64) (*openwechat.SentMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.sent[id]
	return m, ok
}
