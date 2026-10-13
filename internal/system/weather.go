package system

import (
	"time"

	"github.com/l1jgo/server/internal/config"
	coresys "github.com/l1jgo/server/internal/core/system"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

// WeatherSystem 為可選的 Go 自動天氣；預設關閉以對齊 Java。
// 關閉時保留世界初始晴天或 GM 設定，不因登入、首 tick 或換小時改變。
type WeatherSystem struct {
	world    *world.State
	enabled  bool
	interval int
	ticks    int
}

func NewWeatherSystem(ws *world.State, cfg config.WorldConfig) *WeatherSystem {
	interval := cfg.WeatherInterval
	if interval <= 0 {
		interval = 100
	}
	return &WeatherSystem{world: ws, enabled: cfg.WeatherEnabled, interval: interval}
}

func (s *WeatherSystem) Phase() coresys.Phase { return coresys.PhasePostUpdate }

func (s *WeatherSystem) Update(_ time.Duration) {
	if !s.enabled {
		return
	}
	s.ticks++
	if s.ticks < s.interval {
		return
	}
	s.ticks = 0
	s.world.RandomizeWeather()
	s.world.AllPlayers(func(p *world.PlayerInfo) {
		handler.SendWeather(p.Session, s.world.Weather)
	})
}
