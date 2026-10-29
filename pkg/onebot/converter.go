package onebot

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"regexp"
	"time"

	"github.com/eatmoreapple/openwechat/pkg/openwechat"
)

// maxImageBytes 接收图片的最大字节数, 超出则降级为占位文本
const maxImageBytes = 4 * 1024 * 1024

// 群消息内容里 "@昵称\u2005" 的前缀, 转成 OneBot 时需要剥离
var atPrefixRegexp = regexp.MustCompile(`^@[^\x{2005}]*\x{2005}`)

// Converter 把 openwechat 的消息转换成 OneBot v11 事件
type Converter struct {
	bot    *openwechat.Bot
	ids    *IDMapper
	store  *MessageStore
	logger *log.Logger
}

func NewConverter(bot *openwechat.Bot, ids *IDMapper, store *MessageStore, logger *log.Logger) *Converter {
	if logger == nil {
		logger = log.Default()
	}
	return &Converter{bot: bot, ids: ids, store: store, logger: logger}
}

// Convert 转换一条消息, 返回 nil 表示该消息不需要上报(自己发送的消息、不支持的类型等)
func (c *Converter) Convert(msg *openwechat.Message) *MessageEvent {
	self, selfErr := c.bot.GetCurrentUser()
	if selfErr != nil {
		c.logger.Printf("onebot: get current user failed: %v", selfErr)
		return nil
	}
	// 自己发出的消息不再上报, 避免 AstrBot 处理后形成回环
	if msg.IsSendBySelf() {
		return nil
	}
	if msg.IsSystem() || msg.IsNotify() || msg.IsRecalled() {
		return nil
	}

	isGroup := msg.IsSendByGroup()
	var groupUserName, senderUserName string
	if isGroup {
		groupUserName = msg.FromUserName
	} else if msg.IsSendByFriend() {
		senderUserName = msg.FromUserName
	} else {
		// 公众号等其它来源暂不上报
		return nil
	}

	var (
		sender *openwechat.User
		err    error
	)
	if isGroup {
		sender, err = msg.SenderInGroup()
	} else {
		sender, err = msg.Sender()
	}
	if err != nil || sender == nil {
		c.logger.Printf("onebot: resolve sender failed: %v", err)
		return nil
	}
	senderUserName = sender.UserName
	card := sender.RemarkName
	if isGroup {
		card = sender.DisplayName
	}

	text := msg.Content
	segs := make([]Segment, 0, 2)
	switch {
	case msg.IsText():
		body := stripAtPrefix(text, msg.IsAt())
		segs = append(segs, TextSegment(body))
	case msg.IsPicture():
		data, imgErr := c.readPicture(msg)
		if imgErr != nil {
			c.logger.Printf("onebot: download image failed: %v", imgErr)
			segs = append(segs, TextSegment("[图片]"))
		} else {
			segs = append(segs, ImageSegment("base64://"+base64.StdEncoding.EncodeToString(data), "[图片]"))
		}
	default:
		// 其余类型暂不支持, 不上报
		return nil
	}

	rawMessage := SegmentsText(segs)
	if msg.IsPicture() {
		rawMessage = "[图片]"
	}
	event := &MessageEvent{
		Time:        time.Now().Unix(),
		SelfID:      c.ids.ID(self.UserName),
		PostType:    "message",
		MessageType: "private",
		SubType:     "friend",
		MessageID:   c.store.NextID(),
		UserID:      c.ids.ID(senderUserName),
		Message:     segs,
		RawMessage:  rawMessage,
		Font:        0,
		Sender: SenderInfo{
			UserID:   c.ids.ID(senderUserName),
			Nickname: sender.NickName,
			Card:     card,
			Sex:      sexOf(sender.Sex),
		},
	}
	if isGroup {
		event.MessageType = "group"
		event.SubType = "normal"
		event.GroupID = c.ids.ID(groupUserName)
		if msg.IsAt() {
			event.Message = append([]Segment{AtSegment(event.SelfID)}, event.Message...)
		}
	}
	c.ids.Bind(senderUserName)
	c.ids.Bind(groupUserName)
	c.store.PutEvent(event.MessageID, event)
	return event
}

// readPicture 下载图片消息的内容
func (c *Converter) readPicture(msg *openwechat.Message) ([]byte, error) {
	resp, err := msg.GetPicture()
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("image too large: > %d bytes", maxImageBytes)
	}
	return data, nil
}

// stripAtPrefix 群消息被 @ 时移除内容里的 "@昵称\u2005" 前缀, 由 OneBot 的 at 段代替
func stripAtPrefix(content string, isAt bool) string {
	if !isAt {
		return content
	}
	return atPrefixRegexp.ReplaceAllString(content, "")
}

func sexOf(sex int) string {
	switch sex {
	case 1:
		return "male"
	case 2:
		return "female"
	default:
		return "unknown"
	}
}
