package onebot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eatmoreapple/openwechat/pkg/openwechat"
)

const Version = "0.1.0"

type Request struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
	Echo   any             `json:"echo,omitempty"`
}

type Response struct {
	Status  string `json:"status"`
	Retcode int    `json:"retcode"`
	Data    any    `json:"data"`
	Echo    any    `json:"echo,omitempty"`
	Wording string `json:"wording,omitempty"`
}

type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string { return e.Message }

type API struct {
	bot    *openwechat.Bot
	ids    *IDMapper
	store  *MessageStore
	sender *Sender
	logger *log.Logger
}

func NewAPI(bot *openwechat.Bot, ids *IDMapper, store *MessageStore, logger *log.Logger) *API {
	if logger == nil {
		logger = log.Default()
	}
	return &API{bot: bot, ids: ids, store: store, sender: NewSender(bot, ids), logger: logger}
}

func (a *API) Handle(ctx context.Context, req Request) Response {
	data, err := a.Call(ctx, req.Action, req.Params)
	if err == nil {
		return Response{Status: "ok", Retcode: 0, Data: data, Echo: req.Echo}
	}
	retcode := 1
	if apiErr, ok := err.(*APIError); ok {
		retcode = apiErr.Code
	}
	a.logger.Printf("onebot: action %q failed: %v", req.Action, err)
	return Response{Status: "failed", Retcode: retcode, Data: nil, Echo: req.Echo, Wording: err.Error()}
}

func (a *API) Call(ctx context.Context, action string, raw json.RawMessage) (any, error) {
	params := raw
	if len(params) == 0 || string(params) == "null" {
		params = json.RawMessage(`{}`)
	}
	switch action {
	case "get_login_info":
		self, err := a.self()
		if err != nil {
			return nil, err
		}
		id := a.ids.Bind(self.UserName)
		return map[string]any{"user_id": id, "nickname": self.NickName}, nil
	case "get_friend_list":
		self, err := a.self()
		if err != nil {
			return nil, err
		}
		friends, err := self.Friends()
		if err != nil {
			return nil, err
		}
		result := make([]map[string]any, 0, len(friends))
		for _, friend := range friends {
			result = append(result, map[string]any{"user_id": a.ids.Bind(friend.UserName), "nickname": friend.NickName, "remark": friend.RemarkName})
		}
		return result, nil
	case "get_group_list":
		self, err := a.self()
		if err != nil {
			return nil, err
		}
		groups, err := self.Groups()
		if err != nil {
			return nil, err
		}
		result := make([]map[string]any, 0, len(groups))
		for _, group := range groups {
			result = append(result, map[string]any{"group_id": a.ids.Bind(group.UserName), "group_name": group.NickName, "member_count": group.MemberCount, "max_member_count": 500})
		}
		return result, nil
	case "get_group_info":
		var p struct {
			GroupID int64 `json:"group_id"`
		}
		if err := decodeParams(params, &p); err != nil {
			return nil, err
		}
		group, err := a.resolveGroup(p.GroupID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"group_id": p.GroupID, "group_name": group.NickName, "member_count": group.MemberCount, "max_member_count": 500}, nil
	case "get_group_member_list":
		var p struct {
			GroupID int64 `json:"group_id"`
		}
		if err := decodeParams(params, &p); err != nil {
			return nil, err
		}
		group, err := a.resolveGroup(p.GroupID)
		if err != nil {
			return nil, err
		}
		members, err := group.Members()
		if err != nil {
			return nil, err
		}
		result := make([]map[string]any, 0, len(members))
		for _, member := range members {
			result = append(result, a.memberInfo(p.GroupID, member))
		}
		return result, nil
	case "get_group_member_info":
		var p struct {
			GroupID int64 `json:"group_id"`
			UserID  int64 `json:"user_id"`
			NoCache bool  `json:"no_cache"`
		}
		if err := decodeParams(params, &p); err != nil {
			return nil, err
		}
		group, err := a.resolveGroup(p.GroupID)
		if err != nil {
			return nil, err
		}
		members, err := group.Members()
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			if a.ids.Bind(member.UserName) == p.UserID {
				return a.memberInfo(p.GroupID, member), nil
			}
		}
		return nil, &APIError{Code: 1404, Message: "group member not found"}
	case "send_private_msg", "send_group_msg", "send_msg":
		return a.send(ctx, action, params)
	case "get_msg":
		var p struct {
			MessageID int64 `json:"message_id"`
		}
		if err := decodeParams(params, &p); err != nil {
			return nil, err
		}
		event, ok := a.store.Event(p.MessageID)
		if !ok {
			return nil, &APIError{Code: 1404, Message: "message not found"}
		}
		return map[string]any{"message_id": event.MessageID, "real_id": event.MessageID, "time": event.Time, "message_type": event.MessageType, "user_id": event.UserID, "group_id": event.GroupID, "sender": event.Sender, "message": event.Message}, nil
	case "delete_msg":
		var p struct {
			MessageID int64 `json:"message_id"`
		}
		if err := decodeParams(params, &p); err != nil {
			return nil, err
		}
		sent, ok := a.store.Sent(p.MessageID)
		if !ok {
			return nil, &APIError{Code: 1404, Message: "sent message not found"}
		}
		if !sent.CanRevoke() {
			return nil, &APIError{Code: 1400, Message: "message is outside WeChat revoke window"}
		}
		self, err := a.self()
		if err != nil {
			return nil, err
		}
		if err := self.RevokeMessage(sent); err != nil {
			return nil, err
		}
		return map[string]any{}, nil
	case "get_status":
		online := a.bot.Alive()
		return map[string]any{"app_initialized": online, "app_enabled": true, "app_good": online, "online": online, "good": online, "stat": map[string]any{}}, nil
	case "get_version_info":
		return map[string]any{"app_name": "openwechat-onebot", "app_version": Version, "protocol_version": "v11"}, nil
	case "can_send_image":
		return map[string]any{"yes": true}, nil
	case "can_send_record":
		return map[string]any{"yes": false}, nil
	default:
		return nil, &APIError{Code: 1404, Message: "unsupported action: " + action}
	}
}

func (a *API) self() (*openwechat.Self, error) {
	self, err := a.bot.GetCurrentUser()
	if err != nil {
		return nil, err
	}
	a.ids.Bind(self.UserName)
	return self, nil
}

func decodeParams(raw json.RawMessage, target any) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return &APIError{Code: 1400, Message: "invalid params: " + err.Error()}
	}
	return nil
}

type sendParams struct {
	UserID      int64           `json:"user_id"`
	GroupID     int64           `json:"group_id"`
	MessageType string          `json:"message_type"`
	Message     json.RawMessage `json:"message"`
	AutoEscape  bool            `json:"auto_escape"`
}

func (a *API) send(ctx context.Context, action string, raw json.RawMessage) (any, error) {
	var p sendParams
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	isGroup := action == "send_group_msg"
	if action == "send_msg" {
		switch p.MessageType {
		case "group":
			isGroup = true
		case "private":
			isGroup = false
		default:
			return nil, &APIError{Code: 1400, Message: "message_type must be private or group"}
		}
	}
	var targetID int64
	if isGroup {
		targetID = p.GroupID
	} else {
		targetID = p.UserID
	}
	if targetID <= 0 {
		return nil, &APIError{Code: 1400, Message: "missing target id"}
	}
	username, err := a.resolveTarget(targetID, isGroup)
	if err != nil {
		return nil, err
	}
	segments, err := parseMessageParam(p.Message, p.AutoEscape)
	if err != nil {
		return nil, &APIError{Code: 1400, Message: err.Error()}
	}
	self, err := a.self()
	if err != nil {
		return nil, err
	}
	sent, err := a.sender.Send(ctx, username, isGroup, segments)
	if err != nil {
		return nil, err
	}
	id := a.store.NextID()
	selfID := a.ids.ID(self.UserName)
	event := &MessageEvent{
		Time:        time.Now().Unix(),
		SelfID:      selfID,
		PostType:    "message",
		MessageType: "private",
		SubType:     "friend",
		MessageID:   id,
		UserID:      selfID,
		Message:     segments,
		RawMessage:  SegmentsText(segments),
		Sender:      SenderInfo{UserID: selfID, Nickname: self.NickName},
	}
	if isGroup {
		event.MessageType = "group"
		event.SubType = "normal"
		event.GroupID = targetID
	}
	a.store.PutSentEvent(id, sent, event)
	return map[string]any{"message_id": id}, nil
}

func (a *API) resolveTarget(id int64, group bool) (string, error) {
	if username, ok := a.ids.Username(id); ok {
		return username, nil
	}
	self, err := a.self()
	if err != nil {
		return "", err
	}
	if group {
		groups, err := self.Groups()
		if err != nil {
			return "", err
		}
		for _, item := range groups {
			if EncodeID(item.UserName) == id {
				a.ids.Bind(item.UserName)
				return item.UserName, nil
			}
		}
	} else {
		members, err := self.Members()
		if err != nil {
			return "", err
		}
		for _, item := range members {
			if EncodeID(item.UserName) == id {
				a.ids.Bind(item.UserName)
				return item.UserName, nil
			}
		}
	}
	return "", &APIError{Code: 1404, Message: fmt.Sprintf("target id %d not found", id)}
}

func (a *API) resolveGroup(id int64) (*openwechat.Group, error) {
	username, err := a.resolveTarget(id, true)
	if err != nil {
		return nil, err
	}
	self, err := a.self()
	if err != nil {
		return nil, err
	}
	groups, err := self.Groups()
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if group.UserName == username {
			return group, nil
		}
	}
	return nil, &APIError{Code: 1404, Message: "group not found"}
}

func (a *API) memberInfo(groupID int64, member *openwechat.User) map[string]any {
	role := "member"
	if member.IsOwner != 0 {
		role = "owner"
	}
	return map[string]any{
		"group_id":          groupID,
		"user_id":           a.ids.Bind(member.UserName),
		"nickname":          member.NickName,
		"card":              member.DisplayName,
		"sex":               sexOf(member.Sex),
		"age":               0,
		"area":              strings.Trim(strings.Join([]string{member.Province, member.City}, " "), " "),
		"join_time":         0,
		"last_sent_time":    0,
		"level":             "",
		"role":              role,
		"unfriendly":        false,
		"title":             "",
		"title_expire_time": 0,
		"card_changeable":   false,
	}
}
