package system

import (
	"testing"
	"time"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// 測試用城堡表：肯特（1）與亞丁（7），座標取自 data/yaml/castles.yaml。
func testCastleTable() *data.CastleTable {
	return data.NewCastleTable([]*data.CastleConfig{
		{
			ID:      1,
			Name:    "肯特",
			WarArea: data.CastleArea{X1: 33089, X2: 33219, Y1: 32717, Y2: 32827, Map: 4},
			Tower:   data.CastlePoint{X: 33168, Y: 32779, Map: 4},
			Getback: []data.CastlePoint{
				{X: 33048, Y: 32750, Map: 4},
				{X: 33059, Y: 32768, Map: 4},
			},
		},
		{
			ID:      7,
			Name:    "亞丁",
			WarArea: data.CastleArea{X1: 34007, X2: 34162, Y1: 33172, Y2: 33332, Map: 4},
			Tower:   data.CastlePoint{X: 34090, Y: 33260, Map: 4},
			SubTowers: []data.SubTower{
				{X: 34057, Y: 33291, Map: 4},
				{X: 34123, Y: 33291, Map: 4},
				{X: 34057, Y: 33230, Map: 4},
				{X: 34123, Y: 33230, Map: 4},
			},
		},
	}, nil)
}

func testWarNpcTable() *data.NpcTable {
	tower := func(id int32, name string) *data.NpcTemplate {
		return &data.NpcTemplate{NpcID: id, Name: name, Impl: "L1Tower", GfxID: 1672, HP: 5000, Level: 1}
	}
	return data.NewNpcTable(
		tower(81111, "守護者之塔"),
		tower(81189, "亞丁主塔"),
		tower(81190, "子塔1"),
		tower(81191, "子塔2"),
		tower(81192, "子塔3"),
		tower(81193, "子塔4"),
		&data.NpcTemplate{NpcID: 81125, Name: "王冠", Impl: "L1Crown", GfxID: 1673, HP: 1},
	)
}

func newTestCastleSystem(t *testing.T) *CastleSystem {
	t.Helper()
	ws := world.NewState()
	return &CastleSystem{
		deps: &handler.Deps{
			World:   ws,
			Castles: testCastleTable(),
			Npcs:    testWarNpcTable(),
			Log:     zap.NewNop(),
		},
		castles: map[int32]*handler.CastleInfo{
			1: {CastleID: 1, Name: "肯特"},
			7: {CastleID: 7, Name: "亞丁"},
		},
		warEnd: map[int32]time.Time{},
	}
}

func addWarNpc(ws *world.State, npcID int32, impl string, x, y int32, mapID int16, dead bool) *world.NpcInfo {
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: npcID,
		Impl:  impl,
		X:     x,
		Y:     y,
		MapID: mapID,
		HP:    1,
		MaxHP: 1,
	}
	ws.AddNpc(npc)
	if dead {
		npc.Dead = true
		npc.HP = 0
		npc.DeleteTimer = 50
		ws.NpcDied(npc)
	}
	return npc
}

func towersInArea(sys *CastleSystem, castleID int32) []*world.NpcInfo {
	var out []*world.NpcInfo
	for _, n := range sys.deps.World.NpcList() {
		if n.Impl == "L1Tower" && sys.deps.Castles.CheckInWarArea(castleID, n.X, n.Y, n.MapID) {
			out = append(out, n)
		}
	}
	return out
}

func TestSpawnTowerReplacesDeadAndAliveTowers(t *testing.T) {
	sys := newTestCastleSystem(t)
	ws := sys.deps.World

	deadTower := addWarNpc(ws, 81111, "L1Tower", 33168, 32779, 4, true)
	strayTower := addWarNpc(ws, 81111, "L1Tower", 33150, 32760, 4, false)
	// 另一座城堡的塔不受影響
	otherTower := addWarNpc(ws, 81111, "L1Tower", 34090, 33260, 4, false)

	sys.SpawnTower(1)

	towers := towersInArea(sys, 1)
	if len(towers) != 1 {
		t.Fatalf("肯特戰爭區守護塔數量錯誤：got %d want 1", len(towers))
	}
	tower := towers[0]
	if tower.ID == deadTower.ID || tower.ID == strayTower.ID {
		t.Fatalf("舊塔未被刪除：id=%d", tower.ID)
	}
	if tower.NpcID != 81111 || tower.Dead || tower.HP != 5000 || tower.MaxHP != 5000 {
		t.Fatalf("新塔狀態錯誤：npc=%d dead=%v hp=%d/%d", tower.NpcID, tower.Dead, tower.HP, tower.MaxHP)
	}
	if tower.X != 33168 || tower.Y != 32779 || tower.MapID != 4 {
		t.Fatalf("新塔座標錯誤：(%d,%d,%d)", tower.X, tower.Y, tower.MapID)
	}
	if ws.GetNpc(deadTower.ID) != nil || ws.GetNpc(strayTower.ID) != nil {
		t.Fatal("舊塔仍留在世界中")
	}
	if ws.GetNpc(otherTower.ID) == nil {
		t.Fatal("其他城堡的塔被誤刪")
	}
}

func TestSpawnTowerAdenSpawnsMainAndSubTowers(t *testing.T) {
	sys := newTestCastleSystem(t)
	ws := sys.deps.World

	addWarNpc(ws, 81189, "L1Tower", 34090, 33260, 4, true)
	addWarNpc(ws, 81190, "L1Tower", 34057, 33291, 4, true)

	sys.SpawnTower(7)

	towers := towersInArea(sys, 7)
	if len(towers) != 5 {
		t.Fatalf("亞丁守護塔數量錯誤：got %d want 5", len(towers))
	}
	byNpc := map[int32]*world.NpcInfo{}
	for _, n := range towers {
		if n.Dead {
			t.Fatalf("亞丁塔 %d 重生後仍為死亡", n.NpcID)
		}
		byNpc[n.NpcID] = n
	}
	want := map[int32][2]int32{
		81189: {34090, 33260},
		81190: {34057, 33291},
		81191: {34123, 33291},
		81192: {34057, 33230},
		81193: {34123, 33230},
	}
	for npcID, pos := range want {
		n := byNpc[npcID]
		if n == nil {
			t.Fatalf("缺少亞丁塔 %d", npcID)
		}
		if n.X != pos[0] || n.Y != pos[1] {
			t.Fatalf("亞丁塔 %d 座標錯誤：(%d,%d) want (%d,%d)", npcID, n.X, n.Y, pos[0], pos[1])
		}
	}
}

func TestEndWarClearsCrownRespawnsTowerAndUsesConfiguredInterval(t *testing.T) {
	sys := newTestCastleSystem(t)
	sys.deps.Config = &config.Config{Gameplay: config.GameplayConfig{
		WarTime:     90 * time.Minute,
		WarInterval: 4 * 24 * time.Hour,
	}}
	ws := sys.deps.World

	ci := sys.castles[1]
	ci.IsWar = true
	warStart := time.Date(2026, 10, 10, 20, 0, 0, 0, time.UTC)
	ci.WarTime = warStart
	sys.warEnd[1] = warStart.Add(90 * time.Minute)

	crown := addWarNpc(ws, 81125, "L1Crown", 33168, 32779, 4, false)
	deadTower := addWarNpc(ws, 81111, "L1Tower", 33168, 32779, 4, true)

	sys.endWar(1, ci)

	if ci.IsWar {
		t.Fatal("攻城結束後 IsWar 仍為 true")
	}
	if ws.GetNpc(crown.ID) != nil {
		t.Fatal("攻城結束後王冠未被刪除")
	}
	if ws.GetNpc(deadTower.ID) != nil {
		t.Fatal("攻城結束後舊塔未被刪除")
	}
	towers := towersInArea(sys, 1)
	if len(towers) != 1 || towers[0].Dead {
		t.Fatalf("攻城結束後守護塔未重生：count=%d", len(towers))
	}

	wantNext := warStart.Add(4 * 24 * time.Hour)
	if !ci.WarTime.Equal(wantNext) {
		t.Fatalf("下次攻城時間錯誤：got %v want %v", ci.WarTime, wantNext)
	}
	if !sys.warEnd[1].Equal(wantNext.Add(90 * time.Minute)) {
		t.Fatalf("下次攻城結束時間錯誤：got %v want %v", sys.warEnd[1], wantNext.Add(90*time.Minute))
	}
}

func TestWarIntervalDefaultsToJavaValues(t *testing.T) {
	sys := newTestCastleSystem(t)
	if got := sys.warInterval(); got != 4*24*time.Hour {
		t.Fatalf("預設攻城間隔錯誤：got %v want 96h", got)
	}
	if got := sys.warDuration(); got != 2*time.Hour {
		t.Fatalf("預設攻城時長錯誤：got %v want 2h", got)
	}
	sys.deps.Config = &config.Config{} // 零值視同未設定
	if got := sys.warInterval(); got != 4*24*time.Hour {
		t.Fatalf("零值設定應回退預設：got %v", got)
	}
}

func TestTickWarPushesExpiredWarTimeForward(t *testing.T) {
	sys := newTestCastleSystem(t)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	sys.now = func() time.Time { return now }

	ci := sys.castles[1]
	ci.WarTime = now.Add(-3 * 24 * time.Hour)
	sys.warEnd[1] = ci.WarTime.Add(2 * time.Hour)
	delete(sys.castles, 7)

	sys.TickWar()

	if ci.IsWar {
		t.Fatal("過期攻城時間不應開戰")
	}
	wantNext := now.Add(4 * 24 * time.Hour)
	if !ci.WarTime.Equal(wantNext) {
		t.Fatalf("過期攻城時間未推進：got %v want %v", ci.WarTime, wantNext)
	}
	if !sys.warEnd[1].Equal(wantNext.Add(2 * time.Hour)) {
		t.Fatalf("推進後結束時間錯誤：got %v", sys.warEnd[1])
	}

	// 再 tick 一次：時間在未來，不應再變動。
	sys.TickWar()
	if !ci.WarTime.Equal(wantNext) {
		t.Fatalf("未過期的攻城時間被重複推進：got %v", ci.WarTime)
	}
}

func TestTickWarStartsWarWhenInWindow(t *testing.T) {
	sys := newTestCastleSystem(t)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	sys.now = func() time.Time { return now }
	delete(sys.castles, 7)

	ci := sys.castles[1]
	ci.WarTime = now.Add(-time.Minute)
	sys.warEnd[1] = ci.WarTime.Add(2 * time.Hour)

	sys.TickWar()
	if !ci.IsWar {
		t.Fatal("攻城時段內應開戰")
	}
}

func TestTeleportNonClanPlayersUsesGetbackAndSkipsGM(t *testing.T) {
	sys := newTestCastleSystem(t)
	ws := sys.deps.World

	type tp struct {
		name       string
		x, y       int32
		mapID      int16
		clanID     int32
		access     int16
		gmModeOff  bool
		wantKicked bool
	}
	cases := []tp{
		{name: "owner", x: 33100, y: 32750, mapID: 4, clanID: 7, wantKicked: false},
		{name: "gm", x: 33100, y: 32750, mapID: 4, clanID: 9, access: world.AccessLevelGM, wantKicked: false},
		{name: "gm-off", x: 33100, y: 32750, mapID: 4, clanID: 9, access: world.AccessLevelGM, gmModeOff: true, wantKicked: true},
		{name: "enemy", x: 33100, y: 32750, mapID: 4, clanID: 9, wantKicked: true},
		{name: "noclan", x: 33100, y: 32750, mapID: 4, clanID: 0, wantKicked: true},
		{name: "outside", x: 33000, y: 32750, mapID: 4, clanID: 9, wantKicked: false},
		{name: "othermap", x: 33100, y: 32750, mapID: 15, clanID: 9, wantKicked: false},
	}
	players := map[string]*world.PlayerInfo{}
	for i, c := range cases {
		p := &world.PlayerInfo{
			SessionID:   uint64(i + 1),
			CharID:      int32(i + 1),
			Name:        c.name,
			X:           c.x,
			Y:           c.y,
			MapID:       c.mapID,
			ClanID:      c.clanID,
			AccessLevel: c.access,
			GMModeOff:   c.gmModeOff,
		}
		ws.AddPlayer(p)
		players[c.name] = p
	}

	type call struct {
		x, y    int32
		mapID   int16
		heading int16
	}
	teleported := map[string]call{}
	sys.teleport = func(_ *l1net.Session, player *world.PlayerInfo, x, y int32, mapID, heading int16) {
		teleported[player.Name] = call{x, y, mapID, heading}
	}

	sys.teleportNonClanPlayers(1, &handler.CastleInfo{CastleID: 1, OwnerClanID: 7})

	for _, c := range cases {
		got, ok := teleported[c.name]
		if ok != c.wantKicked {
			t.Fatalf("%s：傳送與否錯誤 got=%v want=%v", c.name, ok, c.wantKicked)
		}
		if !ok {
			continue
		}
		if got.heading != 5 || got.mapID != 4 {
			t.Fatalf("%s：傳送參數錯誤 %+v", c.name, got)
		}
		validGetback := (got.x == 33048 && got.y == 32750) || (got.x == 33059 && got.y == 32768)
		if !validGetback {
			t.Fatalf("%s：傳送座標不在回城點清單中 %+v", c.name, got)
		}
	}
}

func TestGetbackLocFallsBackToSilverKnightTown(t *testing.T) {
	table := testCastleTable()
	loc := table.GetbackLoc(7, nil) // 測試表的亞丁未設 getback
	if loc != data.DefaultGetbackLoc {
		t.Fatalf("無回城點時應回銀騎士村：got %+v", loc)
	}
	loc = table.GetbackLoc(99, nil)
	if loc != data.DefaultGetbackLoc {
		t.Fatalf("不存在的城堡應回銀騎士村：got %+v", loc)
	}
	picked := table.GetbackLoc(1, func(n int) int {
		if n != 2 {
			t.Fatalf("候選數錯誤：%d", n)
		}
		return 1
	})
	if picked.X != 33059 || picked.Y != 32768 {
		t.Fatalf("隨機索引未套用：%+v", picked)
	}
}

func TestCanDamageAdenMainTowerNeedsThreeSubTowersDead(t *testing.T) {
	sys := newTestCastleSystem(t)
	ws := sys.deps.World
	sys.castles[7].IsWar = true
	sys.castles[7].OwnerClanID = 0 // 無城主：不需宣戰

	main := addWarNpc(ws, 81189, "L1Tower", 34090, 33260, 4, false)
	subs := []*world.NpcInfo{
		addWarNpc(ws, 81190, "L1Tower", 34057, 33291, 4, false),
		addWarNpc(ws, 81191, "L1Tower", 34123, 33291, 4, false),
		addWarNpc(ws, 81192, "L1Tower", 34057, 33230, 4, false),
		addWarNpc(ws, 81193, "L1Tower", 34123, 33230, 4, false),
	}
	attacker := &world.PlayerInfo{CharID: 1, ClanID: 9}

	if sys.CanDamageTower(attacker, main) {
		t.Fatal("子塔全存活時不應可攻擊主塔")
	}
	subs[0].Dead = true
	subs[1].Dead = true
	if sys.CanDamageTower(attacker, main) {
		t.Fatal("只破壞 2 座子塔時不應可攻擊主塔")
	}
	subs[2].Dead = true
	if !sys.CanDamageTower(attacker, main) {
		t.Fatal("破壞 3 座子塔後應可攻擊主塔（Java: subTowerDeadCount >= 3）")
	}
}

func TestLoadCastleTableGetbackFromYaml(t *testing.T) {
	table, err := data.LoadCastleTable("../../data/yaml/castles.yaml")
	if err != nil {
		t.Fatalf("載入 castles.yaml 失敗：%v", err)
	}
	for id := int32(1); id <= 8; id++ {
		c := table.Get(id)
		if c == nil {
			t.Fatalf("castles.yaml 缺少城堡 %d", id)
		}
		if len(c.Getback) == 0 {
			t.Fatalf("城堡 %d 未設定 getback 回城點", id)
		}
		for _, p := range c.Getback {
			if p.Map == 0 || p.X == 0 || p.Y == 0 {
				t.Fatalf("城堡 %d 回城點不完整：%+v", id, p)
			}
			// 回城點必須在戰爭區外，否則趕人後仍在旗內
			if table.CheckInWarArea(id, p.X, p.Y, p.Map) {
				t.Fatalf("城堡 %d 回城點 %+v 位於戰爭區內", id, p)
			}
		}
	}
	if table.Get(8).Getback[0].Map != 310 {
		t.Fatalf("狄亞得回城點地圖應為 310：%+v", table.Get(8).Getback[0])
	}
}
