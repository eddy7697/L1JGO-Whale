package handler

import (
	"strconv"
	"testing"

	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

func TestGMWeatherSupportsJavaClearSnowRainAndStopRain(t *testing.T) {
	ws := world.NewState()
	gm := &world.PlayerInfo{SessionID: 1, Session: newHandlerTestSession(t, 1), CharID: 1001}
	other := &world.PlayerInfo{SessionID: 2, Session: newHandlerTestSession(t, 2), CharID: 1002, MapID: 2005}
	ws.AddPlayer(gm)
	ws.AddPlayer(other)
	for _, weather := range []int{0, 1, 2, 3, 4, 16, 17, 18, 19} {
		gmWeather(gm.Session, gm, []string{strconv.Itoa(weather)}, &Deps{World: ws})
		if ws.Weather != byte(weather) {
			t.Fatalf("GM 無法設定 Java 天氣值 %d", weather)
		}
		for _, p := range []*world.PlayerInfo{gm, other} {
			count := 0
			for _, pkt := range drainHandlerTestPackets(p.Session) {
				if pkt[0] == packet.S_OPCODE_WEATHER {
					count++
					if len(pkt) != 8 || pkt[1] != byte(weather) {
						t.Fatalf("錯誤天氣封包: %x", pkt)
					}
				}
			}
			if count != 1 {
				t.Fatalf("天氣須廣播全世界一次: player=%d count=%d", p.CharID, count)
			}
		}
	}
	for _, arg := range []string{"-1", "5", "15", "20", "256", "rain"} {
		gmWeather(gm.Session, gm, []string{arg}, &Deps{World: ws})
		drainHandlerTestPackets(gm.Session)
		if ws.Weather != 19 || len(drainHandlerTestPackets(other.Session)) != 0 {
			t.Fatalf("無效天氣 %q 不應改世界狀態", arg)
		}
	}
}
