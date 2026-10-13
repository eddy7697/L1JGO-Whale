package system

import (
	"testing"
	"time"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

func TestWeatherDefaultsToJavaClearAndDisabledPreservesWorldWeather(t *testing.T) {
	ws := world.NewState()
	if ws.Weather != 4 {
		t.Fatalf("Java L1World 初始天氣應為 4，got=%d", ws.Weather)
	}
	p := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001})
	s := NewWeatherSystem(ws, config.WorldConfig{WeatherInterval: 1})
	for _, weather := range []byte{4, 19, 3, 16, 0} {
		ws.Weather = weather // 模擬 GM 手動改天氣；關閉自動天氣不應覆寫。
		for tick := 0; tick < 36000; tick++ {
			s.Update(200 * time.Millisecond)
		}
		if ws.Weather != weather || len(drainSkillTestPackets(p.Session)) != 0 {
			t.Fatalf("停用自動天氣時不應改變或廣播天氣: want=%d got=%d", weather, ws.Weather)
		}
		// 登入與死亡重返世界沿用相同的目前天氣封包。
		handler.SendWeather(p.Session, ws.Weather)
		packets := drainSkillTestPackets(p.Session)
		if len(packets) != 1 || packets[0][0] != packet.S_OPCODE_WEATHER || packets[0][1] != weather {
			t.Fatalf("天氣未依 Java S_Weather 傳送: %x", packets)
		}
	}
}

func TestOptInWeatherHonorsConfiguredInterval(t *testing.T) {
	for _, interval := range []int{1, 3, 0, -1} {
		ws := world.NewState()
		p := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, MapID: 4})
		other := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, MapID: 2005, ShowID: 3})
		s := NewWeatherSystem(ws, config.WorldConfig{WeatherEnabled: true, WeatherInterval: interval})
		wantInterval := interval
		if wantInterval <= 0 {
			wantInterval = 100
		}
		for tick := 1; tick <= wantInterval*2; tick++ {
			s.Update(200 * time.Millisecond)
			for _, recipient := range []*world.PlayerInfo{p, other} {
				packets := drainSkillTestPackets(recipient.Session)
				if tick%wantInterval != 0 {
					if len(packets) != 0 || (tick < wantInterval && ws.Weather != 4) {
						t.Fatalf("間隔前不可改天氣: interval=%d tick=%d packets=%x", interval, tick, packets)
					}
					continue
				}
				if len(packets) != 1 || len(packets[0]) != 8 || packets[0][0] != packet.S_OPCODE_WEATHER || packets[0][1] != ws.Weather {
					t.Fatalf("啟用後應按間隔廣播目前天氣: interval=%d tick=%d packets=%x", interval, tick, packets)
				}
			}
		}
	}
}
