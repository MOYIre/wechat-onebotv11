package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/eatmoreapple/openwechat/pkg/onebot"
)

func main() {
	configPath := flag.String("config", "onebot.json", "配置文件路径")
	wsURL := flag.String("ws", "", "覆盖反向 WebSocket 地址")
	token := flag.String("token", "", "覆盖 WebSocket access token")
	hotReload := flag.String("hot-reload", "", "覆盖热登录凭据文件路径")
	mode := flag.String("mode", "", "覆盖登录模式: desktop 或 normal")
	flag.Parse()

	cfg, err := onebot.LoadConfig(*configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Fatalf("加载配置失败: %v", err)
		}
		cfg = onebot.DefaultConfig()
		log.Printf("未找到配置文件 %q, 使用默认配置", *configPath)
	}
	if *wsURL != "" {
		cfg.WSURL = *wsURL
	}
	if *token != "" {
		cfg.AccessToken = *token
	}
	if *hotReload != "" {
		cfg.HotReloadFile = *hotReload
	}
	if *mode != "" {
		cfg.LoginMode = *mode
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := onebot.NewApp(cfg, log.Default()).Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("openwechat-onebot 退出: %v", err)
	}
}
