package onebot

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// 消息段中支持发送的类型, 其余类型(at/reply 除外)在发送时被忽略
const (
	segText  = "text"
	segAt    = "at"
	segImage = "image"
	segReply = "reply"
	segFace  = "face"
)

var cqCodeRegexp = regexp.MustCompile(`\[CQ:([^\[\],]+)((?:,[^\[\]]*)*)\]`)

// parseMessageParam 将 OneBot v11 的 message 字段统一解析为消息段数组。
//
// message 可以是消息段数组, 也可以是纯文本(可能含 CQ 码)。
// autoEscape 为 true 时纯文本不再做 CQ 码解析。
func parseMessageParam(raw json.RawMessage, autoEscape bool) ([]Segment, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var segs []Segment
		if err := json.Unmarshal(raw, &segs); err != nil {
			return nil, fmt.Errorf("invalid message array: %w", err)
		}
		return segs, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, fmt.Errorf("invalid message: %w", err)
	}
	if autoEscape {
		return []Segment{TextSegment(text)}, nil
	}
	return parseCQCodes(text), nil
}

// parseCQCodes 将 CQ 码字符串解析为消息段
func parseCQCodes(text string) []Segment {
	var segs []Segment
	last := 0
	for _, m := range cqCodeRegexp.FindAllStringSubmatchIndex(text, -1) {
		start, end := m[0], m[1]
		if start > last {
			segs = append(segs, TextSegment(unescapeCQ(text[last:start])))
		}
		name := text[m[2]:m[3]]
		var data map[string]any
		if m[4] >= 0 {
			data = parseCQParams(text[m[4]:m[5]])
		}
		switch name {
		case segText:
			if t, ok := data["text"].(string); ok {
				segs = append(segs, TextSegment(unescapeCQ(t)))
			}
		case segAt:
			segs = append(segs, Segment{Type: segAt, Data: data})
		case segImage:
			segs = append(segs, Segment{Type: segImage, Data: data})
		case segReply:
			segs = append(segs, Segment{Type: segReply, Data: data})
		case segFace:
			segs = append(segs, Segment{Type: segFace, Data: data})
		default:
			// 其余 CQ 码不支持, 直接丢弃
		}
		last = end
	}
	if last < len(text) {
		segs = append(segs, TextSegment(unescapeCQ(text[last:])))
	}
	if len(segs) == 0 && text != "" {
		segs = append(segs, TextSegment(unescapeCQ(text)))
	}
	return segs
}

func parseCQParams(s string) map[string]any {
	// s 形如 ",key1=value1,key2=value2"
	params := map[string]any{}
	for _, kv := range strings.Split(strings.TrimPrefix(s, ","), ",") {
		if kv == "" {
			continue
		}
		idx := strings.Index(kv, "=")
		if idx < 0 {
			params[unescapeCQ(kv)] = ""
			continue
		}
		params[unescapeCQ(kv[:idx])] = unescapeCQ(kv[idx+1:])
	}
	return params
}

func unescapeCQ(s string) string {
	s = strings.ReplaceAll(s, "&#91;", "[")
	s = strings.ReplaceAll(s, "&#93;", "]")
	s = strings.ReplaceAll(s, "&#44;", ",")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return s
}

// SegmentsText 拼接所有文本段, 主要用于测试与日志
func SegmentsText(segs []Segment) string {
	var sb strings.Builder
	for _, seg := range segs {
		if seg.Type != segText {
			continue
		}
		if t, ok := seg.Data["text"].(string); ok {
			sb.WriteString(t)
		}
	}
	return sb.String()
}
