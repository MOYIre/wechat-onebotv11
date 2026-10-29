package onebot

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"
)

// Config openwechat-onebot 的运行配置
type Config struct {
	// WSURL AstrBot aiocqhttp 反向 WebSocket 地址, 如 ws://127.0.0.1:6199/ws
	WSURL string `json:"ws_url"`
	// AccessToken 鉴权 token, 需与 AstrBot 侧保持一致, 为空则不鉴权
	AccessToken string `json:"access_token"`
	// HotReloadFile 热登录凭据文件路径, 为空表示每次扫码登录
	HotReloadFile string `json:"hot_reload_file"`
	// LoginMode 登录模式: desktop(默认) 或 normal
	LoginMode string `json:"login_mode"`
	// MaxReconnectInterval 断线重连的最大间隔
	MaxReconnectIntervalSeconds int `json:"max_reconnect_interval_seconds"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		WSURL:                       "ws://127.0.0.1:6199/ws",
		HotReloadFile:               "hotreload.json",
		LoginMode:                   "desktop",
		MaxReconnectIntervalSeconds: 30,
	}
}

// LoadConfig 从 JSON 文件加载配置, 不存在的字段使用默认值
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate 检查启动所需配置
func (c Config) Validate() error {
	u, err := url.Parse(c.WSURL)
	if err != nil || u.Host == "" || (u.Scheme != "ws" && u.Scheme != "wss") {
		return fmt.Errorf("ws_url must be a ws:// or wss:// URL")
	}
	if c.LoginMode != "" && c.LoginMode != "desktop" && c.LoginMode != "normal" {
		return fmt.Errorf("login_mode must be desktop or normal")
	}
	return nil
}

func (c *Config) reconnectInterval() time.Duration {
	max := c.MaxReconnectIntervalSeconds
	if max <= 0 {
		max = 30
	}
	return time.Duration(max) * time.Second
}

func (c *Config) mode() string {
	if c.LoginMode == "normal" {
		return "normal"
	}
	return "desktop"
}
