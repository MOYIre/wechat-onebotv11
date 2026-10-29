package onebot

import (
	"hash/fnv"
	"sync"
)

// EncodeID 将微信的字符串 UserName 稳定映射为 OneBot v11 需要的正整数 ID。
//
// OneBot v11 的 user_id / group_id 是 int64, 而微信的 UserName 是 "@xxx" 这样的
// 字符串, 无法直接转换。这里使用 FNV-1a 哈希取正数, 同一个 UserName 始终映射到
// 同一个 ID, 重启后也保持不变。
func EncodeID(username string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(username))
	// 限制在 IEEE 754 可精确表示范围内, 兼容 JavaScript 客户端。
	id := int64(h.Sum64() & 0x001fffffffffffff)
	if id == 0 {
		id = 1
	}
	return id
}

// IDMapper 维护 ID 到 UserName 的反查关系。
//
// ID 是单向哈希无法反解, 所以记录下见过的映射。未命中时可以扫描联系人列表补全,
// 此时再缓存结果, 避免每次发送消息都全量扫描。
type IDMapper struct {
	mu     sync.RWMutex
	byID   map[int64]string
	byName map[string]int64
}

func NewIDMapper() *IDMapper {
	return &IDMapper{
		byID:   make(map[int64]string),
		byName: make(map[string]int64),
	}
}

// Bind 记录 UserName 与其 ID 的映射, 已存在则保持不变
func (m *IDMapper) Bind(username string) int64 {
	if username == "" {
		return 0
	}
	m.mu.RLock()
	id, ok := m.byName[username]
	m.mu.RUnlock()
	if ok {
		return id
	}
	id = EncodeID(username)
	m.mu.Lock()
	m.byName[username] = id
	m.byID[id] = username
	m.mu.Unlock()
	return id
}

// ID 返回 UserName 对应的 ID, 不存在则现场计算
func (m *IDMapper) ID(username string) int64 {
	if username == "" {
		return 0
	}
	m.mu.RLock()
	id, ok := m.byName[username]
	m.mu.RUnlock()
	if ok {
		return id
	}
	return EncodeID(username)
}

// Username 反查 UserName, 未命中返回 ("", false)
func (m *IDMapper) Username(id int64) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	name, ok := m.byID[id]
	return name, ok
}

// Size 返回已缓存的映射数量, 主要用于测试
func (m *IDMapper) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.byID)
}
