package system

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// hasRedMessage 檢查是否送出指定 ID 的 S_RedMessage（opcode 105，格式同 S_ServerMessage）。
func hasRedMessage(packets [][]byte, msgID uint16) bool {
	for _, pkt := range packets {
		if len(pkt) >= 3 && pkt[0] == packet.S_OPCODE_REDMESSAGE &&
			binary.LittleEndian.Uint16(pkt[1:3]) == msgID {
			return true
		}
	}
	return false
}

type pvpTeleportCall struct {
	x, y    int32
	mapID   int16
	heading int16
}

func capturePvPTeleport(s *PvPSystem) *[]pvpTeleportCall {
	calls := &[]pvpTeleportCall{}
	s.teleport = func(_ *l1net.Session, player *world.PlayerInfo, x, y int32, mapID, heading int16) {
		*calls = append(*calls, pvpTeleportCall{x, y, mapID, heading})
		player.X, player.Y, player.MapID = x, y, mapID
	}
	return calls
}

// ==================== 1-22 PK 地獄 ====================

func TestHellTimeForPKCountLikeJava(t *testing.T) {
	cases := []struct{ pk, want int32 }{{10, 300}, {9, 300}, {11, 600}, {12, 900}, {20, 3300}}
	for _, c := range cases {
		if got := HellTimeForPKCount(c.pk, 10); got != c.want {
			t.Fatalf("PK=%d 地獄秒數=%d want %d", c.pk, got, c.want)
		}
	}
}

func TestBeginHellFirstTeleportsAndNotifiesLikeJava(t *testing.T) {
	ws := world.NewState()
	killer := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "pker",
		X: 100, Y: 100, MapID: 4, PKCount: 12,
	})
	s := newPvPShowIDTestSystem(t, ws)
	calls := capturePvPTeleport(s)

	s.BeginHell(killer, true)

	if killer.HellTime != 900 {
		t.Fatalf("HellTime=%d want 900", killer.HellTime)
	}
	if len(*calls) != 1 || (*calls)[0] != (pvpTeleportCall{HellX, HellY, HellMapID, 5}) {
		t.Fatalf("應傳送至地獄 666 (32701,32777) heading 5，got %+v", *calls)
	}
	if !hasRedMessage(drainSkillTestPackets(killer.Session), 552) {
		t.Fatalf("首次入獄應送 S_RedMessage 552")
	}
	if !killer.Dirty {
		t.Fatalf("入獄應標記存檔")
	}
}

func TestBeginHellResumeKeepsTimeAndSkipsTeleportInsideHell(t *testing.T) {
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "jailed",
		X: HellX, Y: HellY, MapID: HellMapID, PKCount: 12, HellTime: 123,
	})
	s := newPvPShowIDTestSystem(t, ws)
	calls := capturePvPTeleport(s)

	s.BeginHell(p, false)

	if p.HellTime != 123 {
		t.Fatalf("續算不應重設秒數，got %d", p.HellTime)
	}
	if len(*calls) != 0 {
		t.Fatalf("已在地獄不應再傳送，got %+v", *calls)
	}
	pk := drainSkillTestPackets(p.Session)
	if !hasRedMessage(pk, 637) || hasRedMessage(pk, 552) {
		t.Fatalf("續算應送 637 而非 552")
	}
}

func TestBeginHellResumeOutsideHellTeleportsBack(t *testing.T) {
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "escaped",
		X: 100, Y: 100, MapID: 4, HellTime: 50,
	})
	s := newPvPShowIDTestSystem(t, ws)
	calls := capturePvPTeleport(s)

	s.BeginHell(p, false)

	if len(*calls) != 1 || (*calls)[0].mapID != HellMapID {
		t.Fatalf("登入時不在地獄應強制傳回，got %+v", *calls)
	}
}

func TestHellCountdownEverySecondSkipsDeadAndReleasesToOrcishForest(t *testing.T) {
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "jailed",
		X: HellX, Y: HellY, MapID: HellMapID, HellTime: 2,
	})
	dead := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, Name: "dead",
		X: HellX, Y: HellY, MapID: HellMapID, HellTime: 2, Dead: true,
	})
	s := newPvPShowIDTestSystem(t, ws)
	calls := capturePvPTeleport(s)

	for i := 0; i < hellTicksPerSecond-1; i++ {
		s.Update(200 * time.Millisecond)
	}
	if p.HellTime != 2 {
		t.Fatalf("未滿 1 秒不應扣秒，got %d", p.HellTime)
	}
	s.Update(200 * time.Millisecond)
	if p.HellTime != 1 {
		t.Fatalf("滿 5 tick 應扣 1 秒，got %d", p.HellTime)
	}
	if dead.HellTime != 2 {
		t.Fatalf("死亡中不應倒數，got %d", dead.HellTime)
	}
	for i := 0; i < hellTicksPerSecond; i++ {
		s.Update(200 * time.Millisecond)
	}
	if p.HellTime != 0 {
		t.Fatalf("倒數結束 HellTime 應為 0，got %d", p.HellTime)
	}
	if len(*calls) != 1 || (*calls)[0].mapID != 4 || (*calls)[0].heading != 5 {
		t.Fatalf("結束應傳回火田村 map 4，got %+v", *calls)
	}
	found := false
	for _, loc := range orcishForestGetback {
		if loc[0] == (*calls)[0].x && loc[1] == (*calls)[0].y {
			found = true
		}
	}
	if !found {
		t.Fatalf("座標不在火田村回城點清單 %+v", (*calls)[0])
	}
	if p.MapID != 4 {
		t.Fatalf("玩家地圖應為 4，got %d", p.MapID)
	}
}

func TestProcessPKKillReachingThresholdBeginsHell(t *testing.T) {
	ws := world.NewState()
	killer := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "killer",
		X: 100, Y: 100, MapID: 900, ShowID: 100, Lawful: 1000, PKCount: 9, Level: 50,
	})
	victim := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, Name: "victim",
		X: 101, Y: 100, MapID: 900, ShowID: 100, Lawful: 0,
	})
	s := newPvPShowIDTestSystem(t, ws)
	calls := capturePvPTeleport(s)

	s.processPKKill(killer, victim)

	if killer.PKCount != 10 {
		t.Fatalf("PKCount=%d want 10", killer.PKCount)
	}
	if killer.HellTime != 300 {
		t.Fatalf("達門檻應入獄 300 秒，got %d", killer.HellTime)
	}
	if len(*calls) != 1 || (*calls)[0].mapID != HellMapID {
		t.Fatalf("應傳送至地獄，got %+v", *calls)
	}
	pk := drainSkillTestPackets(killer.Session)
	if !hasRedMessage(pk, 552) || hasRedMessage(pk, 551) {
		t.Fatalf("達門檻應送 552、不送 551")
	}
}

func TestProcessPKKillBelowThresholdOnlyWarns(t *testing.T) {
	ws := world.NewState()
	killer := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "killer",
		X: 100, Y: 100, MapID: 900, ShowID: 100, Lawful: 1000, PKCount: 4, Level: 50,
	})
	victim := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, Name: "victim",
		X: 101, Y: 100, MapID: 900, ShowID: 100, Lawful: 0,
	})
	s := newPvPShowIDTestSystem(t, ws)
	calls := capturePvPTeleport(s)

	s.processPKKill(killer, victim)

	if killer.HellTime != 0 || len(*calls) != 0 {
		t.Fatalf("未達門檻不應入獄 HellTime=%d calls=%+v", killer.HellTime, *calls)
	}
	if !hasRedMessage(drainSkillTestPackets(killer.Session), 551) {
		t.Fatalf("5 次應送警告 551")
	}
}

type hellRestartPvPSpy struct {
	fakePvPManager
	calls []bool
}

func (s *hellRestartPvPSpy) BeginHell(_ *world.PlayerInfo, first bool) {
	s.calls = append(s.calls, first)
}

func TestRestartInHellRespawnsAtHellLikeJava(t *testing.T) {
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "jailed",
		X: HellX + 3, Y: HellY, MapID: HellMapID, HellTime: 80, Dead: true, Level: 30, MaxHP: 300, MaxMP: 100,
		Known: world.NewKnownEntities(),
	})
	spy := &hellRestartPvPSpy{}
	cfg := &config.Config{}
	cfg.Gameplay.InitialFood = 40
	ds := NewDeathSystem(&handler.Deps{
		World: ws, Config: cfg, Log: zap.NewNop(), PvP: spy,
		Scripting: newPvPShowIDTestSystem(t, ws).deps.Scripting,
	})

	ds.ProcessRestart(p.Session, p)

	if p.Dead {
		t.Fatalf("重生後不應仍為死亡")
	}
	if p.MapID != HellMapID || p.X != HellX || p.Y != HellY {
		t.Fatalf("地獄中重生應回 666 (32701,32777)，got map=%d (%d,%d)", p.MapID, p.X, p.Y)
	}
	if len(spy.calls) != 1 || spy.calls[0] {
		t.Fatalf("重生應呼叫 BeginHell(false)，got %+v", spy.calls)
	}
}

func TestRestartOutsideHellUsesNormalGetback(t *testing.T) {
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "free",
		X: 100, Y: 100, MapID: 4, Dead: true, Level: 30, MaxHP: 300, MaxMP: 100,
		Known: world.NewKnownEntities(),
	})
	spy := &hellRestartPvPSpy{}
	cfg := &config.Config{}
	ds := NewDeathSystem(&handler.Deps{
		World: ws, Config: cfg, Log: zap.NewNop(), PvP: spy,
		Scripting: newPvPShowIDTestSystem(t, ws).deps.Scripting,
	})

	ds.ProcessRestart(p.Session, p)

	if p.MapID == HellMapID || len(spy.calls) != 0 {
		t.Fatalf("非地獄玩家不應進地獄 map=%d calls=%+v", p.MapID, spy.calls)
	}
}

// ==================== 1-27 隱身解除延遲 ====================

func TestCastableWithInvisLikeJava(t *testing.T) {
	if !castableWithInvisLikeJava(13) || !castableWithInvisLikeJava(60) || !castableWithInvisLikeJava(219) {
		t.Fatalf("13/60/219 應在 CAST_WITH_INVIS")
	}
	if castableWithInvisLikeJava(10) || castableWithInvisLikeJava(87) || castableWithInvisLikeJava(132) {
		t.Fatalf("10/87/132 不應在 CAST_WITH_INVIS")
	}
}

func TestInvisDelayTicksDownEachBuffTick(t *testing.T) {
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, InvisDelayTicks: InvisDelayTicks,
	})
	s := &SkillSystem{deps: &handler.Deps{World: ws, Log: zap.NewNop()}}
	for i := 0; i < InvisDelayTicks; i++ {
		if p.InvisDelayTicks <= 0 {
			t.Fatalf("第 %d tick 延遲提前結束", i)
		}
		s.tickPlayerBuffs(p)
	}
	if p.InvisDelayTicks != 0 {
		t.Fatalf("15 tick 後應歸零，got %d", p.InvisDelayTicks)
	}
}

func TestMeleeAttackBlockedDuringInvisDelay(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "attacker",
		X: 100, Y: 100, MapID: 900, ShowID: 100, HP: 100, MaxHP: 100, Str: 20, Dex: 20, Level: 50,
		InvisDelayTicks: 3,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, Name: "target",
		X: 101, Y: 100, MapID: 900, ShowID: 100, HP: 1000, MaxHP: 1000,
	})
	pvp := &fakePvPManager{}
	s := newCombatLOSTestSystem(t, ws, pvp)

	s.processMeleeAttack(player.SessionID, target.CharID)
	if pvp.meleeCalls != 0 {
		t.Fatalf("隱身延遲中不應攻擊，got %d", pvp.meleeCalls)
	}

	player.InvisDelayTicks = 0
	s.processMeleeAttack(player.SessionID, target.CharID)
	if pvp.meleeCalls != 1 {
		t.Fatalf("延遲結束後應可攻擊，got %d", pvp.meleeCalls)
	}
}

func TestDetectionSelfRevealStartsInvisDelay(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "hidden",
		X: 100, Y: 100, MapID: 900, ShowID: 100, Invisible: true,
	})
	player.AddBuff(&world.ActiveBuff{SkillID: 60, TicksLeft: 100, SetInvisible: true})
	s := newSkillTestSystem(t, ws)

	s.executeSelfSkill(player.Session, player, &data.SkillInfo{SkillID: 13, Target: "none"})

	if player.InvisDelayTicks != InvisDelayTicks {
		t.Fatalf("無所遁形術揭露自己後應鎖 %d tick，got %d", InvisDelayTicks, player.InvisDelayTicks)
	}
	if player.IsInvisible() {
		t.Fatalf("揭露後不應仍隱身")
	}
}

func TestUnequipInvisCloakStartsInvisDelayOnlyWhenInvisible(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "cloaked",
		X: 100, Y: 100, MapID: 900, ShowID: 100, Inv: world.NewInventory(),
	})
	es := NewEquipSystem(&handler.Deps{World: ws, Items: testItemTable(t), Log: zap.NewNop()})

	cloak := player.Inv.AddItem(20077, 1, "隱身斗篷", 0, 10, false, 1)
	cloak.Equipped = true
	player.Equip.Set(world.SlotCloak, cloak)
	player.Invisible = true
	es.UnequipSlot(player.Session, player, world.SlotCloak)
	if player.InvisDelayTicks != InvisDelayTicks {
		t.Fatalf("隱身中脫下斗篷應鎖 %d tick，got %d", InvisDelayTicks, player.InvisDelayTicks)
	}
	if player.Invisible {
		t.Fatalf("脫下斗篷後不應隱身")
	}

	player.InvisDelayTicks = 0
	cloak2 := player.Inv.AddItem(20077, 1, "隱身斗篷", 0, 10, false, 1)
	cloak2.Equipped = true
	player.Equip.Set(world.SlotCloak, cloak2)
	player.Invisible = false
	es.UnequipSlot(player.Session, player, world.SlotCloak)
	if player.InvisDelayTicks != 0 {
		t.Fatalf("非隱身狀態脫下斗篷不應鎖定，got %d", player.InvisDelayTicks)
	}
}

// ==================== 1-26 地面物品擁有者保護 ====================

func newGroundOwnerTestWorld(t *testing.T) (*world.State, *world.PlayerInfo, *world.PlayerInfo, *ItemGroundSystem) {
	t.Helper()
	ws := world.NewState()
	owner := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "owner",
		X: 10, Y: 10, MapID: 4, ShowID: 7, Str: 20, Con: 20, Inv: world.NewInventory(), Known: world.NewKnownEntities(),
	})
	other := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, Name: "other",
		X: 11, Y: 10, MapID: 4, ShowID: 7, Str: 20, Con: 20, Inv: world.NewInventory(), Known: world.NewKnownEntities(),
	})
	sys := NewItemGroundSystem(&handler.Deps{World: ws, Items: testItemTable(t), Log: zap.NewNop()})
	return ws, owner, other, sys
}

func TestPickupRejectsOwnerProtectedItemLikeJava(t *testing.T) {
	ws, owner, other, sys := newGroundOwnerTestWorld(t)
	g := &world.GroundItem{ID: world.NextGroundItemID(), ItemID: 40308, Count: 5, Name: "金幣",
		X: 10, Y: 10, MapID: 4, ShowID: 7, OwnerID: owner.CharID, OwnerTicks: 50, TTL: 100}
	ws.AddGroundItem(g)

	sys.PickupItem(other.Session, other, g.ID)
	if ws.GetGroundItem(g.ID) == nil {
		t.Fatalf("保護期間他人不應撿起")
	}
	if !hasServerMessage(drainSkillTestPackets(other.Session), 623) {
		t.Fatalf("應送 S_ServerMessage 623")
	}

	sys.PickupItem(owner.Session, owner, g.ID)
	if ws.GetGroundItem(g.ID) != nil {
		t.Fatalf("擁有者本人應可撿起")
	}
}

func TestGroundOwnerProtectionExpiresAfterTicks(t *testing.T) {
	ws, owner, other, sys := newGroundOwnerTestWorld(t)
	g := &world.GroundItem{ID: world.NextGroundItemID(), ItemID: 40308, Count: 5, Name: "金幣",
		X: 10, Y: 10, MapID: 4, ShowID: 7, OwnerID: owner.CharID, OwnerTicks: 3, TTL: 100}
	ws.AddGroundItem(g)

	for i := 0; i < 3; i++ {
		ws.TickGroundItems()
	}
	if g.OwnerID != 0 {
		t.Fatalf("保護到期應清除 OwnerID，got %d", g.OwnerID)
	}
	sys.PickupItem(other.Session, other, g.ID)
	if ws.GetGroundItem(g.ID) != nil {
		t.Fatalf("保護到期後他人應可撿起")
	}
}

func TestPlayerDropHasNoOwnerProtectionLikeJava(t *testing.T) {
	ws, owner, _, sys := newGroundOwnerTestWorld(t)
	item := owner.Inv.AddItem(40308, 10, "金幣", 101, 0, true, 1)

	sys.DropItem(owner.Session, owner, item.ObjectID, 10)

	ground := ws.GetNearbyGroundItems(owner.X, owner.Y, owner.MapID)
	if len(ground) != 1 {
		t.Fatalf("應有一個地上物，got %d", len(ground))
	}
	if ground[0].OwnerID != 0 || ground[0].OwnerTicks != 0 {
		t.Fatalf("玩家丟棄不應有擁有者保護，got owner=%d ticks=%d", ground[0].OwnerID, ground[0].OwnerTicks)
	}
}

func TestPickupIgnoredWhileInvisibleOrInvisDelay(t *testing.T) {
	ws, _, other, sys := newGroundOwnerTestWorld(t)
	g := &world.GroundItem{ID: world.NextGroundItemID(), ItemID: 40308, Count: 5, Name: "金幣",
		X: 10, Y: 10, MapID: 4, ShowID: 7, TTL: 100}
	ws.AddGroundItem(g)

	other.InvisDelayTicks = 2
	sys.PickupItem(other.Session, other, g.ID)
	if ws.GetGroundItem(g.ID) == nil {
		t.Fatalf("隱身延遲中不應撿起")
	}
	other.InvisDelayTicks = 0
	other.Invisible = true
	sys.PickupItem(other.Session, other, g.ID)
	if ws.GetGroundItem(g.ID) == nil {
		t.Fatalf("隱身中不應撿起")
	}
}

func TestFishingDropsFishWithOwnerProtectionWhenBagFull(t *testing.T) {
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "fisher",
		X: 10, Y: 10, MapID: 4, ShowID: 7, Str: 20, Con: 20, Inv: world.NewInventory(), Known: world.NewKnownEntities(),
		Fishing: true,
	})
	for i := 0; i < world.MaxInventorySize; i++ {
		p.Inv.AddItem(int32(50000+i), 1, "filler", 0, 0, false, 1)
	}
	fs := NewFishingSystem(&handler.Deps{World: ws, Items: newPvPShowIDTestSystem(t, ws).deps.Items, Log: zap.NewNop()})

	if !fs.dropFishIfCannotCarry(p, 41298) {
		t.Fatalf("背包已滿應掉落")
	}
	if p.Fishing {
		t.Fatalf("掉落後應停止釣魚")
	}
	ground := ws.GetNearbyGroundItems(p.X, p.Y, p.MapID)
	if len(ground) != 1 || ground[0].ItemID != 41298 {
		t.Fatalf("應有一條魚在地上，got %+v", ground)
	}
	if ground[0].OwnerID != p.CharID || ground[0].OwnerTicks != fishOwnerTicks {
		t.Fatalf("魚應有 10 秒擁有者保護，got owner=%d ticks=%d", ground[0].OwnerID, ground[0].OwnerTicks)
	}

	free := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, Name: "free",
		X: 10, Y: 10, MapID: 4, ShowID: 7, Str: 20, Con: 20, Inv: world.NewInventory(), Known: world.NewKnownEntities(),
		Fishing: true,
	})
	if fs.dropFishIfCannotCarry(free, 41298) {
		t.Fatalf("背包有空間不應掉落")
	}
}
