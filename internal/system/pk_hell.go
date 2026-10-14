package system

import (
	"fmt"
	"math/rand"
	"time"

	coresys "github.com/l1jgo/server/internal/core/system"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
)

// PK 地獄（Java: L1PcInstance.beginHell / endHell、L1PcHellMonitor、C_Restart）。
// PK 次數達門檻時關進地圖 666 倒數；死亡不倒數；重登／重生續算；倒數結束回火田村。
const (
	HellMapID int16 = 666
	HellX     int32 = 32701
	HellY     int32 = 32777

	hellTicksPerSecond = 5 // 200ms tick
)

// 火田村回城點（Java L1TownLocation.GETBACK_LOC_ORCISH_FOREST，map 4）。
var orcishForestGetback = [][2]int32{
	{32750, 32435}, {32745, 32447}, {32738, 32452}, {32741, 32436}, {32749, 32446},
}

// HellTimeForPKCount Java: PK ≤ 門檻 → 300 秒；否則 300*(PK-門檻)+300。
func HellTimeForPKCount(pkCount, punish int32) int32 {
	if pkCount <= punish {
		return 300
	}
	return 300*(pkCount-punish) + 300
}

// Phase PK 地獄倒數屬一般更新階段。
func (s *PvPSystem) Phase() coresys.Phase { return coresys.PhaseUpdate }

// Update 每秒扣地獄秒數（Java L1PcHellMonitor 每 1000ms）；死亡中不倒數。
func (s *PvPSystem) Update(_ time.Duration) {
	if s.deps == nil || s.deps.World == nil {
		return
	}
	var finished []*world.PlayerInfo
	s.deps.World.AllPlayers(func(p *world.PlayerInfo) {
		if p.HellTime <= 0 || p.Dead {
			return
		}
		p.HellTickAcc++
		if p.HellTickAcc < hellTicksPerSecond {
			return
		}
		p.HellTickAcc = 0
		p.HellTime--
		if p.HellTime <= 0 {
			finished = append(finished, p)
		}
	})
	for _, p := range finished {
		s.EndHell(p)
	}
}

// BeginHell 實作 handler.PvPManager。first=true 為達門檻當下（設定秒數、552），
// false 為重登／重生續算（637）。不在地獄地圖時強制傳送。
func (s *PvPSystem) BeginHell(player *world.PlayerInfo, first bool) {
	if player == nil {
		return
	}
	if first {
		punish := int32(10)
		if s.deps != nil && s.deps.Scripting != nil {
			punish = s.deps.Scripting.GetPKThresholds().Punish
		}
		player.HellTime = HellTimeForPKCount(player.PKCount, punish)
		player.HellTickAcc = 0
	}
	if player.HellTime <= 0 {
		return
	}
	player.Dirty = true
	if player.MapID != HellMapID {
		s.teleportPlayer(player, HellX, HellY, HellMapID, 5)
	}
	if player.Session == nil {
		return
	}
	if first {
		// 你的 PK 次數達到 %0，被關進地獄；必須在此反省 %1 分鐘。
		handler.SendRedMessage(player.Session, 552, fmt.Sprintf("%d", player.PKCount), fmt.Sprintf("%d", player.HellTime/60))
	} else {
		// 你必須在此停留 %0 秒。
		handler.SendRedMessage(player.Session, 637, fmt.Sprintf("%d", player.HellTime))
	}
}

// EndHell 倒數結束：清除秒數並傳回火田村（Java endHell → TOWNID_ORCISH_FOREST）。
func (s *PvPSystem) EndHell(player *world.PlayerInfo) {
	if player == nil {
		return
	}
	player.HellTime = 0
	player.HellTickAcc = 0
	player.Dirty = true
	loc := orcishForestGetback[rand.Intn(len(orcishForestGetback))]
	s.teleportPlayer(player, loc[0], loc[1], 4, 5)
}

// teleportPlayer 傳送玩家（測試可替換 s.teleport）。
func (s *PvPSystem) teleportPlayer(player *world.PlayerInfo, x, y int32, mapID, heading int16) {
	if s.teleport != nil {
		s.teleport(player.Session, player, x, y, mapID, heading)
		return
	}
	if player.Session == nil {
		return
	}
	handler.TeleportPlayer(player.Session, player, x, y, mapID, heading, s.deps)
}

// pvpTeleportFunc 測試注入用的傳送函式型別。
type pvpTeleportFunc func(sess *net.Session, player *world.PlayerInfo, x, y int32, mapID, heading int16)
