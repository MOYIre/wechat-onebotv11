package onebot

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/eatmoreapple/openwechat/pkg/openwechat"
)

const maxOutboundImageBytes = 20 * 1024 * 1024

// Sender 将 OneBot 消息段发送到微信
type Sender struct {
	bot *openwechat.Bot
	ids *IDMapper
}

func NewSender(bot *openwechat.Bot, ids *IDMapper) *Sender {
	return &Sender{bot: bot, ids: ids}
}

// Send 按消息段顺序发送文本和图片。微信接口不支持单条消息混合发送多种类型,
// 因此混合消息会拆分为多条微信消息, 并返回最后一条发送结果。
func (s *Sender) Send(ctx context.Context, target string, isGroup bool, segments []Segment) (*openwechat.SentMessage, error) {
	self, err := s.bot.GetCurrentUser()
	if err != nil {
		return nil, err
	}
	var (
		friend *openwechat.Friend
		group  *openwechat.Group
	)
	if isGroup {
		groups, err := self.Groups()
		if err != nil {
			return nil, err
		}
		for _, item := range groups {
			if item.UserName == target {
				group = item
				break
			}
		}
		if group == nil {
			return nil, fmt.Errorf("group %q not found", target)
		}
	} else {
		friends, err := self.Friends()
		if err != nil {
			return nil, err
		}
		for _, item := range friends {
			if item.UserName == target {
				friend = item
				break
			}
		}
		if friend == nil && target == openwechat.FileHelper {
			friend = self.FileHelper()
		}
		if friend == nil {
			return nil, fmt.Errorf("friend %q not found", target)
		}
	}

	var text strings.Builder
	var last *openwechat.SentMessage
	flushText := func() error {
		if text.Len() == 0 {
			return nil
		}
		content := text.String()
		text.Reset()
		if isGroup {
			last, err = group.SendText(content)
		} else {
			last, err = friend.SendText(content)
		}
		return err
	}

	for _, segment := range segments {
		switch segment.Type {
		case segText:
			if value, ok := segment.Data["text"].(string); ok {
				text.WriteString(value)
			}
		case segAt:
			if isGroup {
				text.WriteString(s.mentionText(segment, group))
			}
		case segImage:
			if err := flushText(); err != nil {
				return nil, err
			}
			file, err := openImage(ctx, stringValue(segment.Data["file"]))
			if err != nil {
				return nil, err
			}
			if isGroup {
				last, err = group.SendImage(file)
			} else {
				last, err = friend.SendImage(file)
			}
			_ = file.Close()
			if err != nil {
				return nil, err
			}
		case segReply, segFace:
			// 回复 ID 在微信侧无直接对应关系, 表情暂不进行跨平台转换。
		default:
			// 忽略 OneBot 扩展段及当前不支持的消息段
		}
	}
	if err := flushText(); err != nil {
		return nil, err
	}
	if last == nil {
		return nil, fmt.Errorf("message contains no sendable segment")
	}
	return last, nil
}

func (s *Sender) mentionText(segment Segment, group *openwechat.Group) string {
	qq := intValue(segment.Data["qq"])
	if qq <= 0 {
		return ""
	}
	if name, ok := s.ids.Username(qq); ok {
		if member, err := group.SearchMemberByUsername(name); err == nil {
			display := member.DisplayName
			if display == "" {
				display = member.NickName
			}
			if display != "" {
				return "@" + display + "\u2005"
			}
		}
	}
	return "@" + fmt.Sprint(qq) + "\u2005"
}

func openImage(ctx context.Context, source string) (io.ReadCloser, error) {
	if source == "" {
		return nil, fmt.Errorf("image file is empty")
	}
	if strings.HasPrefix(source, "base64://") {
		data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(source, "base64://"))
		if err != nil {
			return nil, fmt.Errorf("decode base64 image: %w", err)
		}
		if len(data) > maxOutboundImageBytes {
			return nil, fmt.Errorf("image exceeds %d bytes", maxOutboundImageBytes)
		}
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, err
		}
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("download image: HTTP %s", resp.Status)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxOutboundImageBytes+1))
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if len(data) > maxOutboundImageBytes {
			return nil, fmt.Errorf("image exceeds %d bytes", maxOutboundImageBytes)
		}
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	path := source
	if strings.HasPrefix(path, "file://") {
		u, err := url.Parse(path)
		if err != nil {
			return nil, err
		}
		path = u.Path
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if stat.Size() > maxOutboundImageBytes {
		_ = file.Close()
		return nil, fmt.Errorf("image exceeds %d bytes", maxOutboundImageBytes)
	}
	return file, nil
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

func intValue(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		var n int64
		_, _ = fmt.Sscan(v, &n)
		return n
	default:
		return 0
	}
}
