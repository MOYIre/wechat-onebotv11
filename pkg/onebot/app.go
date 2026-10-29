package onebot

import (
	"context"
	"io"
	"log"

	"github.com/eatmoreapple/openwechat/pkg/openwechat"
)

// App 负责把 openwechat 登录生命周期接入 OneBot v11。
type App struct {
	Config Config
	Logger *log.Logger
}

func NewApp(cfg Config, logger *log.Logger) *App {
	if logger == nil {
		logger = log.Default()
	}
	return &App{Config: cfg, Logger: logger}
}

// Run 登录微信并运行 OneBot 反向 WebSocket 连接, 直到 ctx 取消或 Bot 退出。
func (a *App) Run(ctx context.Context) error {
	if err := a.Config.Validate(); err != nil {
		return err
	}
	var mode openwechat.BotPreparer = openwechat.Desktop
	if a.Config.mode() == "normal" {
		mode = openwechat.Normal
	}
	bot := openwechat.DefaultBot(mode, openwechat.WithContextOption(ctx))
	bot.SyncCheckCallback = nil

	ids := NewIDMapper()
	store := NewMessageStore()
	api := NewAPI(bot, ids, store, a.Logger)
	converter := NewConverter(bot, ids, store, a.Logger)
	server := NewServer(a.Config, api, 0, a.Logger)
	bot.MessageHandler = func(msg *openwechat.Message) {
		if event := converter.Convert(msg); event != nil {
			server.SendEvent(event)
		}
	}

	var storage io.ReadWriteCloser
	if a.Config.HotReloadFile != "" {
		storage = openwechat.NewFileHotReloadStorage(a.Config.HotReloadFile)
		defer func() { _ = storage.Close() }()
	}

	if storage != nil {
		if err := bot.HotLogin(storage); err != nil {
			a.Logger.Printf("onebot: hot login failed, falling back to QR login: %v", err)
			if err := bot.Login(); err != nil {
				return err
			}
		}
	} else if err := bot.Login(); err != nil {
		return err
	}

	self, err := bot.GetCurrentUser()
	if err != nil {
		return err
	}
	selfID := ids.Bind(self.UserName)
	server.selfID = selfID
	a.Logger.Printf("onebot: logged in as %s (%d)", self.NickName, selfID)

	serverCtx, cancelServer := context.WithCancel(ctx)
	defer cancelServer()
	go func() {
		if err := server.Run(serverCtx); err != nil && serverCtx.Err() == nil {
			a.Logger.Printf("onebot: server stopped: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		bot.Exit()
	}()

	return bot.Block()
}
