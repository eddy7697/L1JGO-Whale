package system

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

func attachShockStunItemTable(t *testing.T, s *SkillSystem) {
	t.Helper()
	items, err := data.LoadItemTable(
		filepath.Join("..", "..", "data", "yaml", "weapon_list.yaml"),
		filepath.Join("..", "..", "data", "yaml", "armor_list.yaml"),
		filepath.Join("..", "..", "data", "yaml", "etcitem_list.yaml"),
	)
	if err != nil {
		t.Fatalf("讀取物品資料失敗: %v", err)
	}
	s.deps.Items = items
}

func attachShockStunNpcTable(t *testing.T, s *SkillSystem) {
	t.Helper()
	npcs, err := data.LoadNpcTable(filepath.Join("..", "..", "data", "yaml", "npc_list.yaml"))
	if err != nil {
		t.Fatalf("讀取 NPC 資料失敗: %v", err)
	}
	s.deps.Npcs = npcs
}

func newShockStunSafetyMap(t *testing.T) *data.MapDataTable {
	t.Helper()
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "map_list.yaml")
	tileDir := filepath.Join(dir, "tiles")
	if err := os.Mkdir(tileDir, 0o755); err != nil {
		t.Fatalf("建立測試地圖目錄失敗: %v", err)
	}
	yaml := []byte(`maps:
  - map_id: 901
    name: shock_stun_safety_test
    start_x: 100
    end_x: 101
    start_y: 100
    end_y: 100
`)
	if err := os.WriteFile(yamlPath, yaml, 0o644); err != nil {
		t.Fatalf("寫入測試地圖清單失敗: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tileDir, "901.txt"), []byte("15,31\n"), 0o644); err != nil {
		t.Fatalf("寫入測試地圖格資料失敗: %v", err)
	}
	maps, err := data.LoadMapData(yamlPath, tileDir)
	if err != nil {
		t.Fatalf("載入測試地圖失敗: %v", err)
	}
	return maps
}

type shockStunPvPSpy struct {
	called   int
	attacker *world.PlayerInfo
	target   *world.PlayerInfo
}

func (s *shockStunPvPSpy) HandlePvPAttack(_, _ *world.PlayerInfo) {}

func (s *shockStunPvPSpy) HandlePvPFarAttack(_, _ *world.PlayerInfo) {}

func (s *shockStunPvPSpy) AddLawfulFromNpc(_ *world.PlayerInfo, _ int32) {}

func (s *shockStunPvPSpy) BeginHell(_ *world.PlayerInfo, _ bool) {}

func (s *shockStunPvPSpy) TriggerPinkName(attacker, target *world.PlayerInfo) {
	s.called++
	s.attacker = attacker
	s.target = target
}

type shockStunCombatSpy struct {
	requests []handler.AttackRequest
}

func (s *shockStunCombatSpy) QueueAttack(req handler.AttackRequest) {
	s.requests = append(s.requests, req)
}

func (s *shockStunCombatSpy) HandleNpcDeath(_ *world.NpcInfo, _ *world.PlayerInfo, _ []*world.PlayerInfo) *handler.NpcKillResult {
	return nil
}

func (s *shockStunCombatSpy) AddExp(_ *world.PlayerInfo, _ int32) {}

func (s *shockStunCombatSpy) ExecuteRangedAttackOnNpc(_ *world.PlayerInfo, _ int32) {}

func hasGlobalSystemMessageText(packets [][]byte, text string) bool {
	want := append(packet.EncodeString(text), 0)
	for _, pkt := range packets {
		for i := 0; i+2+len(want) <= len(pkt); i++ {
			if pkt[i] == packet.S_OPCODE_MESSAGE && pkt[i+1] == 9 && string(pkt[i+2:i+2+len(want)]) == string(want) {
				return true
			}
		}
	}
	return false
}

func hasNormalChatText(packets [][]byte, text string) bool {
	want := append(packet.EncodeString(text), 0)
	for _, pkt := range packets {
		if len(pkt) >= 6+len(want) && pkt[0] == packet.S_OPCODE_SAY && pkt[1] == handler.ChatNormal &&
			string(pkt[6:6+len(want)]) == string(want) {
			return true
		}
	}
	return false
}

func hasShockStunGMDurationMessage(packets [][]byte) bool {
	for seconds := 1; seconds <= 5; seconds++ {
		if hasNormalChatText(packets, fmt.Sprintf("此次衝暈秒數為%d秒..只有GM看的到", seconds)) {
			return true
		}
	}
	return false
}

func hasActionGfxPacket(packets [][]byte, objectID int32, actionCode byte) bool {
	for _, pkt := range packets {
		if len(pkt) < 6 || pkt[0] != packet.S_OPCODE_ACTION {
			continue
		}
		if int32(binary.LittleEndian.Uint32(pkt[1:5])) == objectID && pkt[5] == actionCode {
			return true
		}
	}
	return false
}

func countSkillEffectPackets(packets [][]byte, objectID int32, gfxID int32) int {
	count := 0
	for _, pkt := range packets {
		if len(pkt) < 7 || pkt[0] != packet.S_OPCODE_EFFECT {
			continue
		}
		if int32(binary.LittleEndian.Uint32(pkt[1:5])) == objectID &&
			int32(binary.LittleEndian.Uint16(pkt[5:7])) == gfxID {
			count++
		}
	}
	return count
}

func TestSkillClanShockStunDurationMatchesJavaConfigRange(t *testing.T) {
	rand.Seed(1)

	for i := 0; i < 100; i++ {
		dur := shockStunDurationSeconds()
		if dur < 1 || dur > 5 {
			t.Fatalf("Java SHOCK_STUN_TIMER=1~5，暈眩秒數不可超出範圍，got=%d", dur)
		}
	}
}

func TestSkillClanShockStunGmReceivesJavaDurationMessage(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "gm",
		AccessLevel: 200,
		X:           100,
		Y:           100,
		MapID:       4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "target",
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -100,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if !hasShockStunGMDurationMessage(drainSkillTestPackets(caster.Session)) {
		t.Fatal("Java SHOCK_STUN 成功時 GM 會收到 S_ServerMessage(String) 顯示此次衝暈秒數")
	}
}

// Java SHOCK_STUN.start(L1PcInstance,...) 第 44-46 行 `if (srcpc.isGm()) { ... }`
// 是嚴格 GM 門檻：非 GM 玩家施放成功的衝暈秒數不會回饋給自己。既有測試覆蓋
// 「GM caster 收到」與「附近 GM 觀察者不收到（非廣播）」，本步補上第三條 negative
// case：非 GM caster 施放 87 成功時也不收到 GM 秒數訊息，鎖定 Go 不會誤把
// GM 訊息廣播給所有 caster（避免 `srcpc.isGm()` 退化為「成功就送」）。
func TestSkillClanShockStunNonGmCasterDoesNotReceiveDurationMessageLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "regular-player",
		AccessLevel: 0,
		X:           100,
		Y:           100,
		MapID:       4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "target",
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -100,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	_ = drainSkillTestPackets(caster.Session)
	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if !target.HasBuff(87) {
		t.Fatal("測試前提：SHOCK_STUN 應命中目標套用 87，未命中無法驗證 GM 訊息 negative 行為")
	}
	if hasShockStunGMDurationMessage(drainSkillTestPackets(caster.Session)) {
		t.Fatal("Java `if (srcpc.isGm())` 嚴格 GM 門檻：非 GM 玩家施放 87 成功時不應收到「此次衝暈秒數為...」訊息")
	}
}

// Java SHOCK_STUN.start(L1PcInstance,...) 第 44-46 行：
//
//	if (srcpc.isGm()) {
//	    srcpc.sendPackets(new S_ServerMessage("此次衝暈秒數為..."));
//	}
//
// 使用 `srcpc.sendPackets`（caster only），不是 `sendPacketsAll`（broadcast），
// 因此附近的 GM 觀察者不應收到此訊息。既有測試僅驗證 caster 收到，本測試補上
// 「附近 GM 觀察者不收到」的負面回歸，鎖定 Go `SendNormalChat(sess, ...)` 不會誤廣播。
func TestSkillClanShockStunGmDurationMessageNotBroadcastToNearbyGmLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "gm-caster",
		AccessLevel: 200,
		X:           100,
		Y:           100,
		MapID:       4,
	})
	observer := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   2,
		Session:     newSkillTestSession(t, 2),
		CharID:      1002,
		Name:        "gm-observer",
		AccessLevel: 200,
		X:           100,
		Y:           101,
		MapID:       4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  3,
		Session:    newSkillTestSession(t, 3),
		CharID:     1003,
		Name:       "target",
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -100,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	_ = drainSkillTestPackets(caster.Session)
	_ = drainSkillTestPackets(observer.Session)
	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if !hasShockStunGMDurationMessage(drainSkillTestPackets(caster.Session)) {
		t.Fatal("Java srcpc.sendPackets 仍會把 GM 秒數訊息送給施法者")
	}
	if hasShockStunGMDurationMessage(drainSkillTestPackets(observer.Session)) {
		t.Fatal("Java 使用 srcpc.sendPackets 非 sendPacketsAll，附近 GM 觀察者不應收到此訊息")
	}
}

func TestSkillClanShockStunGmReceivesJavaDurationMessageForNpcTarget(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "gm",
		AccessLevel: 200,
		Level:       60,
		X:           100,
		Y:           100,
		MapID:       4,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "stun target",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 50,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	for seed := int64(1); seed <= 100; seed++ {
		rand.Seed(seed)
		npc.Paralyzed = false
		npc.ActiveDebuffs = nil
		_ = drainSkillTestPackets(caster.Session)

		s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)
		if !npc.HasDebuff(87) {
			continue
		}
		if !hasShockStunGMDurationMessage(drainSkillTestPackets(caster.Session)) {
			t.Fatal("Java SHOCK_STUN 玩家施放到 NPC 成功時，GM 也會收到 S_ServerMessage(String) 顯示此次衝暈秒數")
		}
		return
	}
	t.Fatal("測試種子 1..100 未觸發 SHOCK_STUN NPC 成功，無法驗證 GM 秒數訊息")
}

// Java `SHOCK_STUN.start(L1PcInstance,...)` 第 44-46 行 `if (srcpc.isGm())` 嚴格
// GM 門檻對 NPC 目標分支同樣適用（GM 訊息在 player/NPC 目標 if/else 分支之前）。
// 既有 `GmReceivesJavaDurationMessageForNpcTarget` 是 positive case，本步補上
// negative parallel：非 GM 玩家對 NPC 施放 87 成功時也不收到 GM 秒數訊息，與
// `NonGmCasterDoesNotReceiveDurationMessageLikeJava`（玩家目標版）配對完成
// 兩種目標的 negative 覆蓋。
func TestSkillClanShockStunNonGmCasterDoesNotReceiveDurationMessageForNpcTargetLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:        1,
		Session:          newSkillTestSession(t, 1),
		CharID:           1001,
		Name:             "regular-player",
		AccessLevel:      0,
		Level:            99,
		OriginalMagicHit: 100,
		X:                100,
		Y:                100,
		MapID:            4,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "stun target",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 1,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	_ = drainSkillTestPackets(caster.Session)
	s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)

	if !npc.HasDebuff(87) {
		t.Fatal("測試前提：高等級 + OriginalMagicHit 100 玩家施放對 Level 1 NPC 應命中，未命中無法驗證 GM 訊息行為")
	}
	if hasShockStunGMDurationMessage(drainSkillTestPackets(caster.Session)) {
		t.Fatal("Java `if (srcpc.isGm())` 嚴格 GM 門檻對 NPC 目標分支同樣適用，非 GM 玩家對 NPC 施放 87 成功時不應收到「此次衝暈秒數為...」訊息")
	}
}

func TestSkillClanShockStunNpcExistingDebuffDoesNotRefreshLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "gm",
		AccessLevel: 200,
		Level:       60,
		X:           100,
		Y:           100,
		MapID:       4,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "stun target",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 50,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}
	npc.Paralyzed = true
	npc.AddDebuff(87, 999)

	for seed := int64(1); seed <= 100; seed++ {
		rand.Seed(seed)
		_ = drainSkillTestPackets(caster.Session)

		s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)
		if got := npc.ActiveDebuffs[87]; got != 999 {
			t.Fatalf("Java SHOCK_STUN 目標已有 87 效果時不刷新 NPC debuff，got ticks=%d", got)
		}
		packets := drainSkillTestPackets(caster.Session)
		if hasShockStunGMDurationMessage(packets) {
			t.Fatal("Java SHOCK_STUN 目標已有 87 效果時不會重送 GM 秒數訊息")
		}
		if !hasSkillEffectPacket(packets, npc.ID, 4434) {
			t.Fatal("Java SHOCK_STUN 目標已有 87 效果時仍會由 L1SkillUse 送目標 S_SkillSound(4434)")
		}
		if effects := ws.GetNearbyGroundEffects(npc.X, npc.Y, npc.MapID); len(effects) != 0 {
			t.Fatalf("Java SHOCK_STUN 目標已有 87 效果時不會再次 spawnEffect(81162)，got effects=%d", len(effects))
		}
	}
}

func TestSkillClanShockStunPlayerExistingBuffDoesNotRefreshButSendsJavaTargetGfx(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "gm",
		AccessLevel: 200,
		Level:       60,
		X:           100,
		Y:           100,
		MapID:       4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "target",
		Level:      50,
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -100,
		Paralyzed:  true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 87, TicksLeft: 999, SetParalyzed: true})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 0}

	_ = drainSkillTestPackets(target.Session)
	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if got := target.GetBuff(87).TicksLeft; got != 999 {
		t.Fatalf("Java SHOCK_STUN 目標已有 87 效果時不刷新玩家 buff，got ticks=%d", got)
	}
	packets := drainSkillTestPackets(target.Session)
	if hasShockStunGMDurationMessage(drainSkillTestPackets(caster.Session)) {
		t.Fatal("Java SHOCK_STUN 目標已有 87 效果時不會重送 GM 秒數訊息")
	}
	if hasActionGfxPacket(packets, caster.CharID, 19) {
		t.Fatal("Java SHOCK_STUN 目標已有 87 效果時只送目標 S_SkillSound(4434)，不送施法者 S_DoActionGFX")
	}
	if !hasSkillEffectPacket(packets, target.CharID, 4434) {
		t.Fatal("Java SHOCK_STUN 目標已有 87 效果時仍會由 L1SkillUse 送目標 S_SkillSound(4434)")
	}
	if effects := ws.GetNearbyGroundEffects(target.X, target.Y, target.MapID); len(effects) != 0 {
		t.Fatalf("Java SHOCK_STUN 目標已有 87 效果時不會再次 spawnEffect(81162)，got effects=%d", len(effects))
	}
}

func TestSkillClanShockStunPlayerCasterDoesNotParalyzeGuardNpcLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		Level:     60,
		X:         100,
		Y:         100,
		MapID:     4,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 70500,
		Impl:  "L1Guard",
		Name:  "guard target",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 50,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	for seed := int64(1); seed <= 100; seed++ {
		rand.Seed(seed)
		npc.Paralyzed = false
		npc.ActiveDebuffs = nil
		for _, effect := range ws.GroundEffectList() {
			ws.RemoveGroundEffect(effect.ID)
		}

		s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)
		if !npc.HasDebuff(87) {
			continue
		}
		if npc.Paralyzed {
			t.Fatal("Java 玩家施放 SHOCK_STUN 到 L1Guard 會套 87 效果與 81162，但不會 setParalyzed(true)")
		}
		return
	}
	t.Fatal("測試種子 1..100 未觸發 SHOCK_STUN L1Guard 成功，無法驗證玩家施放 NPC 類別邊界")
}

// Java SHOCK_STUN.start(L1PcInstance,...) 第 53-57 行明確列出 L1MonsterInstance / L1SummonInstance /
// L1PetInstance 三個 NPC 類別會 setParalyzed(true)。L1Monster 已被多個成功測試隱含覆蓋；
// L1Guard / L1Guardian 已有 negative 排除測試。本測試補上 L1Pet 的 positive 包含案例，
// 鎖定 Go `executeNpcDebuffSkill` case 87 對 `Impl == "L1Pet"` 同時套 87 debuff、81162 效果與 setParalyzed(true)。
func TestSkillClanShockStunPlayerCasterParalyzesPetNpcLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		Level:     60,
		X:         100,
		Y:         100,
		MapID:     4,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 70600,
		Impl:  "L1Pet",
		Name:  "pet target",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 50,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	for seed := int64(1); seed <= 100; seed++ {
		rand.Seed(seed)
		npc.Paralyzed = false
		npc.ActiveDebuffs = nil
		for _, effect := range ws.GroundEffectList() {
			ws.RemoveGroundEffect(effect.ID)
		}

		s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)
		if !npc.HasDebuff(87) {
			continue
		}
		if !npc.Paralyzed {
			t.Fatal("Java 玩家施放 SHOCK_STUN 到 L1Pet 第 53-57 行會 setParalyzed(true)，Go 應同步設定")
		}
		effects := ws.GetNearbyGroundEffects(npc.X, npc.Y, npc.MapID)
		if len(effects) != 1 || effects[0].NpcID != 81162 {
			t.Fatalf("Java SHOCK_STUN 對 L1Pet 仍會 spawnEffect(81162)，got effects=%v", effects)
		}
		return
	}
	t.Fatal("測試種子 1..100 未觸發 SHOCK_STUN L1Pet 成功，無法驗證玩家施放 setParalyzed 包含案例")
}

// Java SHOCK_STUN.start(L1PcInstance,...) 第 53-57 行三類 setParalyzed positive 之最後一項：
// L1SummonInstance。L1Monster 已隱含覆蓋；L1Pet 已有 TestSkillClanShockStunPlayerCasterParalyzesPetNpcLikeJava。
// 本測試補上 L1Summon 鎖定 Go `executeNpcDebuffSkill` case 87 對 `Impl == "L1Summon"`
// 同時套 87 debuff、81162 效果與 setParalyzed(true)，完成 Java 第 53-57 行三類 positive 全覆蓋。
func TestSkillClanShockStunPlayerCasterParalyzesSummonNpcLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		Level:     60,
		X:         100,
		Y:         100,
		MapID:     4,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 70700,
		Impl:  "L1Summon",
		Name:  "summon target",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 50,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	for seed := int64(1); seed <= 100; seed++ {
		rand.Seed(seed)
		npc.Paralyzed = false
		npc.ActiveDebuffs = nil
		for _, effect := range ws.GroundEffectList() {
			ws.RemoveGroundEffect(effect.ID)
		}

		s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)
		if !npc.HasDebuff(87) {
			continue
		}
		if !npc.Paralyzed {
			t.Fatal("Java 玩家施放 SHOCK_STUN 到 L1Summon 第 53-57 行會 setParalyzed(true)，Go 應同步設定")
		}
		effects := ws.GetNearbyGroundEffects(npc.X, npc.Y, npc.MapID)
		if len(effects) != 1 || effects[0].NpcID != 81162 {
			t.Fatalf("Java SHOCK_STUN 對 L1Summon 仍會 spawnEffect(81162)，got effects=%v", effects)
		}
		return
	}
	t.Fatal("測試種子 1..100 未觸發 SHOCK_STUN L1Summon 成功，無法驗證玩家施放 setParalyzed 包含案例")
}

// Java SHOCK_STUN.start(L1PcInstance,...) 第 53-57 行只對 L1MonsterInstance / L1SummonInstance / L1PetInstance
// 設 `setParalyzed(true)`，不包含 L1GuardianInstance；對應的 NPC caster 分支 (第 76-78 行) 才會包含 Guardian。
// L1Guard 已有 TestSkillClanShockStunPlayerCasterDoesNotParalyzeGuardNpcLikeJava 鎖定；
// 本測試補上 L1Guardian 同樣不被玩家施放路徑 setParalyzed 的回歸。
func TestSkillClanShockStunPlayerCasterDoesNotParalyzeGuardianNpcLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		Level:     60,
		X:         100,
		Y:         100,
		MapID:     4,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 70501,
		Impl:  "L1Guardian",
		Name:  "guardian target",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 50,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	for seed := int64(1); seed <= 100; seed++ {
		rand.Seed(seed)
		npc.Paralyzed = false
		npc.ActiveDebuffs = nil
		for _, effect := range ws.GroundEffectList() {
			ws.RemoveGroundEffect(effect.ID)
		}

		s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)
		if !npc.HasDebuff(87) {
			continue
		}
		if npc.Paralyzed {
			t.Fatal("Java 玩家施放 SHOCK_STUN 到 L1Guardian 會套 87 效果與 81162，但不會 setParalyzed(true)（Java 第 53-57 行排除 Guardian）")
		}
		return
	}
	t.Fatal("測試種子 1..100 未觸發 SHOCK_STUN L1Guardian 成功，無法驗證玩家施放 NPC 類別邊界")
}

func TestSkillClanShockStunIntReducesMpConsumeLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "caster",
		Level:       60,
		Intel:       18,
		X:           100,
		Y:           100,
		MapID:       4,
		MP:          9,
		MaxMP:       9,
		KnownSpells: []int32{87},
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		Level:     1,
		X:         101,
		Y:         100,
		MapID:     4,
	})
	weapon := caster.Inv.AddItemWithID(7001, 52, 1, "雙手劍", 0, 1000, false, 1)
	weapon.Equipped = true
	caster.Equip.Set(world.SlotWeapon, weapon)

	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  target.CharID,
	})

	if caster.MP != 0 {
		t.Fatalf("Java 對 SHOCK_STUN 會以 INT 18 將 MP 15 扣減 6，應消耗 9 MP，MP=%d", caster.MP)
	}
}

// Java `C_UseSkill` 第 87-88 行 `_cast_with_silence[]` 明確列出 `SHOCK_STUN`，
// 第 182-200 行對 `SILENCE/AREA_OF_SILENCE/STATUS_POISON_SILENCE` 三種沉默 buff
// 只在 `!isSilenceUsableSkill(skillId)` 時設 `isError=true`，因此被沉默的玩家
// 仍可施放 87。鎖定 Go `isCastableWhileSilenced(87)=true` 行為：Silenced 玩家
// 施放 87 不被 285 攔下、仍能正常套用到目標。
func TestSkillClanShockStunSilencedPlayerCanCastLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "silenced-caster",
		Level:       60,
		Intel:       18,
		X:           100,
		Y:           100,
		MapID:       4,
		MP:          100,
		MaxMP:       100,
		Silenced:    true,
		KnownSpells: []int32{87},
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		Level:     1,
		X:         101,
		Y:         100,
		MapID:     4,
	})
	weapon := caster.Inv.AddItemWithID(7001, 52, 1, "雙手劍", 0, 1000, false, 1)
	weapon.Equipped = true
	caster.Equip.Set(world.SlotWeapon, weapon)

	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)

	rand.Seed(1)
	mpBefore := caster.MP

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  target.CharID,
	})

	if hasServerMessage(drainSkillTestPackets(caster.Session), 285) {
		t.Fatal("Java _cast_with_silence 列出 SHOCK_STUN，被沉默玩家施放 87 不應送 S_ServerMessage(285)")
	}
	if caster.MP == mpBefore {
		t.Fatal("Java 被沉默玩家施放 SHOCK_STUN 仍會進入 useConsume 扣除 MP，MP 未變表示 silence gate 在 useConsume 之前攔下技能")
	}
}

// Java `L1Magic.useReuseDelay()` 根據技能表 `reuse_delay` 設定 `_skillDelay`，
// SHOCK_STUN 對應 `reuse_delay: 1500`（毫秒）。鎖定 Go `processSkill` 對 87 cast
// 成功後 `player.SkillDelayUntil` 至少推進 1500ms，避免未來 reuse_delay 字段
// 解析錯誤或被忽略導致玩家連續高頻施放 87。
func TestSkillClanShockStunCastSetsReuseDelayLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "knight",
		X:           100,
		Y:           100,
		MapID:       4,
		Level:       60,
		Intel:       18,
		MP:          100,
		MaxMP:       100,
		KnownSpells: []int32{87},
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		Level:     1,
		X:         101,
		Y:         100,
		MapID:     4,
	})
	weapon := caster.Inv.AddItemWithID(7001, 52, 1, "雙手劍", 0, 1000, false, 1)
	weapon.Equipped = true
	caster.Equip.Set(world.SlotWeapon, weapon)

	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)

	before := time.Now()
	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  target.CharID,
	})

	wantUntil := before.Add(1500 * time.Millisecond)
	if caster.SkillDelayUntil.Before(wantUntil) {
		t.Fatalf("Java SHOCK_STUN `reuse_delay: 1500` 應推進 SkillDelayUntil 至少 1500ms，before=%v SkillDelayUntil=%v diff=%v", before, caster.SkillDelayUntil, caster.SkillDelayUntil.Sub(before))
	}
}

func TestSkillClanShockStunPlayerQueuesJavaOnActionMelee(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     60,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
		Level:     50,
	})
	s := newSkillTestSystem(t, ws)
	combat := &shockStunCombatSpy{}
	s.deps.Combat = combat
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if len(combat.requests) != 1 {
		t.Fatalf("Java L1SkillUse 會在 SHOCK_STUN 目標計算前呼叫 _target.onAction(_player)，got requests=%d", len(combat.requests))
	}
	req := combat.requests[0]
	if req.AttackerSessionID != caster.SessionID || req.TargetID != target.CharID || !req.IsMelee {
		t.Fatalf("SHOCK_STUN onAction 應排入近戰攻擊，request=%+v", req)
	}
}

// Java `SHOCK_STUN.start(L1PcInstance,...)` 第 48 行 `spawnEffect(..., cha.getX(),
// cha.getY(), ...)` 對 NPC 目標（cha 為 L1NpcInstance）同樣使用 target NPC 座標。
// 與「玩家→玩家」版配對，鎖定 Go `executeNpcDebuffSkill` 對 NPC 目標的 87 case 也
// 把 81162 spawn 在 npc.X/npc.Y 而非 player caster 座標。caster (100,100) 與
// NPC target (104,100) 分開 4 格嚴格驗證。
func TestSkillClanShockStunPlayerCasterNpcTargetEffectSpawnsAtTargetCoordsLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:        1,
		Session:          newSkillTestSession(t, 1),
		CharID:           1001,
		Name:             "knight",
		X:                100,
		Y:                100,
		MapID:            4,
		Level:            99,
		OriginalMagicHit: 100,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "far-npc",
		X:     104,
		Y:     100,
		MapID: 4,
		Level: 1,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434, Ranged: 5}

	s.executeBuffSkill(caster.Session, caster, skill, npc.ID)

	if !npc.HasDebuff(87) {
		t.Fatal("測試前提：玩家施放 SHOCK_STUN 對 Level 1 NPC 應命中，未命中無法驗證 spawn 座標")
	}
	effects := ws.GetNearbyGroundEffects(npc.X, npc.Y, npc.MapID)
	if len(effects) != 1 {
		t.Fatalf("玩家施放 SHOCK_STUN 對 NPC 成功時應 spawnEffect 1 筆 81162，got=%d", len(effects))
	}
	if effects[0].X != npc.X || effects[0].Y != npc.Y {
		t.Fatalf("Java spawnEffect 第 3-4 個參數使用 cha.getX()/cha.getY()（NPC target 座標），effect 應 spawn 在 (npc.X=%d, npc.Y=%d)，got=(X=%d, Y=%d)", npc.X, npc.Y, effects[0].X, effects[0].Y)
	}
}

// Java `SHOCK_STUN.start(L1PcInstance,...)` 第 48 行
// `L1SpawnUtil.spawnEffect(81162, shock, cha.getX(), cha.getY(), srcpc.getMapId(), srcpc, 0)`
// 第 3-4 個參數使用 `cha.getX()/cha.getY()`（target 座標），不是 `srcpc.getX()`
// （player caster 座標）。與 NPC single + AOE 版配對，完成三條路徑的座標精確驗證。
// 鎖定 Go `applyShockStunToPlayer` 不會把 81162 spawn 在 caster 座標。
func TestSkillClanShockStunPlayerCasterEffectSpawnsAtTargetCoordsLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:        1,
		Session:          newSkillTestSession(t, 1),
		CharID:           1001,
		Name:             "knight",
		X:                100,
		Y:                100,
		MapID:            4,
		Level:            99,
		OriginalMagicHit: 100,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "far-target",
		Level:      1,
		X:          104,
		Y:          100,
		MapID:      4,
		RegistStun: -100,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434, Ranged: 5}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if !target.HasBuff(87) {
		t.Fatal("測試前提：玩家施放 SHOCK_STUN 對 Level 1 + RegistStun -100 應命中，未命中無法驗證 spawn 座標")
	}
	effects := ws.GetNearbyGroundEffects(target.X, target.Y, target.MapID)
	if len(effects) != 1 {
		t.Fatalf("玩家施放 SHOCK_STUN 成功時應 spawnEffect 1 筆 81162，got=%d", len(effects))
	}
	if effects[0].X != target.X || effects[0].Y != target.Y {
		t.Fatalf("Java spawnEffect 第 3-4 個參數使用 cha.getX()/cha.getY()（target 座標），effect 應 spawn 在 (target.X=%d, target.Y=%d)，got=(X=%d, Y=%d)", target.X, target.Y, effects[0].X, effects[0].Y)
	}
}

// Java `SHOCK_STUN.start(L1NpcInstance,...)` 第 71 行
// `L1SpawnUtil.spawnEffect(81162, shock, cha.getX(), cha.getY(), npc.getMapId(), npc, 0)`
// 第 3-4 個參數使用 `cha.getX()/cha.getY()`（target 玩家座標），不是 `npc.getX()`
// （NPC caster 座標）。與 AOE `EffectSpawnsAtTargetCoordsLikeJava` 配對，本步補上
// NPC 單體 caster 路徑的座標精確驗證：caster (100,100) 與 target (104,100) 分開
// 4 格，嚴格驗證 `effects[0].X == target.X && effects[0].Y == target.Y`，鎖定
// Go `ApplyNpcShockStun` 不會把 81162 spawn 在 caster 座標。
func TestSkillClanShockStunNpcCasterEffectSpawnsAtTargetCoordsLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "far-target",
		Level:      1,
		X:          104,
		Y:          100,
		MapID:      4,
		Paralyzed:  true,
		RegistStun: -100,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 99,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434, Ranged: 5}
	s.ApplyNpcShockStun(npc, target, skill, 0)

	if !target.HasBuff(87) {
		t.Fatal("測試前提：NPC 施放 SHOCK_STUN 對 Level 1 + RegistStun -100 應命中，未命中無法驗證 spawn 座標")
	}
	effects := ws.GetNearbyGroundEffects(target.X, target.Y, target.MapID)
	if len(effects) != 1 {
		t.Fatalf("NPC 施放 SHOCK_STUN 成功時應 spawnEffect 1 筆 81162，got=%d", len(effects))
	}
	if effects[0].X != target.X || effects[0].Y != target.Y {
		t.Fatalf("Java spawnEffect 第 3-4 個參數使用 cha.getX()/cha.getY()（target 座標），effect 應 spawn 在 (target.X=%d, target.Y=%d)，got=(X=%d, Y=%d)", target.X, target.Y, effects[0].X, effects[0].Y)
	}
}

func TestSkillClanShockStunInvisibleCasterBlockedLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "invis-knight",
		X:           100,
		Y:           100,
		MapID:       4,
		Level:       60,
		HP:          200,
		MaxHP:       200,
		MP:          100,
		MaxMP:       100,
		Invisible:   true,
		KnownSpells: []int32{87},
	})
	caster.AddBuff(&world.ActiveBuff{SkillID: 60, TicksLeft: 100, SetInvisible: true})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
		Level:     50,
	})
	s := newSkillBuffTestSystem(t, ws)
	combat := &shockStunCombatSpy{}
	s.deps.Combat = combat
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  target.CharID,
	})

	if !caster.Invisible || !caster.HasBuff(60) {
		t.Fatalf("Java C_UseSkill 會先以 isInvisUsableSkill 擋下 SHOCK_STUN，不應解除隱身，Invisible=%v buff60=%v", caster.Invisible, caster.GetBuff(60))
	}
	if len(combat.requests) != 0 {
		t.Fatalf("隱身狀態下 SHOCK_STUN 應被 C_UseSkill 拒絕，不應觸發 onAction，requests=%d", len(combat.requests))
	}
	if !hasServerMessage(drainSkillTestPackets(caster.Session), 1003) {
		t.Fatal("Java C_UseSkill 對隱身不可施放技能會回覆 S_ServerMessage(1003)")
	}
}

func TestSkillClanShockStunPolymorphBlockTakesPriorityOverInvisibilityLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "poly-invis-knight",
		X:           100,
		Y:           100,
		MapID:       4,
		Level:       60,
		MP:          100,
		MaxMP:       100,
		Invisible:   true,
		PolyID:      95,
		TempCharGfx: 95,
		KnownSpells: []int32{87},
	})
	caster.AddBuff(&world.ActiveBuff{SkillID: 60, TicksLeft: 100, SetInvisible: true})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
		Level:     50,
	})
	s := newSkillBuffTestSystem(t, ws)
	polys, err := data.LoadPolymorphTable(filepath.Join("..", "..", "data", "yaml", "polymorph_list.yaml"))
	if err != nil {
		t.Fatalf("載入變形資料失敗: %v", err)
	}
	s.deps.Polys = polys
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  target.CharID,
	})

	packets := drainSkillTestPackets(caster.Session)
	if !hasServerMessage(packets, 285) {
		t.Fatal("Java C_UseSkill 會先以 poly.canUseSkill=false 擋下技能並送 S_ServerMessage(285)")
	}
	if hasServerMessage(packets, 1003) {
		t.Fatal("Java C_UseSkill 在不可施法變形時尚未進入隱身技能限制，不應送 S_ServerMessage(1003)")
	}
	if !caster.Invisible || !caster.HasBuff(60) {
		t.Fatalf("不可施法變形應優先返回，不應解除隱身，Invisible=%v buff60=%v", caster.Invisible, caster.HasBuff(60))
	}
}

func TestSkillClanShockStunParalysisBlockTakesPriorityOverInvisibilityLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "stunned-invis-knight",
		X:           100,
		Y:           100,
		MapID:       4,
		Level:       60,
		MP:          100,
		MaxMP:       100,
		Invisible:   true,
		Paralyzed:   true,
		KnownSpells: []int32{87},
	})
	caster.AddBuff(&world.ActiveBuff{SkillID: 60, TicksLeft: 100, SetInvisible: true})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
		Level:     50,
	})
	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  target.CharID,
	})

	packets := drainSkillTestPackets(caster.Session)
	if !hasServerMessage(packets, 285) {
		t.Fatal("Java C_UseSkill 會先以 isParalyzedX 擋下技能並送 S_ServerMessage(285)")
	}
	if hasServerMessage(packets, 1003) {
		t.Fatal("Java C_UseSkill 在麻痺狀態時尚未進入隱身技能限制，不應送 S_ServerMessage(1003)")
	}
	if !caster.Invisible || !caster.HasBuff(60) {
		t.Fatalf("麻痺阻擋應優先返回，不應解除隱身，Invisible=%v buff60=%v", caster.Invisible, caster.HasBuff(60))
	}
}

func TestSkillClanShockStunNpcQueuesJavaOnActionMelee(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     60,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "stun target",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 50,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	combat := &shockStunCombatSpy{}
	s.deps.Combat = combat
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, npc.ID)

	if len(combat.requests) != 1 {
		t.Fatalf("Java L1SkillUse 對 NPC SHOCK_STUN 也會呼叫 _target.onAction(_player)，got requests=%d", len(combat.requests))
	}
	req := combat.requests[0]
	if req.AttackerSessionID != caster.SessionID || req.TargetID != npc.ID || !req.IsMelee {
		t.Fatalf("SHOCK_STUN NPC onAction 應排入近戰攻擊，request=%+v", req)
	}
}

func TestSkillClanShockStunPlayerTriggersPinkNameLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "target",
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -100,
	})
	s := newSkillTestSystem(t, ws)
	pvp := &shockStunPvPSpy{}
	s.deps.PvP = pvp
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if pvp.called != 1 || pvp.attacker != caster || pvp.target != target {
		t.Fatalf("Java SHOCK_STUN 成功時會 L1PinkName.onAction(target, caster)，called=%d attacker=%v target=%v", pvp.called, pvp.attacker, pvp.target)
	}
}

// Java `L1SkillUse.EXCEPT_COUNTER_MAGIC[]` 第 146 行明確列出 `SHOCK_STUN`，
// 因此 `_isCounterMagic = false`，`isUseCounterMagic` 對 87 永遠回傳 false。
// Counter Magic (buff 31) 不會抵擋 SHOCK_STUN，目標的 CM buff 不會被消耗，
// SHOCK_STUN 應正常套用。Go `counterMagicExempt[87] = true` 已存在，本測試補上鎖定。
func TestSkillClanShockStunPlayerBypassesCounterMagicLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "cm-target",
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -100,
	})
	// Counter Magic buff (Java: COUNTER_MAGIC = 31)
	target.AddBuff(&world.ActiveBuff{SkillID: 31, TicksLeft: 100})
	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if !target.HasBuff(87) || !target.Paralyzed {
		t.Fatalf("Java EXCEPT_COUNTER_MAGIC 包含 SHOCK_STUN，CM 不應抵擋，buff87=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	if !target.HasBuff(31) {
		t.Fatal("Java 對 SHOCK_STUN 不觸發 Counter Magic，目標的 CM buff 不應被消耗")
	}
}

// Java SHOCK_STUN.start(L1PcInstance,...) 第 38-58 行 `L1PinkName.onAction` 位於
// `if (!cha.hasSkillEffect(87))` 區塊內；目標已有 87 時整段被跳過，PinkName 不應被再次觸發。
// 既有 TestSkillClanShockStunPlayerTriggersPinkNameLikeJava 是 positive case，
// 本測試補上 negative case：目標已有 87 + 玩家施放 → PvP.TriggerPinkName 不應被呼叫。
func TestSkillClanShockStunPlayerExistingBuffDoesNotTriggerPinkNameLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "already-stunned",
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -100,
		Paralyzed:  true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 87, TicksLeft: 999, SetParalyzed: true})
	s := newSkillTestSystem(t, ws)
	pvp := &shockStunPvPSpy{}
	s.deps.PvP = pvp
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if pvp.called != 0 {
		t.Fatalf("Java 目標已有 87 時 L1PinkName.onAction 不應被再次觸發（整段被 !hasSkillEffect(87) 跳過），called=%d", pvp.called)
	}
}

func TestSkillClanShockStunPlayerSuccessUsesJavaImpactHaloRate(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     60,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
		Level:     50,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("Java SHOCK_STUN 高等玩家成功率基準為 IMPACT_HALO_1=40，不應使用 Go 一般公式 80%%，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
}

func TestSkillClanShockStunPlayerTargetSafetyZoneBlocksLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     901,
		Level:     80,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "safe-target",
		X:          101,
		Y:          100,
		MapID:      901,
		Level:      1,
		RegistStun: -100,
	})
	s := newSkillTestSystem(t, ws)
	s.deps.MapData = newShockStunSafetyMap(t)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("Java SHOCK_STUN 在安全區應被 checkZone 擋下，不應套用 87，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	packets := drainSkillTestPackets(caster.Session)
	if !hasGlobalSystemMessageText(packets, "在安全區域無法使用此技能。") {
		t.Fatal("Java SHOCK_STUN 安全區拒絕時應送 S_SystemMessage(\"在安全區域無法使用此技能。\")")
	}
	if countSkillEffectPackets(drainSkillTestPackets(target.Session), target.CharID, 4434) != 0 {
		t.Fatal("Java SHOCK_STUN 安全區拒絕時 targetList 為空，不應送目標 4434")
	}
}

func TestSkillClanShockStunPlayerTargetRangeMatchesJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     80,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "far-target",
		X:          102,
		Y:          100,
		MapID:      4,
		Level:      1,
		Sleeped:    true,
		RegistStun: -100,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 66, TicksLeft: 100, SetSleeped: true})
	target.AddBuff(&world.ActiveBuff{SkillID: 153, TicksLeft: 100})
	s := newSkillTestSystem(t, ws)
	combat := &shockStunCombatSpy{}
	s.deps.Combat = combat
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434, Ranged: 1}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("Java L1SkillUse 會以 ranged=1 擋下遠距 SHOCK_STUN，不應套用 87，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	if len(combat.requests) != 0 {
		t.Fatalf("射程外 SHOCK_STUN 不應進入 runSkill 觸發 onAction，requests=%d", len(combat.requests))
	}
	if !target.Sleeped || !target.HasBuff(66) || !target.HasBuff(153) {
		t.Fatalf("射程外 SHOCK_STUN 不應清除目標既有狀態，Sleeped=%v buff66=%v buff153=%v", target.Sleeped, target.HasBuff(66), target.HasBuff(153))
	}
	if countSkillEffectPackets(drainSkillTestPackets(target.Session), target.CharID, 4434) != 0 {
		t.Fatal("射程外 SHOCK_STUN 不應送目標 4434")
	}
}

func TestSkillClanShockStunNpcTargetRangeMatchesJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     80,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45000,
		Impl:          "L1Monster",
		Name:          "far-npc",
		X:             102,
		Y:             100,
		MapID:         4,
		Level:         1,
		Sleeped:       true,
		ActiveDebuffs: map[int32]int{66: 100, 153: 100},
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	combat := &shockStunCombatSpy{}
	s.deps.Combat = combat
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434, Ranged: 1}

	s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)

	if npc.HasDebuff(87) || npc.Paralyzed {
		t.Fatalf("Java L1SkillUse 會以 ranged=1 擋下遠距 NPC 目標 SHOCK_STUN，不應套用 87，debuff=%v Paralyzed=%v", npc.HasDebuff(87), npc.Paralyzed)
	}
	if len(combat.requests) != 0 {
		t.Fatalf("射程外 NPC 目標 SHOCK_STUN 不應進入 runSkill 觸發 onAction，requests=%d", len(combat.requests))
	}
	if !npc.Sleeped || !npc.HasDebuff(66) || !npc.HasDebuff(153) {
		t.Fatalf("射程外 NPC 目標 SHOCK_STUN 不應清除既有狀態，Sleeped=%v debuff66=%v debuff153=%v", npc.Sleeped, npc.HasDebuff(66), npc.HasDebuff(153))
	}
}

func TestSkillClanShockStunRangeFailureDoesNotConsumeMpLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "knight",
		X:           100,
		Y:           100,
		MapID:       4,
		Level:       80,
		MP:          15,
		MaxMP:       15,
		KnownSpells: []int32{87},
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "far-target",
		X:         102,
		Y:         100,
		MapID:     4,
		Level:     1,
	})
	weapon := caster.Inv.AddItemWithID(7001, 52, 1, "雙手劍", 0, 1000, false, 1)
	weapon.Equipped = true
	caster.Equip.Set(world.SlotWeapon, weapon)
	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  target.CharID,
	})

	if caster.MP != 15 {
		t.Fatalf("Java checkUseSkill 會在 useConsume 前以 ranged=1 擋下射程外 SHOCK_STUN，不應消耗 MP，MP=%d", caster.MP)
	}
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("射程外 SHOCK_STUN 不應套用 87，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
}

func TestSkillClanShockStunPlayerTargetBehindWallDoesNotConsumeMpLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:        1,
		Session:          newSkillTestSession(t, 1),
		CharID:           1001,
		Name:             "knight",
		X:                100,
		Y:                100,
		MapID:            900,
		Level:            99,
		MP:               15,
		MaxMP:            15,
		OriginalMagicHit: 100,
		KnownSpells:      []int32{87},
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "wall-target",
		X:         104,
		Y:         100,
		MapID:     900,
		Level:     1,
	})
	weapon := caster.Inv.AddItemWithID(7001, 52, 1, "雙手劍", 0, 1000, false, 1)
	weapon.Equipped = true
	caster.Equip.Set(world.SlotWeapon, weapon)
	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	s.deps.MapData = newSkillLOSTestMap(t)
	s.deps.Skills.Get(87).Ranged = 5

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  target.CharID,
	})

	if caster.MP != 15 {
		t.Fatalf("Java glanceCheck 擋下隔牆 SHOCK_STUN 時不應消耗 MP，MP=%d", caster.MP)
	}
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("隔牆 SHOCK_STUN 不應套用 87，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	if countSkillEffectPackets(drainSkillTestPackets(target.Session), target.CharID, 4434) != 0 {
		t.Fatal("隔牆 SHOCK_STUN 不應送目標 4434 特效")
	}
	if effects := ws.GetNearbyGroundEffects(target.X, target.Y, target.MapID); len(effects) != 0 {
		t.Fatalf("隔牆 SHOCK_STUN 不應建立 81162 效果 NPC，got=%d", len(effects))
	}
}

func TestSkillClanShockStunNpcTargetBehindWallDoesNotConsumeMpLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:        1,
		Session:          newSkillTestSession(t, 1),
		CharID:           1001,
		Name:             "knight",
		X:                100,
		Y:                100,
		MapID:            900,
		Level:            99,
		MP:               15,
		MaxMP:            15,
		OriginalMagicHit: 100,
		KnownSpells:      []int32{87},
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "wall-npc",
		X:     104,
		Y:     100,
		MapID: 900,
		Level: 1,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	weapon := caster.Inv.AddItemWithID(7001, 52, 1, "雙手劍", 0, 1000, false, 1)
	weapon.Equipped = true
	caster.Equip.Set(world.SlotWeapon, weapon)
	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	s.deps.MapData = newSkillLOSTestMap(t)
	s.deps.Skills.Get(87).Ranged = 5

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  npc.ID,
	})

	if caster.MP != 15 {
		t.Fatalf("Java glanceCheck 擋下隔牆 NPC 目標 SHOCK_STUN 時不應消耗 MP，MP=%d", caster.MP)
	}
	if npc.HasDebuff(87) || npc.Paralyzed {
		t.Fatalf("隔牆 NPC 目標 SHOCK_STUN 不應套用 87，debuff=%v Paralyzed=%v", npc.ActiveDebuffs[87], npc.Paralyzed)
	}
	if countSkillEffectPackets(drainSkillTestPackets(caster.Session), npc.ID, 4434) != 0 {
		t.Fatal("隔牆 NPC 目標 SHOCK_STUN 不應送目標 4434 特效")
	}
	if effects := ws.GetNearbyGroundEffects(npc.X, npc.Y, npc.MapID); len(effects) != 0 {
		t.Fatalf("隔牆 NPC 目標 SHOCK_STUN 不應建立 81162 效果 NPC，got=%d", len(effects))
	}
}

func TestSkillClanShockStunPlayerTargetDifferentShowIDDoesNotConsumeMpLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:        1,
		Session:          newSkillTestSession(t, 1),
		CharID:           1001,
		Name:             "knight",
		X:                100,
		Y:                100,
		MapID:            4,
		ShowID:           100,
		Level:            99,
		MP:               15,
		MaxMP:            15,
		OriginalMagicHit: 100,
		KnownSpells:      []int32{87},
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "other-show",
		X:          101,
		Y:          100,
		MapID:      4,
		ShowID:     200,
		Level:      1,
		RegistStun: -100,
	})
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  target.CharID,
	})

	if caster.MP != 15 {
		t.Fatalf("Java isTarget 會以 showId 不同擋下 SHOCK_STUN，不應消耗 MP，MP=%d", caster.MP)
	}
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("不同 showId 目標不應套用 87，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	if countSkillEffectPackets(drainSkillTestPackets(target.Session), target.CharID, 4434) != 0 {
		t.Fatal("不同 showId 目標不應收到 SHOCK_STUN 4434 特效")
	}
	if effects := ws.GetNearbyGroundEffects(target.X, target.Y, target.MapID); len(effects) != 0 {
		t.Fatalf("不同 showId 目標不應建立 81162 效果 NPC，got=%d", len(effects))
	}
}

func TestSkillClanShockStunNpcTargetDifferentShowIDDoesNotConsumeMpLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:        1,
		Session:          newSkillTestSession(t, 1),
		CharID:           1001,
		Name:             "knight",
		X:                100,
		Y:                100,
		MapID:            4,
		ShowID:           100,
		Level:            99,
		MP:               15,
		MaxMP:            15,
		OriginalMagicHit: 100,
		KnownSpells:      []int32{87},
	})
	npc := &world.NpcInfo{
		ID:     world.NextNpcID(),
		NpcID:  45000,
		Impl:   "L1Monster",
		Name:   "other-show-npc",
		X:      101,
		Y:      100,
		MapID:  4,
		ShowID: 200,
		Level:  1,
		HP:     100,
		MaxHP:  100,
	}
	ws.AddNpc(npc)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  npc.ID,
	})

	if caster.MP != 15 {
		t.Fatalf("Java isTarget 會以 NPC showId 不同擋下 SHOCK_STUN，不應消耗 MP，MP=%d", caster.MP)
	}
	if npc.HasDebuff(87) || npc.Paralyzed {
		t.Fatalf("不同 showId NPC 不應套用 87，debuff=%v Paralyzed=%v", npc.ActiveDebuffs[87], npc.Paralyzed)
	}
	if countSkillEffectPackets(drainSkillTestPackets(caster.Session), npc.ID, 4434) != 0 {
		t.Fatal("不同 showId NPC 不應收到 SHOCK_STUN 4434 特效")
	}
	if effects := ws.GetNearbyGroundEffects(npc.X, npc.Y, npc.MapID); len(effects) != 0 {
		t.Fatalf("不同 showId NPC 不應建立 81162 效果 NPC，got=%d", len(effects))
	}
}

func TestSkillClanShockStunNpcHiddenTargetDoesNotConsumeMpLikeJava(t *testing.T) {
	for _, tc := range []struct {
		name   string
		hidden int
	}{
		{name: "sink", hidden: world.NpcHiddenSink},
		{name: "fly", hidden: world.NpcHiddenFly},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rand.Seed(1)
			ws := world.NewState()
			caster := addSkillTestPlayer(ws, &world.PlayerInfo{
				SessionID:        1,
				Session:          newSkillTestSession(t, 1),
				CharID:           1001,
				Name:             "knight",
				X:                100,
				Y:                100,
				MapID:            4,
				Level:            99,
				MP:               15,
				MaxMP:            15,
				OriginalMagicHit: 100,
				KnownSpells:      []int32{87},
			})
			npc := &world.NpcInfo{
				ID:           world.NextNpcID(),
				NpcID:        45000,
				Impl:         "L1Monster",
				Name:         "hidden-npc",
				X:            101,
				Y:            100,
				MapID:        4,
				Level:        1,
				HP:           100,
				MaxHP:        100,
				HiddenStatus: tc.hidden,
			}
			ws.AddNpc(npc)
			caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
			s := newSkillBuffTestSystem(t, ws)
			attachShockStunItemTable(t, s)
			attachShockStunNpcTable(t, s)

			s.processSkill(handler.SkillRequest{
				SessionID: caster.SessionID,
				SkillID:   87,
				TargetID:  npc.ID,
			})

			if caster.MP != 15 {
				t.Fatalf("Java isTarget 會擋下 hidden NPC SHOCK_STUN，不應消耗 MP，MP=%d", caster.MP)
			}
			if npc.HasDebuff(87) || npc.Paralyzed {
				t.Fatalf("hidden NPC 不應套用 87，debuff=%v Paralyzed=%v", npc.ActiveDebuffs[87], npc.Paralyzed)
			}
			if countSkillEffectPackets(drainSkillTestPackets(caster.Session), npc.ID, 4434) != 0 {
				t.Fatal("hidden NPC 不應收到 SHOCK_STUN 4434 特效")
			}
			if effects := ws.GetNearbyGroundEffects(npc.X, npc.Y, npc.MapID); len(effects) != 0 {
				t.Fatalf("hidden NPC 不應建立 81162 效果 NPC，got=%d", len(effects))
			}
		})
	}
}

func TestSkillClanShockStunNpcHiddenTargetDirectPathReturnsBeforeOnActionLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:        1,
		Session:          newSkillTestSession(t, 1),
		CharID:           1001,
		Name:             "knight",
		X:                100,
		Y:                100,
		MapID:            4,
		Level:            99,
		OriginalMagicHit: 100,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45000,
		Impl:          "L1Monster",
		Name:          "hidden-npc",
		X:             101,
		Y:             100,
		MapID:         4,
		Level:         1,
		HP:            100,
		MaxHP:         100,
		HiddenStatus:  world.NpcHiddenSink,
		Sleeped:       true,
		ActiveDebuffs: map[int32]int{66: 100, 153: 100},
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	combat := &shockStunCombatSpy{}
	s.deps.Combat = combat
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434, Ranged: 1}

	s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)

	if npc.HasDebuff(87) || npc.Paralyzed {
		t.Fatalf("hidden NPC 目標 SHOCK_STUN 不應套用 87，debuff=%v Paralyzed=%v", npc.HasDebuff(87), npc.Paralyzed)
	}
	if len(combat.requests) != 0 {
		t.Fatalf("hidden NPC 目標 SHOCK_STUN 不應進入 runSkill 觸發 onAction，requests=%d", len(combat.requests))
	}
	if !npc.Sleeped || !npc.HasDebuff(66) || !npc.HasDebuff(153) {
		t.Fatalf("hidden NPC 目標 SHOCK_STUN 不應清除既有狀態，Sleeped=%v debuff66=%v debuff153=%v", npc.Sleeped, npc.HasDebuff(66), npc.HasDebuff(153))
	}
	if effects := ws.GetNearbyGroundEffects(npc.X, npc.Y, npc.MapID); len(effects) != 0 {
		t.Fatalf("hidden NPC 目標 SHOCK_STUN 不應建立 81162 效果 NPC，got=%d", len(effects))
	}
}

func TestSkillClanShockStunNpcRejectedTargetDoesNotConsumeMpLikeJava(t *testing.T) {
	for _, tc := range []struct {
		name  string
		impl  string
		maxHP int32
	}{
		{name: "effect", impl: "L1Effect", maxHP: 100},
		{name: "illusory", impl: "L1Illusory", maxHP: 100},
		{name: "doll", impl: "L1Doll", maxHP: 100},
		{name: "hierarch", impl: "L1Hierarch", maxHP: 100},
		{name: "undestroyable-door-zero-hp", impl: "L1Door", maxHP: 0},
		{name: "undestroyable-door-one-hp", impl: "L1Door", maxHP: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rand.Seed(1)
			ws := world.NewState()
			caster := addSkillTestPlayer(ws, &world.PlayerInfo{
				SessionID:        1,
				Session:          newSkillTestSession(t, 1),
				CharID:           1001,
				Name:             "knight",
				X:                100,
				Y:                100,
				MapID:            4,
				Level:            99,
				MP:               15,
				MaxMP:            15,
				OriginalMagicHit: 100,
				KnownSpells:      []int32{87},
			})
			npc := &world.NpcInfo{
				ID:    world.NextNpcID(),
				NpcID: 45000,
				Impl:  tc.impl,
				Name:  tc.name,
				X:     101,
				Y:     100,
				MapID: 4,
				Level: 1,
				HP:    tc.maxHP,
				MaxHP: tc.maxHP,
			}
			ws.AddNpc(npc)
			caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
			s := newSkillBuffTestSystem(t, ws)
			attachShockStunItemTable(t, s)
			attachShockStunNpcTable(t, s)

			s.processSkill(handler.SkillRequest{
				SessionID: caster.SessionID,
				SkillID:   87,
				TargetID:  npc.ID,
			})

			if caster.MP != 15 {
				t.Fatalf("Java isTarget() rejects %s before useConsume; MP=%d", tc.impl, caster.MP)
			}
			if npc.HasDebuff(87) || npc.Paralyzed {
				t.Fatalf("Java rejected %s should not receive SHOCK_STUN; debuff=%v Paralyzed=%v", tc.impl, npc.HasDebuff(87), npc.Paralyzed)
			}
			if countSkillEffectPackets(drainSkillTestPackets(caster.Session), npc.ID, 4434) != 0 {
				t.Fatalf("Java rejected %s should not receive SHOCK_STUN 4434", tc.impl)
			}
			if effects := ws.GetNearbyGroundEffects(npc.X, npc.Y, npc.MapID); len(effects) != 0 {
				t.Fatalf("Java rejected %s should not spawn 81162 effect; got=%d", tc.impl, len(effects))
			}
		})
	}
}

func TestSkillClanShockStunNpcRejectedTargetDirectPathReturnsBeforeOnActionLikeJava(t *testing.T) {
	for _, tc := range []struct {
		name  string
		impl  string
		maxHP int32
	}{
		{name: "effect", impl: "L1Effect", maxHP: 100},
		{name: "illusory", impl: "L1Illusory", maxHP: 100},
		{name: "doll", impl: "L1Doll", maxHP: 100},
		{name: "hierarch", impl: "L1Hierarch", maxHP: 100},
		{name: "undestroyable-door-zero-hp", impl: "L1Door", maxHP: 0},
		{name: "undestroyable-door-one-hp", impl: "L1Door", maxHP: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rand.Seed(1)
			ws := world.NewState()
			caster := addSkillTestPlayer(ws, &world.PlayerInfo{
				SessionID:        1,
				Session:          newSkillTestSession(t, 1),
				CharID:           1001,
				Name:             "knight",
				X:                100,
				Y:                100,
				MapID:            4,
				Level:            99,
				OriginalMagicHit: 100,
			})
			npc := &world.NpcInfo{
				ID:            world.NextNpcID(),
				NpcID:         45000,
				Impl:          tc.impl,
				Name:          tc.name,
				X:             101,
				Y:             100,
				MapID:         4,
				Level:         1,
				HP:            tc.maxHP,
				MaxHP:         tc.maxHP,
				Sleeped:       true,
				ActiveDebuffs: map[int32]int{66: 100, 153: 100},
			}
			ws.AddNpc(npc)
			s := newSkillTestSystem(t, ws)
			combat := &shockStunCombatSpy{}
			s.deps.Combat = combat
			attachShockStunItemTable(t, s)
			caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
			skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434, Ranged: 1}

			s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)

			if npc.HasDebuff(87) || npc.Paralyzed {
				t.Fatalf("Java rejected %s should not receive SHOCK_STUN; debuff=%v Paralyzed=%v", tc.impl, npc.HasDebuff(87), npc.Paralyzed)
			}
			if len(combat.requests) != 0 {
				t.Fatalf("Java rejected %s should return before onAction; requests=%d", tc.impl, len(combat.requests))
			}
			if !npc.Sleeped || !npc.HasDebuff(66) || !npc.HasDebuff(153) {
				t.Fatalf("Java rejected %s should return before runSkill side effects; Sleeped=%v debuff66=%v debuff153=%v", tc.impl, npc.Sleeped, npc.HasDebuff(66), npc.HasDebuff(153))
			}
			if effects := ws.GetNearbyGroundEffects(npc.X, npc.Y, npc.MapID); len(effects) != 0 {
				t.Fatalf("Java rejected %s should not spawn 81162 effect; got=%d", tc.impl, len(effects))
			}
		})
	}
}

func TestSkillClanShockStunPlayerSkipsDoorTargetAfterOnActionLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "knight",
		AccessLevel: 200,
		Level:       99,
		X:           100,
		Y:           100,
		MapID:       4,
	})
	door := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 81012,
		Impl:  "L1Door",
		Name:  "door",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 1,
		HP:    1000,
		MaxHP: 1000,
	}
	ws.AddNpc(door)
	combat := &shockStunCombatSpy{}
	s := newSkillTestSystem(t, ws)
	s.deps.Combat = combat
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, door.ID)

	if door.HasDebuff(87) || door.Paralyzed {
		t.Fatalf("Java isTargetFailure() returns true for L1DoorInstance probability skills; debuff=%v Paralyzed=%v", door.HasDebuff(87), door.Paralyzed)
	}
	if effects := ws.GetNearbyGroundEffects(door.X, door.Y, door.MapID); len(effects) != 0 {
		t.Fatalf("Java SHOCK_STUN should not spawn 81162 on L1DoorInstance; got=%d", len(effects))
	}
	packets := drainSkillTestPackets(caster.Session)
	if hasSkillEffectPacket(packets, door.ID, 4434) {
		t.Fatal("Java sendGrfx() returns when target list is empty after isTargetFailure; door should not receive 4434")
	}
	if hasShockStunGMDurationMessage(packets) {
		t.Fatal("Java SHOCK_STUN should not send GM duration for L1DoorInstance")
	}
	if len(combat.requests) != 1 || combat.requests[0].TargetID != door.ID || !combat.requests[0].IsMelee {
		t.Fatalf("Java _target.onAction(_player) still runs before isTargetFailure for L1DoorInstance; requests=%+v", combat.requests)
	}
}

func TestSkillClanShockStunPlayerTargetClearsSleepLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     80,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "sleep-target",
		X:          101,
		Y:          100,
		MapID:      4,
		Level:      1,
		RegistStun: -100,
		Sleeped:    true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 66, TicksLeft: 100, SetSleeped: true})
	target.AddBuff(&world.ActiveBuff{SkillID: 103, TicksLeft: 100, SetSleeped: true})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if target.Sleeped || target.HasBuff(66) || target.HasBuff(103) {
		t.Fatalf("Java L1SkillUse.runSkill() 會在 SHOCK_STUN 結果處理前移除睡眠效果，Sleeped=%v buff66=%v buff103=%v", target.Sleeped, target.HasBuff(66), target.HasBuff(103))
	}
	if !hasParalysisSubtype(drainSkillTestPackets(target.Session), handler.SleepRemove) {
		t.Fatal("SHOCK_STUN 清除玩家睡眠效果時應送 S_Paralysis SleepRemove")
	}
}

// Java L1SkillUse.runSkill() 第 1965-1968 行對 TYPE_PROBABILITY 技能（含 SHOCK_STUN）
// 在概率結果處理前清除目標 `FOG_OF_SLEEPING(66)` / `PHANTASM(212)` / `103` 三個睡眠類效果。
// 既有玩家/NPC 目標 ClearsSleepLikeJava 測試只覆蓋 66/103；本測試補上 PHANTASM(212) 玩家目標清除，
// 以及對等的 NPC 目標清除，避免 Java 第 1967 行 `cha.removeSkillEffect(212)` 在 Go 被遺漏。
func TestSkillClanShockStunPlayerTargetClearsPhantasmLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     80,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "phantasm-target",
		X:          101,
		Y:          100,
		MapID:      4,
		Level:      1,
		RegistStun: -100,
		Sleeped:    true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 212, TicksLeft: 100, SetSleeped: true})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if target.HasBuff(212) {
		t.Fatal("Java L1SkillUse.runSkill() 第 1967 行會在 SHOCK_STUN 結果處理前移除玩家目標 PHANTASM(212) 效果")
	}
}

func TestSkillClanShockStunNpcTargetClearsPhantasmLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     80,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45000,
		Impl:          "L1Monster",
		Name:          "phantasm-npc",
		X:             101,
		Y:             100,
		MapID:         4,
		Level:         1,
		ActiveDebuffs: map[int32]int{212: 100},
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)

	if npc.HasDebuff(212) {
		t.Fatal("Java L1SkillUse.runSkill() 第 1967 行會在 SHOCK_STUN NPC 目標結果處理前移除 PHANTASM(212) 效果")
	}
}

func TestSkillClanShockStunNpcTargetClearsSleepLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     80,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45000,
		Impl:          "L1Monster",
		Name:          "sleep-npc",
		X:             101,
		Y:             100,
		MapID:         4,
		Level:         1,
		Sleeped:       true,
		ActiveDebuffs: map[int32]int{66: 100, 103: 100},
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)

	if npc.Sleeped || npc.HasDebuff(66) || npc.HasDebuff(103) {
		t.Fatalf("Java L1SkillUse.runSkill() 會在 SHOCK_STUN NPC 目標結果處理前移除睡眠效果，Sleeped=%v debuff66=%v debuff103=%v", npc.Sleeped, npc.HasDebuff(66), npc.HasDebuff(103))
	}
}

func TestSkillClanShockStunPlayerTargetClearsEraseMagicLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     80,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "erase-target",
		X:          101,
		Y:          100,
		MapID:      4,
		Level:      1,
		RegistStun: -100,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 153, TicksLeft: 100})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if target.HasBuff(153) {
		t.Fatal("Java L1SkillUse.runSkill() 會在 SHOCK_STUN 概率技能結果處理時移除目標 ERASE_MAGIC")
	}
}

func TestSkillClanShockStunNpcTargetClearsEraseMagicLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     80,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45000,
		Impl:          "L1Monster",
		Name:          "erase-npc",
		X:             101,
		Y:             100,
		MapID:         4,
		Level:         1,
		ActiveDebuffs: map[int32]int{153: 100},
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)

	if npc.HasDebuff(153) {
		t.Fatal("Java L1SkillUse.runSkill() 會在 SHOCK_STUN NPC 目標概率技能結果處理時移除目標 ERASE_MAGIC")
	}
}

func TestSkillClanShockStunPlayerSuccessIgnoresMRWhenJavaImpactHaloMRZero(t *testing.T) {
	caster := &world.PlayerInfo{Level: 60, Intel: 18}
	target := &world.PlayerInfo{Level: 50, MR: 100}

	if got := shockStunPlayerProbability(caster, target); got != 40 {
		t.Fatalf("Java 預設 IMPACT_HALO_MR=0，SHOCK_STUN 不應扣 MR，got=%d want=40", got)
	}
}

// Java `L1MagicPc.calcProbabilityMagic()` 第 949-961 行 `case PC_PC: case SHOCK_STUN:`
// 明確執行 `probability -= _targetPc.getRegistStun()`。既有測試覆蓋 NPC 施放對玩家
// RegistStun（`TestSkillClanShockStunNpcCasterUsesJavaStunResistProbability` 走端到端
// integration），本步補上 PC_PC 路徑的 unit 級回歸：`shockStunPlayerProbability`
// 對 target.RegistStun 直接做 1:1 扣除，避免未來改成乘倍率或忽略。
func TestSkillClanShockStunPlayerProbabilitySubtractsRegistStunLikeJava(t *testing.T) {
	caster := &world.PlayerInfo{Level: 60, Intel: 0}
	target := &world.PlayerInfo{Level: 50, RegistStun: 10}

	// Java PC_PC 計算：caster Level 60 > target Level 50 → IMPACT_HALO_1 = 40
	// + INT 0 → 任何 INT 加成皆 0
	// - target.RegistStun 10
	// = 40 - 10 = 30
	if got := shockStunPlayerProbability(caster, target); got != 30 {
		t.Fatalf("Java PC_PC SHOCK_STUN 應從機率減去 target.getRegistStun()，got=%d want=30 (40 - 10)", got)
	}
}

// Java L1MagicPc.calcProbabilityMagic() case SHOCK_STUN 第 649-651 行：
//
//	if (ConfigSkill.IMPACT_HALO_INT > 0) { probability += IMPACT_HALO_INT * _pc.getInt(); }
//
// yiwei `各職業技能相關設置.properties` `IMPACT_HALO_INT = 0`，整個 if block 被略過。
// 既有 `IgnoresMRWhenJavaImpactHaloMRZero` 對齊 IMPACT_HALO_MR=0；本測試補上 INT
// 對等的回歸：高 INT 只走 `BaseInt 純 INT 加成` 與 `L1AttackList.INTH` 表格，
// 不應再被任何 `IMPACT_HALO_INT * INT` 線性倍率影響。
func TestSkillClanShockStunPlayerSuccessIgnoresIntMultiplierWhenJavaImpactHaloIntZero(t *testing.T) {
	caster := &world.PlayerInfo{Level: 50, Intel: 127}
	target := &world.PlayerInfo{Level: 50}
	npc := &world.NpcInfo{Level: 50}

	// 同等級 → IMPACT_HALO_2 = 30
	// + BaseInt 45+ → +5
	// + IntMagicHit (127-20)/3 = 35
	// + OriginalMagicHit 0 - RegistStun 0
	// 預期 = 30 + 5 + 35 = 70；任何 `IMPACT_HALO_INT * 127` 倍率都會把結果推高並封頂於 100。
	if got := shockStunPlayerProbability(caster, target); got != 70 {
		t.Fatalf("Java IMPACT_HALO_INT=0，高 INT 玩家目標成功率不應有額外倍率，got=%d want=70", got)
	}
	if got := shockStunNpcProbability(caster, npc); got != 70 {
		t.Fatalf("Java IMPACT_HALO_INT=0，高 INT NPC 目標成功率不應有額外倍率，got=%d want=70", got)
	}
}

func TestSkillClanShockStunPlayerSuccessUsesJavaStunLevel(t *testing.T) {
	caster := &world.PlayerInfo{Level: 49, StunLevel: 2}
	target := &world.PlayerInfo{Level: 50}

	if got := shockStunPlayerProbability(caster, target); got != 40 {
		t.Fatalf("Java SHOCK_STUN 會以攻方 Level + StunLevel 判定高低等成功率，got=%d want=40", got)
	}
}

func TestSkillClanShockStunPlayerSuccessUsesJavaOriginalMagicHit(t *testing.T) {
	caster := &world.PlayerInfo{Level: 50, OriginalMagicHit: 3}
	target := &world.PlayerInfo{Level: 50}
	npc := &world.NpcInfo{Level: 50}

	if got := shockStunPlayerProbability(caster, target); got != 33 {
		t.Fatalf("Java SHOCK_STUN 玩家目標成功率會加 getOriginalMagicHit，got=%d want=33", got)
	}
	if got := shockStunNpcProbability(caster, npc); got != 33 {
		t.Fatalf("Java SHOCK_STUN NPC 目標成功率會加 getOriginalMagicHit，got=%d want=33", got)
	}
}

func TestSkillClanShockStunPlayerSuccessUsesJavaIntMagicHitTable(t *testing.T) {
	caster := &world.PlayerInfo{Level: 50, Intel: 23}
	target := &world.PlayerInfo{Level: 50}
	npc := &world.NpcInfo{Level: 50}

	if got := shockStunPlayerProbability(caster, target); got != 31 {
		t.Fatalf("Java SHOCK_STUN 玩家目標成功率會加 L1AttackList.INTH 的 INT 魔法命中，got=%d want=31", got)
	}
	if got := shockStunNpcProbability(caster, npc); got != 31 {
		t.Fatalf("Java SHOCK_STUN NPC 目標成功率會加 L1AttackList.INTH 的 INT 魔法命中，got=%d want=31", got)
	}
}

func TestSkillClanShockStunPlayerSuccessUsesJavaBaseIntForPureIntBonus(t *testing.T) {
	caster := &world.PlayerInfo{
		Level: 50,
		Intel: 25,
		EquipBonuses: world.EquipStats{
			AddInt: 7,
		},
	}
	target := &world.PlayerInfo{Level: 50}
	npc := &world.NpcInfo{Level: 50}

	if got := shockStunPlayerProbability(caster, target); got != 31 {
		t.Fatalf("Java SHOCK_STUN 純 INT 加成使用 getBaseInt，不應把裝備 INT 算入純 INT 加成，got=%d want=31", got)
	}
	if got := shockStunNpcProbability(caster, npc); got != 31 {
		t.Fatalf("Java SHOCK_STUN NPC 目標純 INT 加成使用 getBaseInt，不應把裝備 INT 算入純 INT 加成，got=%d want=31", got)
	}
}

func TestSkillClanShockStunPlayerMissDoesNotSendTargetEffect(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     60,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
		Level:     50,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("測試前提錯誤：此 seed 應讓 SHOCK_STUN miss，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	casterPackets := drainSkillTestPackets(caster.Session)
	targetPackets := drainSkillTestPackets(target.Session)
	if hasServerMessage(casterPackets, skillMsgCastFail) {
		t.Fatal("Java SHOCK_STUN miss 時 _targetList 為空，不會送 S_ServerMessage(280)")
	}
	if hasActionGfxPacket(casterPackets, caster.CharID, 19) {
		t.Fatal("Java SHOCK_STUN miss 時 sendGrfx 會在 _targetList.size()==0 直接 return，不應送施法者 S_DoActionGFX")
	}
	if hasSkillEffectPacket(targetPackets, target.CharID, 4434) {
		t.Fatal("Java SHOCK_STUN miss 時 _targetList 為空，不應送出目標 S_SkillSound(4434)")
	}
}

func TestSkillClanShockStunNpcSuccessUsesJavaImpactHaloRate(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     60,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "stun target",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 50,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, npc.ID)

	if npc.HasDebuff(87) || npc.Paralyzed {
		t.Fatalf("Java SHOCK_STUN 對 NPC 目標也以 IMPACT_HALO_1=40 為基準，不應使用 Go 一般 NPC 公式 95%%，debuff=%v Paralyzed=%v", npc.HasDebuff(87), npc.Paralyzed)
	}
	casterPackets := drainSkillTestPackets(caster.Session)
	if hasServerMessage(casterPackets, skillMsgCastFail) {
		t.Fatal("Java SHOCK_STUN NPC 目標 miss 時 _targetList 為空，不會送 S_ServerMessage(280)")
	}
	if hasActionGfxPacket(casterPackets, caster.CharID, 19) {
		t.Fatal("Java SHOCK_STUN NPC 目標 miss 時 sendGrfx 會在 _targetList.size()==0 直接 return，不應送施法者 S_DoActionGFX")
	}
}

func TestSkillClanShockStunNpcSuccessIgnoresMRWhenJavaImpactHaloMRZero(t *testing.T) {
	caster := &world.PlayerInfo{Level: 60, Intel: 18}
	npc := &world.NpcInfo{Level: 50, MR: 100}

	if got := shockStunNpcProbability(caster, npc); got != 40 {
		t.Fatalf("Java 預設 IMPACT_HALO_MR=0，SHOCK_STUN NPC 成功率不應扣 MR，got=%d want=40", got)
	}
}

func TestSkillClanShockStunPlayerApplySendsStunApplyPacketLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "target",
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -100,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.applyShockStunToPlayer(caster.Session, caster, target, skill, ws.GetNearbyPlayersAt(target.X, target.Y, target.MapID))

	if !hasParalysisSubtype(drainSkillTestPackets(target.Session), handler.StunApply) {
		t.Fatal("Java SHOCK_STUN.start(L1PcInstance,...) 對玩家目標會送 S_Paralysis(5, true) = 0x16 (StunApply)")
	}
}

func TestSkillClanShockStunNpcCasterApplySendsStunApplyPacketLikeJava(t *testing.T) {
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "victim",
		X:          100,
		Y:          100,
		MapID:      4,
		Level:      50,
		RegistStun: -100,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "stunner",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 99,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434, Ranged: 10}

	for seed := int64(1); seed <= 100; seed++ {
		rand.Seed(seed)
		target.Paralyzed = false
		target.ActiveBuffs = nil
		_ = drainSkillTestPackets(target.Session)

		s.ApplyNpcShockStun(npc, target, skill, 10)
		if !target.HasBuff(87) {
			continue
		}
		if !hasParalysisSubtype(drainSkillTestPackets(target.Session), handler.StunApply) {
			t.Fatal("Java SHOCK_STUN.start(L1NpcInstance,...) 對玩家目標會送 S_Paralysis(5, true) = 0x16 (StunApply)")
		}
		return
	}
	t.Fatal("測試種子 1..100 未觸發 NPC 施放 SHOCK_STUN 成功，無法驗證 StunApply 封包")
}

func TestSkillClanShockStunPlayerExpiryClearsParalyzedLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "target",
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -100,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.applyShockStunToPlayer(caster.Session, caster, target, skill, ws.GetNearbyPlayersAt(target.X, target.Y, target.MapID))
	if !target.Paralyzed || !target.HasBuff(87) {
		t.Fatalf("Java SHOCK_STUN 套用後 target 應被 setParalyzed(true)，Paralyzed=%v HasBuff(87)=%v", target.Paralyzed, target.HasBuff(87))
	}
	buff := target.GetBuff(87)
	if buff == nil {
		t.Fatal("套用 87 buff 失敗")
	}
	buff.TicksLeft = 1
	_ = drainSkillTestPackets(target.Session)

	s.tickPlayerBuffs(target)

	if target.Paralyzed || target.HasBuff(87) {
		t.Fatalf("Java SHOCK_STUN.stop() 對 L1PcInstance 會送 S_Paralysis(5, false) 並清 Paralyzed，Paralyzed=%v HasBuff(87)=%v", target.Paralyzed, target.HasBuff(87))
	}
	if !hasParalysisSubtype(drainSkillTestPackets(target.Session), handler.StunRemove) {
		t.Fatal("Java SHOCK_STUN.stop() 對 L1PcInstance 會送 S_Paralysis(5, false) = 0x17 (StunRemove)")
	}
}

func TestSkillClanShockStunNpcExpiryClearsParalyzedLikeJava(t *testing.T) {
	ws := world.NewState()
	npc := &world.NpcInfo{
		ID:        world.NextNpcID(),
		NpcID:     45000,
		Impl:      "L1Monster",
		Name:      "stun target",
		X:         101,
		Y:         100,
		MapID:     4,
		Level:     50,
		HP:        100,
		MaxHP:     100,
		Paralyzed: true,
	}
	ws.AddNpc(npc)
	npc.AddDebuff(87, 1)

	tickNpcDebuffs(npc, ws, &handler.Deps{World: ws})

	if npc.Paralyzed {
		t.Fatal("Java SHOCK_STUN.stop() 對 L1MonsterInstance/L1SummonInstance/L1GuardianInstance/L1GuardInstance/L1PetInstance 會 setParalyzed(false)，到期後不應保留 Paralyzed=true")
	}
	if npc.HasDebuff(87) {
		t.Fatalf("87 debuff 到期後應由 tickNpcDebuffs 移除，殘留 ticks=%d", npc.ActiveDebuffs[87])
	}
}

func TestSkillClanShockStunPlayerSkipsTowerTargetLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "knight",
		AccessLevel: 200,
		Level:       99,
		X:           100,
		Y:           100,
		MapID:       4,
	})
	tower := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 81011,
		Impl:  "L1Tower",
		Name:  "守護塔",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 1,
		HP:    1000,
		MaxHP: 1000,
	}
	ws.AddNpc(tower)
	combat := &shockStunCombatSpy{}
	s := newSkillTestSystem(t, ws)
	s.deps.Combat = combat
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, tower.ID)

	if tower.HasDebuff(87) || tower.Paralyzed {
		t.Fatalf("Java isTargetFailure() 對 L1TowerInstance 回傳 true，SHOCK_STUN 不應套用 87 debuff 或設 Paralyzed，debuff=%v Paralyzed=%v", tower.HasDebuff(87), tower.Paralyzed)
	}
	if effects := ws.GetNearbyGroundEffects(tower.X, tower.Y, tower.MapID); len(effects) != 0 {
		t.Fatalf("Java SHOCK_STUN 對守護塔目標不會 spawnEffect(81162)，got %d 筆地面效果", len(effects))
	}
	packets := drainSkillTestPackets(caster.Session)
	if hasSkillEffectPacket(packets, tower.ID, 4434) {
		t.Fatal("Java sendGrfx() 在 _targetList.size()==0 時直接 return，守護塔目標不應收到 4434")
	}
	if hasShockStunGMDurationMessage(packets) {
		t.Fatal("Java SHOCK_STUN 守護塔目標不會進入 .start() 流程，GM 不應收到秒數訊息")
	}
	if len(combat.requests) != 1 || combat.requests[0].TargetID != tower.ID || !combat.requests[0].IsMelee {
		t.Fatalf("Java _target.onAction(_player) 仍會在迴圈外觸發 SHOCK_STUN 對守護塔的近戰排程，got requests=%+v", combat.requests)
	}
}

func TestSkillClanShockStunPlayerCreatesJavaEffectNpc(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.applyShockStunToPlayer(caster.Session, caster, target, skill, ws.GetNearbyPlayersAt(target.X, target.Y, target.MapID))

	effects := ws.GetNearbyGroundEffects(target.X, target.Y, target.MapID)
	if len(effects) != 1 {
		t.Fatalf("Java SHOCK_STUN 成功時應 spawnEffect(81162) 到目標座標，got effects=%d", len(effects))
	}
	if effects[0].NpcID != 81162 || effects[0].GfxID != 4183 || effects[0].SkillID != 87 {
		t.Fatalf("SHOCK_STUN 地面效果資料錯誤: %+v", effects[0])
	}
	if effects[0].TicksLeft < 5 || effects[0].TicksLeft > 25 {
		t.Fatalf("SHOCK_STUN 81162 效果時間應等於 1~5 秒暈眩，ticks=%d", effects[0].TicksLeft)
	}
}

func TestSkillClanShockStunPlayerSuccessUsesJavaFixedTargetGfx(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 0}

	s.applyShockStunToPlayer(caster.Session, caster, target, skill, ws.GetNearbyPlayersAt(target.X, target.Y, target.MapID))

	casterPackets := drainSkillTestPackets(caster.Session)
	targetPackets := drainSkillTestPackets(target.Session)
	if hasActionGfxPacket(casterPackets, caster.CharID, 19) {
		t.Fatal("Java SHOCK_STUN 成功時 sendGrfx 特例只送目標 S_SkillSound(4434)，不送施法者 S_DoActionGFX")
	}
	if hasActionGfxPacket(targetPackets, caster.CharID, 19) {
		t.Fatal("Java SHOCK_STUN 成功時目標不應收到施法者 S_DoActionGFX")
	}
	if !hasSkillEffectPacket(targetPackets, target.CharID, 4434) {
		t.Fatal("Java SHOCK_STUN 成功時固定送目標 S_SkillSound(4434)，不依技能資料 CastGfx")
	}
}

func TestSkillClanShockStunPlayerTargetGfxBroadcastsFromTargetLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "target",
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -100,
	})
	observerNearTarget := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    1003,
		Name:      "observer",
		X:         121,
		Y:         100,
		MapID:     4,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 0}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	packets := drainSkillTestPackets(observerNearTarget.Session)
	if !hasSkillEffectPacket(packets, target.CharID, 4434) {
		t.Fatal("Java SHOCK_STUN 玩家目標 4434 由目標 sendPacketsAll 廣播，看得到目標的玩家應收到")
	}
}

func TestSkillClanShockStunPlayerTargetEarthBindBlocksLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		Level:     80,
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -100,
		Paralyzed:  true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 157, TicksLeft: 100, SetParalyzed: true})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 0}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if target.HasBuff(87) {
		t.Fatal("Java 目標已有 EARTH_BIND 時 SHOCK_STUN 應在 calcProbabilityMagic 返回 false，不應套用 87")
	}
	if hasSkillEffectPacket(drainSkillTestPackets(target.Session), target.CharID, 4434) {
		t.Fatal("Java 目標已有 EARTH_BIND 時 SHOCK_STUN targetList 為空，不應送目標 4434")
	}
}

// Java `L1MagicPc.calcProbabilityMagic()` 對玩家或 NPC 目標已有 `ICE_LANCE(50)` 或 `EARTH_BIND(157)`
// 都會讓 SHOCK_STUN 判定失敗。既有 `TestSkillClanShockStunPlayerTargetEarthBindBlocksLikeJava`
// 只覆蓋 buff 157，本測試補上玩家目標 buff 50 (ICE_LANCE) 的對等 negative case。
func TestSkillClanShockStunPlayerTargetIceLanceBlocksLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		Level:     80,
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "ice-target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -100,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 50, TicksLeft: 100})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 0}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if target.HasBuff(87) {
		t.Fatal("Java 目標已有 ICE_LANCE(50) 時 SHOCK_STUN 應在 calcProbabilityMagic 返回 false，不應套用 87")
	}
	if hasSkillEffectPacket(drainSkillTestPackets(target.Session), target.CharID, 4434) {
		t.Fatal("Java 目標已有 ICE_LANCE(50) 時 SHOCK_STUN targetList 為空，不應送目標 4434")
	}
}

func TestSkillClanShockStunPlayerTargetAbsoluteBarrierBlocksLikeJava(t *testing.T) {
	disablePlayerDebuffMRForStatusTest(t, 87)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:       2,
		Session:         newSkillTestSession(t, 2),
		CharID:          1002,
		Name:            "target",
		X:               101,
		Y:               100,
		MapID:           4,
		AbsoluteBarrier: true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 78, TicksLeft: 25, SetAbsoluteBarrier: true})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 0}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("Java L1SkillUse.checkTarget() 會排除 ABSOLUTE_BARRIER 目標，SHOCK_STUN 不應套用，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	if hasSkillEffectPacket(drainSkillTestPackets(target.Session), target.CharID, 4434) {
		t.Fatal("Java SHOCK_STUN 對 ABSOLUTE_BARRIER 目標 targetList 為空，不應送目標 4434")
	}
}

func TestSkillClanShockStunPlayerTargetAbsoluteBarrierDoesNotConsumeMpLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "knight",
		X:           100,
		Y:           100,
		MapID:       4,
		MP:          100,
		MaxMP:       100,
		KnownSpells: []int32{87},
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:       2,
		Session:         newSkillTestSession(t, 2),
		CharID:          1002,
		Name:            "barrier-target",
		X:               101,
		Y:               100,
		MapID:           4,
		AbsoluteBarrier: true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 78, TicksLeft: 25, SetAbsoluteBarrier: true})
	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  target.CharID,
	})

	if caster.MP != 100 {
		t.Fatalf("Java checkTarget 會在 useConsume 前排除 ABSOLUTE_BARRIER 目標，不應消耗 MP，MP=%d", caster.MP)
	}
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("ABSOLUTE_BARRIER 目標不應被 SHOCK_STUN 套用，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
}

func TestSkillClanShockStunUnknownSpellDoesNotCancelCasterAbsoluteBarrierLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:       1,
		Session:         newSkillTestSession(t, 1),
		CharID:          1001,
		Name:            "unknown-shock-stun",
		X:               100,
		Y:               100,
		MapID:           4,
		MP:              100,
		MaxMP:           100,
		AbsoluteBarrier: true,
		KnownSpells:     []int32{},
	})
	caster.AddBuff(&world.ActiveBuff{SkillID: 78, TicksLeft: 20, SetAbsoluteBarrier: true})
	s := newSkillBuffTestSystem(t, ws)

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  caster.CharID,
	})

	if !caster.AbsoluteBarrier || !caster.HasBuff(78) {
		t.Fatalf("Java C_UseSkill 會先以 isSkillMastery 擋下未學會的 SHOCK_STUN，不應解除施法者絕對屏障，AbsoluteBarrier=%v buff78=%v", caster.AbsoluteBarrier, caster.HasBuff(78))
	}
	if hasServerMessage(drainSkillTestPackets(caster.Session), skillMsgCastFail) {
		t.Fatal("Java C_UseSkill 在 isSkillMastery=false 時直接返回，不應送 S_ServerMessage(280)")
	}
}

// Java `C_UseSkill.start()` 的 `useConsume()` 在 `runSkill()` 概率判定之前執行，
// 因此玩家施放 SHOCK_STUN 對 valid target 即使概率失敗（命中 0%），MP 仍會被扣除。
// 既有測試已覆蓋「invalid target 不消耗 MP」（射程外、絕對屏障目標），但
// 「valid target + miss → 仍消耗 MP」的正面案例沒被鎖定，本測試補上。
func TestSkillClanShockStunPlayerMissStillConsumesMpLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "low-level-caster",
		X:           100,
		Y:           100,
		MapID:       4,
		MP:          100,
		MaxMP:       100,
		Level:       1,
		Intel:       12, // INT<=12 不享受 SHOCK_STUN MP 減免
		KnownSpells: []int32{87},
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  2,
		Session:    newSkillTestSession(t, 2),
		CharID:     1002,
		Name:       "high-resist-target",
		X:          101,
		Y:          100,
		MapID:      4,
		Level:      99,  // 攻方 1 vs 防方 99 → IMPACT_HALO_3 = 10
		RegistStun: 100, // 再扣 100 暈眩抗性 → 命中概率被夾到 0%（必失敗）
	})
	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  target.CharID,
	})

	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("RegistStun=100 + 等級差使命中率封底為 0，目標不應被套 87，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	if caster.MP != 85 {
		t.Fatalf("Java useConsume 在 runSkill 之前執行，SHOCK_STUN miss 仍應扣 MpConsume=15，got MP=%d want=85", caster.MP)
	}
}

// Java `C_UseSkill.start()` 的 `useConsume()` 對 NPC 目標同樣在 `runSkill()` 之前執行，
// 因此玩家施放 SHOCK_STUN 對 valid NPC 目標即使概率失敗，MP 仍會被扣除（與玩家目標 miss 相同）。
// 既有玩家目標版 TestSkillClanShockStunPlayerMissStillConsumesMpLikeJava 已覆蓋玩家對玩家路徑；
// 本測試補上 NPC 目標路徑的對等回歸（NPC 目標路徑不會扣 RegistStun，因此命中率夾不到 0，
// 但 MP 在 processSkill 內就消耗，與 hit/miss 結果無關，可直接驗證）。
func TestSkillClanShockStunPlayerMissAgainstNpcStillConsumesMpLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "low-level-caster",
		X:           100,
		Y:           100,
		MapID:       4,
		MP:          100,
		MaxMP:       100,
		Level:       1,
		Intel:       12,
		KnownSpells: []int32{87},
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "alive-npc",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 99,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  npc.ID,
	})

	// Java useConsume 在 runSkill 之前，hit/miss 都應扣 MP。
	if caster.MP != 85 {
		t.Fatalf("Java useConsume 在 runSkill 之前執行，SHOCK_STUN 對 valid NPC 目標即使 miss 也應扣 MpConsume=15，got MP=%d want=85", caster.MP)
	}
}

// Java C_UseSkill.start() 在 isSkillMastery 通過後、`pc.killSkillEffectTimer(MEDITATION)` 之前，
// 會呼叫 `cancelAbsoluteBarrier()` 解除施法者自己的 ABSOLUTE_BARRIER(78)。負面測試
// TestSkillClanShockStunUnknownSpellDoesNotCancelCasterAbsoluteBarrierLikeJava 已鎖定未學會時不解除，
// 本測試補上正面案例：已學會 87 並合法施放時，施法者自己的 78 應被解除。
func TestSkillClanShockStunLegalCastCancelsCasterAbsoluteBarrierLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:       1,
		Session:         newSkillTestSession(t, 1),
		CharID:          1001,
		Name:            "ab-caster",
		X:               100,
		Y:               100,
		MapID:           4,
		MP:              100,
		MaxMP:           100,
		AbsoluteBarrier: true,
		KnownSpells:     []int32{87},
	})
	caster.AddBuff(&world.ActiveBuff{SkillID: 78, TicksLeft: 20, SetAbsoluteBarrier: true})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
	})
	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  target.CharID,
	})

	if caster.AbsoluteBarrier || caster.HasBuff(78) {
		t.Fatalf("Java C_UseSkill 在合法施放 SHOCK_STUN 前會解除施法者自己的 ABSOLUTE_BARRIER(78)，AbsoluteBarrier=%v buff78=%v", caster.AbsoluteBarrier, caster.HasBuff(78))
	}
}

func TestSkillClanShockStunLegalCastCancelsMeditationLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "meditating-knight",
		X:           100,
		Y:           100,
		MapID:       4,
		MP:          100,
		MaxMP:       100,
		KnownSpells: []int32{87},
	})
	caster.AddBuff(&world.ActiveBuff{SkillID: 32, TicksLeft: 20, DeltaMPR: 5})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
	})
	s := newSkillBuffTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   87,
		TargetID:  target.CharID,
	})

	if caster.HasBuff(32) {
		t.Fatal("Java C_UseSkill 合法施放前會解除 MEDITATION(32)，SHOCK_STUN 施放時不應保留冥想術")
	}
}

func TestSkillClanShockStunNpcTargetEarthBindBlocksLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		Level:     80,
		X:         100,
		Y:         100,
		MapID:     4,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 100,
		Impl:  "L1Monster",
		Name:  "frozen",
		Level: 1,
		X:     101,
		Y:     100,
		MapID: 4,
	}
	npc.Paralyzed = true
	npc.AddDebuff(157, 100)
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 0}

	s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)

	if npc.HasDebuff(87) {
		t.Fatal("Java NPC 目標已有 EARTH_BIND 時 SHOCK_STUN 應在 calcProbabilityMagic 返回 false，不應套用 87")
	}
	if hasSkillEffectPacket(drainSkillTestPackets(caster.Session), npc.ID, 4434) {
		t.Fatal("Java NPC 目標已有 EARTH_BIND 時 SHOCK_STUN targetList 為空，不應送目標 4434")
	}
}

// Java `L1MagicPc.calcProbabilityMagic()` 對 NPC 目標已有 ICE_LANCE(50) 或 EARTH_BIND(157)
// 都會讓 SHOCK_STUN 判定失敗。既有 `TestSkillClanShockStunNpcTargetEarthBindBlocksLikeJava`
// 只覆蓋 debuff 157，本測試補上 NPC 目標 debuff 50 (ICE_LANCE) 的對等 negative case。
func TestSkillClanShockStunNpcTargetIceLanceBlocksLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		Level:     80,
		X:         100,
		Y:         100,
		MapID:     4,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 100,
		Impl:  "L1Monster",
		Name:  "ice-npc",
		Level: 1,
		X:     101,
		Y:     100,
		MapID: 4,
	}
	npc.AddDebuff(50, 100)
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 0}

	s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)

	if npc.HasDebuff(87) {
		t.Fatal("Java NPC 目標已有 ICE_LANCE(50) 時 SHOCK_STUN 應在 calcProbabilityMagic 返回 false，不應套用 87")
	}
	if hasSkillEffectPacket(drainSkillTestPackets(caster.Session), npc.ID, 4434) {
		t.Fatal("Java NPC 目標已有 ICE_LANCE(50) 時 SHOCK_STUN targetList 為空，不應送目標 4434")
	}
}

func TestSkillClanShockStunNpcCreatesJavaEffectNpc(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     60,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "stun target",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 50,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	for seed := int64(1); seed <= 100; seed++ {
		rand.Seed(seed)
		npc.Paralyzed = false
		npc.ActiveDebuffs = nil
		for _, effect := range ws.GroundEffectList() {
			ws.RemoveGroundEffect(effect.ID)
		}

		s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)
		if !npc.HasDebuff(87) {
			continue
		}

		effects := ws.GetNearbyGroundEffects(npc.X, npc.Y, npc.MapID)
		if len(effects) != 1 {
			t.Fatalf("Java SHOCK_STUN 對 NPC 成功時應 spawnEffect(81162) 到目標座標，got effects=%d seed=%d", len(effects), seed)
		}
		if effects[0].NpcID != 81162 || effects[0].GfxID != 4183 || effects[0].SkillID != 87 {
			t.Fatalf("SHOCK_STUN NPC 地面效果資料錯誤: %+v", effects[0])
		}
		if effects[0].TicksLeft < 5 || effects[0].TicksLeft > 25 {
			t.Fatalf("SHOCK_STUN NPC 81162 效果時間應等於 1~5 秒暈眩，ticks=%d", effects[0].TicksLeft)
		}
		return
	}
	t.Fatal("測試種子 1..100 未觸發 SHOCK_STUN NPC 成功，無法驗證 81162 效果")
}

func TestSkillClanShockStunNpcSuccessUsesJavaFixedTargetGfx(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     60,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "stun target",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 50,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 0}

	for seed := int64(1); seed <= 100; seed++ {
		rand.Seed(seed)
		npc.Paralyzed = false
		npc.ActiveDebuffs = nil
		_ = drainSkillTestPackets(caster.Session)

		s.executeNpcDebuffSkill(caster.Session, caster, skill, npc)
		if !npc.HasDebuff(87) {
			continue
		}
		if !hasSkillEffectPacket(drainSkillTestPackets(caster.Session), npc.ID, 4434) {
			t.Fatal("Java SHOCK_STUN 對 NPC 成功時固定送目標 S_SkillSound(4434)，不依技能資料 CastGfx")
		}
		return
	}
	t.Fatal("測試種子 1..100 未觸發 SHOCK_STUN NPC 成功，無法驗證固定 4434")
}

func TestSkillClanShockStunNpcCasterUsesJavaDurationAndEffectNpc(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -30,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能資料失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 0)

	buff := target.GetBuff(87)
	if buff == nil || !target.Paralyzed {
		t.Fatalf("Java NPC 施放 SHOCK_STUN 應套玩家暈眩，buff=%v Paralyzed=%v", buff, target.Paralyzed)
	}
	if buff.TicksLeft < 5 || buff.TicksLeft > 25 {
		t.Fatalf("Java NPC 施放 SHOCK_STUN 使用 1~5 秒，不應使用技能表 6 秒，ticks=%d", buff.TicksLeft)
	}
	effects := ws.GetNearbyGroundEffects(target.X, target.Y, target.MapID)
	if len(effects) != 1 {
		t.Fatalf("Java NPC 施放 SHOCK_STUN 應在目標座標 spawnEffect(81162)，got effects=%d", len(effects))
	}
	if effects[0].NpcID != 81162 || effects[0].SkillID != 87 || effects[0].OwnerCharID != npc.ID {
		t.Fatalf("NPC SHOCK_STUN 地面效果資料錯誤: %+v", effects[0])
	}
}

func TestSkillClanShockStunNpcCasterBroadcastsJavaActionGfx(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -30,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能資料失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 0)

	if !hasActionGfxPacket(drainSkillTestPackets(target.Session), npc.ID, 19) {
		t.Fatal("Java NPC 施放 SHOCK_STUN 應廣播施法者 S_DoActionGFX")
	}
}

func TestSkillClanShockStunNpcCasterExistingBuffSendsJavaPacketsOnly(t *testing.T) {
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		Level:     1,
		X:         101,
		Y:         100,
		MapID:     4,
		Paralyzed: true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 87, TicksLeft: 999, SetParalyzed: true})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能資料失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 0)

	if got := target.GetBuff(87).TicksLeft; got != 999 {
		t.Fatalf("Java NPC 施放 SHOCK_STUN 到已有 87 目標不應刷新時間，ticks=%d", got)
	}
	if effects := ws.GetNearbyGroundEffects(target.X, target.Y, target.MapID); len(effects) != 0 {
		t.Fatalf("Java NPC 施放 SHOCK_STUN 到已有 87 目標不應再次 spawnEffect(81162)，got=%d", len(effects))
	}
	packets := drainSkillTestPackets(target.Session)
	if !hasActionGfxPacket(packets, npc.ID, 19) {
		t.Fatal("Java NPC 施放 SHOCK_STUN 到已有 87 目標仍應廣播施法者 S_DoActionGFX")
	}
	if countSkillEffectPackets(packets, target.CharID, 4434) == 0 {
		t.Fatal("Java NPC 施放 SHOCK_STUN 到已有 87 目標仍應送目標 S_SkillSound(4434)")
	}
}

func TestSkillClanShockStunNpcCasterExistingBuffStillUsesJavaProbability(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		Paralyzed:  true,
		RegistStun: 200,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 87, TicksLeft: 999, SetParalyzed: true})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能資料失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 0)

	if got := target.GetBuff(87).TicksLeft; got != 999 {
		t.Fatalf("Java NPC 施放 SHOCK_STUN 到已有 87 目標即使命中判定失敗也不應刷新時間，ticks=%d", got)
	}
	packets := drainSkillTestPackets(target.Session)
	if !hasActionGfxPacket(packets, npc.ID, 19) {
		t.Fatal("Java NPC 施放 SHOCK_STUN 命中判定失敗時仍只廣播施法者 S_DoActionGFX")
	}
	if countSkillEffectPackets(packets, target.CharID, 4434) != 0 {
		t.Fatal("Java NPC 施放 SHOCK_STUN 命中判定失敗時 targetList 為空，不應送目標 S_SkillSound(4434)")
	}
}

func TestSkillClanShockStunNpcCasterUsesJavaStunResistProbability(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "resist-target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: 127,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 99,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	s.ApplyNpcShockStun(npc, target, &data.SkillInfo{SkillID: 87, BuffDuration: 6}, 0)

	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("Java L1MagicNpc 對 SHOCK_STUN 會以 70/50/30 成功率再扣暈眩抗性，高暈抗目標不應被套用，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	if effects := ws.GetNearbyGroundEffects(target.X, target.Y, target.MapID); len(effects) != 0 {
		t.Fatalf("NPC SHOCK_STUN 抵抗時不應產生 81162 效果，got=%d", len(effects))
	}
}

func TestSkillClanShockStunNpcCasterProbabilityCapsAtJavaNinety(t *testing.T) {
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 99,
	}
	target := &world.PlayerInfo{
		CharID:     1001,
		Name:       "target",
		Level:      1,
		RegistStun: -100,
	}

	if got := shockStunNpcCasterProbability(npc, target, 10); got != 90 {
		t.Fatalf("Java L1MagicNpc.calcProbabilityMagic 會將 SHOCK_STUN 成功率封頂為 90，got=%d", got)
	}
}

func TestSkillClanShockStunNpcCasterTargetEarthBindBlocksLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "earth-bind-target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		Paralyzed:  true,
		RegistStun: -100,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 157, TicksLeft: 100, SetParalyzed: true})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 99,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	s.ApplyNpcShockStun(npc, target, &data.SkillInfo{SkillID: 87, BuffDuration: 6, ActionID: 19, CastGfx: 4434}, 0)

	if target.HasBuff(87) {
		t.Fatal("Java L1MagicNpc 目標已有 EARTH_BIND 時 SHOCK_STUN 應在 calcProbabilityMagic 返回 false，不應套用 87")
	}
	packets := drainSkillTestPackets(target.Session)
	if !hasActionGfxPacket(packets, npc.ID, 19) {
		t.Fatal("Java NPC 施放 SHOCK_STUN 判定失敗時仍應廣播施法者 S_DoActionGFX")
	}
	if countSkillEffectPackets(packets, target.CharID, 4434) != 0 {
		t.Fatal("Java NPC 施放 SHOCK_STUN 對 EARTH_BIND 目標 targetList 為空，不應送目標 4434")
	}
}

// Java `L1MagicNpc.calcProbabilityMagic()` 與 `L1MagicPc.calcProbabilityMagic()`
// 對 NPC 施放 SHOCK_STUN 同樣以 `hasSkillEffect(50) || hasSkillEffect(157)` 判定
// 自動失敗。既有 TestSkillClanShockStunNpcCasterTargetEarthBindBlocksLikeJava
// 只覆蓋玩家目標 buff 157（EARTH_BIND），本步補上 buff 50（ICE_LANCE）的對等
// negative case，鎖定 Go `ApplyNpcShockStun` 對 `target.HasBuff(50) || target.HasBuff(157)`
// 的 `||` 雙條件邏輯雙向覆蓋。
func TestSkillClanShockStunNpcCasterTargetIceLanceBlocksLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "ice-lance-target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		Paralyzed:  true,
		RegistStun: -100,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 50, TicksLeft: 100, SetParalyzed: true})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 99,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	s.ApplyNpcShockStun(npc, target, &data.SkillInfo{SkillID: 87, BuffDuration: 6, ActionID: 19, CastGfx: 4434}, 0)

	if target.HasBuff(87) {
		t.Fatal("Java L1MagicNpc 目標已有 ICE_LANCE 時 SHOCK_STUN 應在 calcProbabilityMagic 返回 false，不應套用 87")
	}
	packets := drainSkillTestPackets(target.Session)
	if !hasActionGfxPacket(packets, npc.ID, 19) {
		t.Fatal("Java NPC 施放 SHOCK_STUN 判定失敗時仍應廣播施法者 S_DoActionGFX")
	}
	if countSkillEffectPackets(packets, target.CharID, 4434) != 0 {
		t.Fatal("Java NPC 施放 SHOCK_STUN 對 ICE_LANCE 目標 targetList 為空，不應送目標 4434")
	}
}

func TestSkillClanShockStunNpcCasterLeverageAffectsJavaProbability(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -30,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能資料失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 1)

	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("Java NPC SHOCK_STUN 會套用 mob_skill leverage 到命中率，低 leverage 時此 seed 應 miss，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	if effects := ws.GetNearbyGroundEffects(target.X, target.Y, target.MapID); len(effects) != 0 {
		t.Fatalf("Java NPC SHOCK_STUN leverage miss 不應生成 81162，got=%d", len(effects))
	}
	packets := drainSkillTestPackets(target.Session)
	if !hasActionGfxPacket(packets, npc.ID, 19) {
		t.Fatal("Java NPC SHOCK_STUN leverage miss 仍應只廣播施法者 S_DoActionGFX")
	}
	if countSkillEffectPackets(packets, target.CharID, 4434) != 0 {
		t.Fatal("Java NPC SHOCK_STUN leverage miss 不應送目標 S_SkillSound(4434)")
	}
}

// 對應 Java `L1SkillUse.runSkill()` 第 1965-1968 行的 NPC caster 路徑，
// `ApplyNpcShockStun` 走相同 `clearShockStunSleepEffects`，本步補上 PHANTASM(212) 玩家目標清除回歸，
// 與既有 `TestSkillClanShockStunNpcCasterMissClearsSleepLikeJava`（covers 66/103）形成完整覆蓋。
func TestSkillClanShockStunNpcCasterClearsPhantasmLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "phantasm-target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		Sleeped:    true,
		RegistStun: 200,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 212, TicksLeft: 100, SetSleeped: true})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	s.ApplyNpcShockStun(npc, target, &data.SkillInfo{SkillID: 87, BuffDuration: 6, ActionID: 19, CastGfx: 4434}, 0)

	if target.HasBuff(212) {
		t.Fatal("Java NPC 施放 SHOCK_STUN 在 runSkill 之前也會 removeSkillEffect(212)，PHANTASM 應被清除")
	}
}

func TestSkillClanShockStunNpcCasterMissClearsSleepLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "sleep-target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		Sleeped:    true,
		RegistStun: 200,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 66, TicksLeft: 100, SetSleeped: true})
	target.AddBuff(&world.ActiveBuff{SkillID: 103, TicksLeft: 100, SetSleeped: true})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能表失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 0)

	if target.Sleeped || target.HasBuff(66) || target.HasBuff(103) {
		t.Fatalf("Java L1SkillUse.runSkill() 會在概率判定前移除睡眠效果，Sleeped=%v buff66=%v buff103=%v", target.Sleeped, target.HasBuff(66), target.HasBuff(103))
	}
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("SHOCK_STUN miss 不應套用 87，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	packets := drainSkillTestPackets(target.Session)
	if !hasParalysisSubtype(packets, handler.SleepRemove) {
		t.Fatalf("解除睡眠時應送 S_Paralysis SleepRemove，packets=%v", packets)
	}
	if !hasActionGfxPacket(packets, npc.ID, 19) {
		t.Fatal("SHOCK_STUN miss 仍應廣播施法者 S_DoActionGFX")
	}
	if countSkillEffectPackets(packets, target.CharID, 4434) != 0 {
		t.Fatal("SHOCK_STUN miss 不應送目標 S_SkillSound(4434)")
	}
}

func TestSkillClanShockStunNpcCasterMissClearsEraseMagicLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "erase-target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: 200,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 153, TicksLeft: 100})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	s.ApplyNpcShockStun(npc, target, &data.SkillInfo{SkillID: 87, BuffDuration: 6, ActionID: 19, CastGfx: 4434}, 0)

	if target.HasBuff(153) {
		t.Fatal("Java L1SkillUse.runSkill() 會在 SHOCK_STUN NPC 施放概率判定後、命中結果前移除目標 ERASE_MAGIC")
	}
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("SHOCK_STUN miss 不應套用 87，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
}

func TestSkillClanShockStunNpcCasterSkipsGmInvisibleTargetLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "gm-invis-target",
		Level:       1,
		X:           103,
		Y:           100,
		MapID:       4,
		AccessLevel: 200,
		GMInvisible: true,
		Invisible:   true,
		Sleeped:     true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 66, TicksLeft: 100, SetSleeped: true})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能表失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 0)

	if !target.Sleeped || !target.HasBuff(66) {
		t.Fatalf("Java checkTarget 會先排除 GM 隱身目標，不應進入 runSkill 清除睡眠，Sleeped=%v buff66=%v", target.Sleeped, target.HasBuff(66))
	}
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("GM 隱身目標不應被 NPC SHOCK_STUN 套用 87，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	packets := drainSkillTestPackets(target.Session)
	if hasActionGfxPacket(packets, npc.ID, 19) {
		t.Fatal("Java checkUseSkill=false 時不會送 NPC SHOCK_STUN 施法動作")
	}
	if hasParalysisSubtype(packets, handler.SleepRemove) {
		t.Fatal("GM 隱身目標被排除時不應送睡眠解除")
	}
	if countSkillEffectPackets(packets, target.CharID, 4434) != 0 {
		t.Fatal("GM 隱身目標被排除時不應送目標 S_SkillSound(4434)")
	}
}

func TestSkillClanShockStunNpcCasterGmInvisibleTargetDoesNotConsumeMpLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "gm-invis-target",
		Level:       1,
		X:           103,
		Y:           100,
		MapID:       4,
		AccessLevel: 200,
		GMInvisible: true,
		Invisible:   true,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能表失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 0)

	if npc.MP != 100 {
		t.Fatalf("Java isTarget 會在 useConsume 前排除 GM 隱身目標，不應消耗 NPC MP，MP=%d", npc.MP)
	}
}

func TestSkillClanShockStunNpcCasterSkipsAbsoluteBarrierTargetLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:       1,
		Session:         newSkillTestSession(t, 1),
		CharID:          1001,
		Name:            "barrier-target",
		Level:           1,
		X:               103,
		Y:               100,
		MapID:           4,
		AbsoluteBarrier: true,
		Sleeped:         true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 78, TicksLeft: 100, SetAbsoluteBarrier: true})
	target.AddBuff(&world.ActiveBuff{SkillID: 66, TicksLeft: 100, SetSleeped: true})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	s.ApplyNpcShockStun(npc, target, &data.SkillInfo{SkillID: 87, BuffDuration: 6, ActionID: 19, CastGfx: 4434}, 0)

	if !target.AbsoluteBarrier || !target.HasBuff(78) {
		t.Fatalf("Java checkTarget 會排除 ABSOLUTE_BARRIER 目標，不應移除目標絕對屏障，AbsoluteBarrier=%v buff78=%v", target.AbsoluteBarrier, target.HasBuff(78))
	}
	if !target.Sleeped || !target.HasBuff(66) {
		t.Fatalf("Java checkUseSkill=false 時不會進入 runSkill，不應清除睡眠，Sleeped=%v buff66=%v", target.Sleeped, target.HasBuff(66))
	}
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("ABSOLUTE_BARRIER 目標不應被 NPC SHOCK_STUN 套用，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	packets := drainSkillTestPackets(target.Session)
	if hasActionGfxPacket(packets, npc.ID, 19) {
		t.Fatal("Java checkUseSkill=false 時不應送 NPC SHOCK_STUN 施法動作")
	}
	if hasParalysisSubtype(packets, handler.SleepRemove) {
		t.Fatal("ABSOLUTE_BARRIER 目標被排除時不應送睡眠解除封包")
	}
	if countSkillEffectPackets(packets, target.CharID, 4434) != 0 {
		t.Fatal("ABSOLUTE_BARRIER 目標被排除時不應送目標 S_SkillSound(4434)")
	}
}

func TestSkillClanShockStunNpcCasterAbsoluteBarrierTargetKeepsAggroLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:       1,
		Session:         newSkillTestSession(t, 1),
		CharID:          1001,
		Name:            "barrier-target",
		Level:           1,
		X:               101,
		Y:               100,
		MapID:           4,
		AbsoluteBarrier: true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 78, TicksLeft: 100, SetAbsoluteBarrier: true})
	npc := &world.NpcInfo{
		ID:          2001,
		NpcID:       45000,
		Impl:        "L1Monster",
		Name:        "mob",
		Level:       50,
		X:           100,
		Y:           100,
		MapID:       4,
		MP:          100,
		MaxMP:       100,
		AggroTarget: target.SessionID,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能表失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 0)

	if npc.AggroTarget != target.SessionID {
		t.Fatalf("Java L1MobSkillUse.checkUseSkill=false 只讓技能失敗，不應清掉怪物仇恨目標，AggroTarget=%d", npc.AggroTarget)
	}
	if npc.MP != 100 {
		t.Fatalf("ABSOLUTE_BARRIER 目標在 useConsume 前被排除，不應消耗 NPC MP，MP=%d", npc.MP)
	}
}

func TestSkillClanShockStunNpcCasterSkipsDeadTargetLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "dead-target",
		Level:     1,
		X:         103,
		Y:         100,
		MapID:     4,
		Dead:      true,
		Sleeped:   true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 66, TicksLeft: 100, SetSleeped: true})
	target.AddBuff(&world.ActiveBuff{SkillID: 153, TicksLeft: 100})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	s.ApplyNpcShockStun(npc, target, &data.SkillInfo{SkillID: 87, BuffDuration: 6, ActionID: 19, CastGfx: 4434}, 0)

	if !target.Dead {
		t.Fatal("測試前提錯誤：目標應維持死亡狀態")
	}
	if !target.Sleeped || !target.HasBuff(66) {
		t.Fatalf("Java checkTarget 會先排除死亡目標，不應進入 runSkill 清除睡眠，Sleeped=%v buff66=%v", target.Sleeped, target.HasBuff(66))
	}
	if !target.HasBuff(153) {
		t.Fatal("死亡目標被排除時不應清除 ERASE_MAGIC")
	}
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("死亡目標不應被 NPC SHOCK_STUN 套用，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	packets := drainSkillTestPackets(target.Session)
	if hasActionGfxPacket(packets, npc.ID, 19) {
		t.Fatal("Java checkUseSkill=false 時不應送 NPC SHOCK_STUN 施法動作")
	}
	if hasParalysisSubtype(packets, handler.SleepRemove) {
		t.Fatal("死亡目標被排除時不應送睡眠解除封包")
	}
	if countSkillEffectPackets(packets, target.CharID, 4434) != 0 {
		t.Fatal("死亡目標被排除時不應送目標 S_SkillSound(4434)")
	}
}

func TestSkillClanShockStunNpcCasterSkipsDifferentMapTargetLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "other-map-target",
		Level:     1,
		X:         103,
		Y:         100,
		MapID:     5,
		Sleeped:   true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 66, TicksLeft: 100, SetSleeped: true})
	target.AddBuff(&world.ActiveBuff{SkillID: 153, TicksLeft: 100})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	s.ApplyNpcShockStun(npc, target, &data.SkillInfo{SkillID: 87, BuffDuration: 6, ActionID: 19, CastGfx: 4434}, 0)

	if !target.Sleeped || !target.HasBuff(66) {
		t.Fatalf("Java 怪物單體技能目標來自同地圖可見玩家，不應進入 runSkill 清除跨地圖目標睡眠，Sleeped=%v buff66=%v", target.Sleeped, target.HasBuff(66))
	}
	if !target.HasBuff(153) {
		t.Fatal("跨地圖目標被排除時不應清除 ERASE_MAGIC")
	}
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("跨地圖目標不應被 NPC SHOCK_STUN 套用，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
}

func TestSkillClanShockStunNpcCasterSkipsOutOfRangeTargetLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "far-target",
		Level:     1,
		X:         102,
		Y:         100,
		MapID:     4,
		Sleeped:   true,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 66, TicksLeft: 100, SetSleeped: true})
	target.AddBuff(&world.ActiveBuff{SkillID: 153, TicksLeft: 100})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	s.ApplyNpcShockStun(npc, target, &data.SkillInfo{SkillID: 87, BuffDuration: 6, ActionID: 19, CastGfx: 4434, Ranged: 1}, 0)

	if !target.Sleeped || !target.HasBuff(66) {
		t.Fatalf("Java makeTargetList 會以 ranged=1 擋下射程外 NPC SHOCK_STUN，不應進入 runSkill 清除睡眠，Sleeped=%v buff66=%v", target.Sleeped, target.HasBuff(66))
	}
	if !target.HasBuff(153) {
		t.Fatal("射程外目標被排除時不應清除 ERASE_MAGIC")
	}
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("射程外目標不應被 NPC SHOCK_STUN 套用，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
}

func TestSkillClanShockStunNpcCasterDifferentMapTargetDoesNotConsumeMpLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "other-map-target",
		Level:     1,
		X:         103,
		Y:         100,
		MapID:     5,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能表失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 0)

	if npc.MP != 100 {
		t.Fatalf("Java 怪物單體技能目標來自同地圖可見玩家，跨地圖目標不應進入 useConsume 消耗 NPC MP，MP=%d", npc.MP)
	}
}

func TestSkillClanShockStunNpcCasterDifferentShowIDTargetDoesNotConsumeMpLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "other-show-target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		ShowID:     200,
		RegistStun: -100,
	})
	npc := &world.NpcInfo{
		ID:     2001,
		NpcID:  45000,
		Impl:   "L1Monster",
		Name:   "mob",
		Level:  50,
		X:      100,
		Y:      100,
		MapID:  4,
		ShowID: 100,
		MP:     100,
		MaxMP:  100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能表失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 0)

	if npc.MP != 100 {
		t.Fatalf("Java isTarget 會以 showId 不同擋下 NPC SHOCK_STUN，不應消耗 NPC MP，MP=%d", npc.MP)
	}
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("NPC SHOCK_STUN 不應套用到不同 showId 玩家，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
}

func TestSkillClanShockStunNpcCasterOutOfRangeTargetDoesNotConsumeMpLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "far-target",
		Level:     1,
		X:         102,
		Y:         100,
		MapID:     4,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能表失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 0)

	if npc.MP != 100 {
		t.Fatalf("Java makeTargetList 會在 useConsume 前以 ranged=1 排除射程外目標，不應消耗 NPC MP，MP=%d", npc.MP)
	}
}

func TestSkillClanShockStunNpcCasterDeadTargetDoesNotConsumeMpLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "dead-target",
		Level:     1,
		X:         103,
		Y:         100,
		MapID:     4,
		Dead:      true,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
		MP:    100,
		MaxMP: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能表失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 0)

	if npc.MP != 100 {
		t.Fatalf("Java checkUseSkill 會在 useConsume 前排除死亡目標，不應消耗 NPC MP，MP=%d", npc.MP)
	}
}

func TestSkillClanShockStunPlayerCasterSkipsGmInvisibleTargetLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		Level:     80,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   2,
		Session:     newSkillTestSession(t, 2),
		CharID:      1002,
		Name:        "gm-invis-target",
		X:           101,
		Y:           100,
		MapID:       4,
		Level:       1,
		AccessLevel: 200,
		GMInvisible: true,
		Invisible:   true,
		Sleeped:     true,
		RegistStun:  -100,
	})
	target.AddBuff(&world.ActiveBuff{SkillID: 66, TicksLeft: 100, SetSleeped: true})
	target.AddBuff(&world.ActiveBuff{SkillID: 153, TicksLeft: 100})
	s := newSkillTestSystem(t, ws)
	combat := &shockStunCombatSpy{}
	s.deps.Combat = combat
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("Java L1SkillUse.checkTarget() 會排除 GM 隱身玩家，SHOCK_STUN 不應套用，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	if len(combat.requests) != 0 {
		t.Fatalf("Java checkUseSkill=false 時不會進入 runSkill，不應觸發 SHOCK_STUN onAction，requests=%d", len(combat.requests))
	}
	if !target.Sleeped || !target.HasBuff(66) || !target.HasBuff(153) {
		t.Fatalf("Java checkUseSkill=false 時不會進入 runSkill，不應清除 GM 隱身目標既有狀態，Sleeped=%v buff66=%v buff153=%v", target.Sleeped, target.HasBuff(66), target.HasBuff(153))
	}
	if countSkillEffectPackets(drainSkillTestPackets(target.Session), target.CharID, 4434) != 0 {
		t.Fatal("Java checkUseSkill=false 時不應送 GM 隱身目標 4434")
	}
}

func TestSkillClanShockStunNpcCasterMissSendsOnlyJavaActionGfx(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "resist-target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: 127,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 99,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能資料失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 0, 0)

	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("Java NPC 施放 SHOCK_STUN miss 時不應套用 87，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	if effects := ws.GetNearbyGroundEffects(target.X, target.Y, target.MapID); len(effects) != 0 {
		t.Fatalf("Java NPC 施放 SHOCK_STUN miss 時不應生成 81162，got=%d", len(effects))
	}
	packets := drainSkillTestPackets(target.Session)
	if !hasActionGfxPacket(packets, npc.ID, 19) {
		t.Fatal("Java NPC 施放 SHOCK_STUN miss 時仍應廣播施法者 S_DoActionGFX")
	}
	if countSkillEffectPackets(packets, target.CharID, 4434) != 0 {
		t.Fatal("Java NPC 施放 SHOCK_STUN miss 時 targetList 為空，不應送目標 S_SkillSound(4434)")
	}
}

func TestSkillClanShockStunNpcCasterUsesMobSkillActIDOverride(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -30,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能資料失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 7, 0, 0)

	packets := drainSkillTestPackets(target.Session)
	if !hasActionGfxPacket(packets, npc.ID, 7) {
		t.Fatal("Java NPC 施放 SHOCK_STUN 應優先使用 mob_skill act_id 覆寫施法者動作")
	}
	if hasActionGfxPacket(packets, npc.ID, 19) {
		t.Fatal("mob_skill act_id 有值時不應回退使用技能表 action_id=19")
	}
}

func TestSkillClanShockStunNpcCasterUsesMobSkillGfxIDOverride(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -30,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能表失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 1234, 0)

	packets := drainSkillTestPackets(target.Session)
	if countSkillEffectPackets(packets, target.CharID, 1234) == 0 {
		t.Fatal("Java NPC 施放 SHOCK_STUN 應優先使用 mob_skill gfx_id 作為目標 S_SkillSound")
	}
	if countSkillEffectPackets(packets, target.CharID, 4434) != 0 {
		t.Fatal("mob_skill gfx_id 非 0 時不應回退技能表 cast_gfx=4434")
	}
}

func TestSkillClanShockStunNpcCasterBroadcastsFromCasterLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:  1,
		Session:    newSkillTestSession(t, 1),
		CharID:     1001,
		Name:       "target",
		Level:      1,
		X:          101,
		Y:          100,
		MapID:      4,
		RegistStun: -30,
	})
	observerNearCaster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "observer-near-caster",
		X:         80,
		Y:         100,
		MapID:     4,
	})
	observerNearTarget := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    1003,
		Name:      "observer-near-target",
		X:         121,
		Y:         100,
		MapID:     4,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("讀取技能表失敗: %v", err)
	}
	s.deps.Skills = skills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcSkill(npc, target, 87, 19, 4434, 0)

	nearCasterPackets := drainSkillTestPackets(observerNearCaster.Session)
	if !hasActionGfxPacket(nearCasterPackets, npc.ID, 19) {
		t.Fatal("Java NPC 施放 SHOCK_STUN 應從施法 NPC 廣播 S_DoActionGFX")
	}
	if countSkillEffectPackets(nearCasterPackets, target.CharID, 4434) == 0 {
		t.Fatal("Java NPC 施放 SHOCK_STUN 的目標 S_SkillSound 也從施法 NPC 廣播")
	}

	nearTargetPackets := drainSkillTestPackets(observerNearTarget.Session)
	if hasActionGfxPacket(nearTargetPackets, npc.ID, 19) || countSkillEffectPackets(nearTargetPackets, target.CharID, 4434) != 0 {
		t.Fatal("不在施法 NPC 可見範圍內的玩家不應收到 NPC SHOCK_STUN 廣播封包")
	}
}

func TestSkillClanShockStunMobAreaShockStunMatchesJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
	})
	existing := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "existing",
		X:         102,
		Y:         100,
		MapID:     4,
		Paralyzed: true,
	})
	existing.AddBuff(&world.ActiveBuff{SkillID: 87, TicksLeft: 999, SetParalyzed: true})
	gmInvis := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   3,
		Session:     newSkillTestSession(t, 3),
		CharID:      1003,
		Name:        "gm-invis",
		X:           103,
		Y:           100,
		MapID:       4,
		AccessLevel: 200,
		GMInvisible: true,
		Invisible:   true,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 231008,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	s.deps.Skill = s
	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcAreaShockStun(npc, 0)

	buff := target.GetBuff(87)
	if buff == nil || !target.Paralyzed {
		t.Fatalf("Java type 5 areashock_stun 應套玩家暈眩，buff=%v Paralyzed=%v", buff, target.Paralyzed)
	}
	if buff.TicksLeft < 10 || buff.TicksLeft > 25 {
		t.Fatalf("Java type 5 areashock_stun 使用 2~5 秒，ticks=%d", buff.TicksLeft)
	}
	if got := existing.GetBuff(87).TicksLeft; got != 999 {
		t.Fatalf("Java type 5 areashock_stun 不重套已有 SHOCK_STUN 的目標，ticks=%d", got)
	}
	if gmInvis.HasBuff(87) {
		t.Fatal("Java type 5 areashock_stun 會略過 GM 隱身目標")
	}
	effects := ws.GetNearbyGroundEffects(target.X, target.Y, target.MapID)
	if len(effects) != 1 {
		t.Fatalf("Java type 5 areashock_stun 應只在新命中的玩家座標 spawnEffect(81162)，got effects=%d", len(effects))
	}
	if effects[0].NpcID != 81162 || effects[0].SkillID != 87 || effects[0].OwnerCharID != npc.ID {
		t.Fatalf("type 5 areashock_stun 地面效果資料錯誤: %+v", effects[0])
	}
	if !hasActionGfxPacket(drainSkillTestPackets(target.Session), npc.ID, 1) {
		t.Fatal("Java type 5 areashock_stun act_id=0 時應廣播 S_DoActionGFX action=1")
	}
}

// Java L1MobSkillUse.areashock_stun() 第 734-738 行：
//
//	int actId = getMobSkillTemplate().getActid(idx);
//	int actionid = 1;
//	if (actId > 0) { actionid = actId; }
//
// 即 mob_skill_template.act_id > 0 時覆寫預設值 1，並由 _attacker.broadcastPacketAll(S_DoActionGFX) 廣播。
// 既有 TestSkillClanShockStunMobAreaShockStunMatchesJava 已驗證 act_id=0 預設 1；
// 本測試補上 act_id > 0 覆寫路徑的回歸（單體 NPC 路徑已有對應 TestSkillClanShockStunNpcCasterUsesMobSkillActIDOverride）。
func TestSkillClanShockStunMobAreaShockStunUsesActIDOverrideLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 231008,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	s.deps.Skill = s
	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcAreaShockStun(npc, 7)

	if !target.HasBuff(87) {
		t.Fatal("Java areashock_stun act_id 覆寫不影響 buff 套用，預期套用 87")
	}
	packets := drainSkillTestPackets(target.Session)
	if hasActionGfxPacket(packets, npc.ID, 1) {
		t.Fatal("Java areashock_stun act_id=7 應覆寫預設 1，不應再廣播 S_DoActionGFX action=1")
	}
	if !hasActionGfxPacket(packets, npc.ID, 7) {
		t.Fatal("Java areashock_stun act_id=7 應廣播 S_DoActionGFX action=7")
	}
}

// Java L1MobSkillUse.areashock_stun() 第 740 行使用 `World.get().getVisiblePlayer(_attacker)`
// 來源，只會包含與 _attacker 同地圖、在可見範圍內的玩家。Go `executeNpcAreaShockStun`
// 透過 `world.GetNearbyPlayersAt(npc.X, npc.Y, npc.MapID)` 已僅限同地圖，本測試補上鎖定，
// 確保未來改動不會誤把不同地圖玩家加入 type 5 命中清單。
func TestSkillClanShockStunMobAreaShockStunSkipsDifferentMapPlayerLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	sameMap := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "same-map",
		X:         101,
		Y:         100,
		MapID:     4,
	})
	otherMap := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "other-map",
		X:         101,
		Y:         100,
		MapID:     5,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 231008,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	s.deps.Skill = s
	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcAreaShockStun(npc, 0)

	if !sameMap.HasBuff(87) || !sameMap.Paralyzed {
		t.Fatalf("Java areashock_stun 同地圖可見玩家應被套 87，buff=%v Paralyzed=%v", sameMap.GetBuff(87), sameMap.Paralyzed)
	}
	if otherMap.HasBuff(87) || otherMap.Paralyzed {
		t.Fatalf("Java getVisiblePlayer(_attacker) 只取同地圖玩家，map=5 玩家不應被套 87，buff=%v Paralyzed=%v", otherMap.GetBuff(87), otherMap.Paralyzed)
	}
	if effects := ws.GetNearbyGroundEffects(otherMap.X, otherMap.Y, otherMap.MapID); len(effects) != 0 {
		t.Fatalf("不同地圖玩家不應觸發 spawnEffect(81162)，got effects=%d", len(effects))
	}
}

func TestSkillClanShockStunMobAreaShockStunSkipsDifferentShowIDPlayerLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	sameShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "same-show",
		X:         101,
		Y:         100,
		MapID:     4,
		ShowID:    100,
		Known:     world.NewKnownEntities(),
	})
	otherShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "other-show",
		X:         101,
		Y:         100,
		MapID:     4,
		ShowID:    200,
		Known:     world.NewKnownEntities(),
	})
	npc := &world.NpcInfo{
		ID:     2001,
		NpcID:  231008,
		Impl:   "L1Monster",
		Name:   "mob",
		Level:  50,
		X:      100,
		Y:      100,
		MapID:  4,
		ShowID: 100,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	s.deps.Skill = s
	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcAreaShockStun(npc, 0)

	if !sameShow.HasBuff(87) || !sameShow.Paralyzed {
		t.Fatalf("Java getVisiblePlayer 同 showId 玩家應被套 87，buff=%v Paralyzed=%v", sameShow.GetBuff(87), sameShow.Paralyzed)
	}
	if otherShow.HasBuff(87) || otherShow.Paralyzed {
		t.Fatalf("Java isTarget 會以 showId 不同擋下 area SHOCK_STUN，buff=%v Paralyzed=%v", otherShow.GetBuff(87), otherShow.Paralyzed)
	}
	if effects := ws.GetNearbyGroundEffects(otherShow.X, otherShow.Y, otherShow.MapID); len(effects) != 1 {
		t.Fatalf("不同 showId 玩家位置只應看見同 showId 目標的 81162，不應為 otherShow 另建效果，got effects=%d", len(effects))
	}
	if len(otherShow.Known.GroundEffects) != 0 {
		t.Fatalf("不同 showId 玩家不應收到 NPC spawnEffect(81162) 可見封包，got known=%d", len(otherShow.Known.GroundEffects))
	}
	if hasActionGfxPacket(drainSkillTestPackets(otherShow.Session), npc.ID, 1) {
		t.Fatal("Java getVisiblePlayer 不會把 area SHOCK_STUN 動作廣播給不同 showId 玩家")
	}
}

func TestSkillClanShockStunMobAreaShockStunIncludesDeadVisiblePlayerLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "dead-target",
		X:         101,
		Y:         100,
		MapID:     4,
		Dead:      true,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 231008,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	s.deps.Skill = s
	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcAreaShockStun(npc, 0)

	if !target.HasBuff(87) || !target.Paralyzed {
		t.Fatalf("Java areashock_stun 只排除 GM 隱身與已有 87，死亡可見玩家仍會被套用 87，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
}

// Java `L1MobSkillUse.areashock_stun()` 第 748 行
// `L1SpawnUtil.spawnEffect(81162, shock, pc.getX(), pc.getY(), _attacker.getMapId(), _attacker, 0)`
// 第三、四個參數明確使用 **target 玩家** 的座標（pc.getX/pc.getY）而不是
// attacker 的座標。既有 `TestSkillClanShockStunMobAreaShockStunMatchesJava` 只用
// `GetNearbyGroundEffects(target.X, target.Y, ...)` 透過 AOI 查找，若 effect 被
// 誤 spawn 在 caster 座標仍可能落在 AOI 內未被檢出。本測試把 caster 和 target
// 放在不同座標（target 距 caster 4 格），並嚴格驗證 `effects[0].X == target.X &&
// effects[0].Y == target.Y`，鎖定 Go AOE 不會把 81162 spawn 在 caster 座標。
func TestSkillClanShockStunMobAreaShockStunEffectSpawnsAtTargetCoordsLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "far-target",
		X:         104,
		Y:         100,
		MapID:     4,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		NpcID: 231008,
		Impl:  "L1Monster",
		Name:  "mob",
		Level: 50,
		X:     100,
		Y:     100,
		MapID: 4,
	}
	ws.AddNpc(npc)

	s := newSkillTestSystem(t, ws)
	attachShockStunNpcTable(t, s)
	s.deps.Skill = s
	ai := NewNpcAISystem(ws, s.deps)
	ai.executeNpcAreaShockStun(npc, 0)

	if !target.HasBuff(87) {
		t.Fatal("測試前提：AOE SHOCK_STUN 應套用 87")
	}
	effects := ws.GetNearbyGroundEffects(target.X, target.Y, target.MapID)
	if len(effects) != 1 {
		t.Fatalf("應只 spawnEffect 1 筆 81162，got=%d", len(effects))
	}
	if effects[0].X != target.X || effects[0].Y != target.Y {
		t.Fatalf("Java spawnEffect 第 3-4 個參數使用 pc.getX()/pc.getY()（target 座標），effect 應 spawn 在 (target.X=%d, target.Y=%d)，got=(X=%d, Y=%d)", target.X, target.Y, effects[0].X, effects[0].Y)
	}
}

func TestSkillClanShockStunMobAreaShockStunMap93DoesNotCooldownLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     93,
	})
	npc := &world.NpcInfo{
		ID:          2001,
		NpcID:       990001,
		Impl:        "L1Monster",
		Name:        "mob",
		Level:       50,
		X:           100,
		Y:           100,
		MapID:       93,
		HP:          100,
		MaxHP:       100,
		MP:          100,
		MaxMP:       100,
		AtkSpeed:    1000,
		Agro:        true,
		AggroTarget: target.SessionID,
	}
	ws.AddNpc(npc)
	mobSkillPath := filepath.Join(t.TempDir(), "mob_skill_list.yaml")
	if err := os.WriteFile(mobSkillPath, []byte(`mob_skills:
  - mob_id: 990001
    skills:
      - act_no: 1
        type: 5
        mp_consume: 0
        trigger_random: 1
        trigger_hp: 0
        trigger_range: -2
        skill_id: 0
        act_id: 0
        leverage: 0
        gfx_id: 0
        skill_area: 1
        change_target: 0
        summon_id: 0
        summon_min: 0
        summon_max: 0
`), 0o644); err != nil {
		t.Fatalf("寫入 mob skill 測試資料失敗: %v", err)
	}
	mobSkills, err := data.LoadMobSkillTable(mobSkillPath)
	if err != nil {
		t.Fatalf("讀取 mob skill 測試資料失敗: %v", err)
	}
	s := newSkillTestSystem(t, ws)
	s.deps.MobSkills = mobSkills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.Update(0)

	if npc.AttackTimer != 0 {
		t.Fatalf("Java areashock_stun 在 map 93 會 return false，不應設定技能延遲，AttackTimer=%d", npc.AttackTimer)
	}
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("map 93 不應套用 type 5 SHOCK_STUN，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	if hasActionGfxPacket(drainSkillTestPackets(target.Session), npc.ID, 1) {
		t.Fatal("map 93 不應廣播 type 5 SHOCK_STUN 施法動作")
	}
}

func TestSkillClanShockStunMobAreaShockStunUsesSubMagicSpeedCooldownLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         990002,
		Impl:          "L1Monster",
		Name:          "mob",
		Level:         50,
		X:             100,
		Y:             100,
		MapID:         4,
		HP:            100,
		MaxHP:         100,
		MP:            100,
		MaxMP:         100,
		AtkSpeed:      1000,
		SubMagicSpeed: 1600,
		Agro:          true,
		AggroTarget:   target.SessionID,
	}
	ws.AddNpc(npc)
	mobSkillPath := filepath.Join(t.TempDir(), "mob_skill_list.yaml")
	if err := os.WriteFile(mobSkillPath, []byte(`mob_skills:
  - mob_id: 990002
    skills:
      - act_no: 1
        type: 5
        mp_consume: 0
        trigger_random: 1
        trigger_hp: 0
        trigger_range: -2
        skill_id: 0
        act_id: 0
        leverage: 0
        gfx_id: 0
        skill_area: 1
        change_target: 0
        summon_id: 0
        summon_min: 0
        summon_max: 0
`), 0o644); err != nil {
		t.Fatalf("寫入 mob skill 測試資料失敗: %v", err)
	}
	mobSkills, err := data.LoadMobSkillTable(mobSkillPath)
	if err != nil {
		t.Fatalf("讀取 mob skill 測試資料失敗: %v", err)
	}
	s := newSkillTestSystem(t, ws)
	s.deps.MobSkills = mobSkills
	s.deps.Skill = s

	ai := NewNpcAISystem(ws, s.deps)
	ai.Update(0)

	if npc.AttackTimer != 8 {
		t.Fatalf("Java areashock_stun 成功後使用 sub_magic_speed 設定延遲，AttackTimer=%d want=8", npc.AttackTimer)
	}
}

func TestSkillClanShockStunCallClanCanTargetSameClanAcrossMaps(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "leader",
		X:         100,
		Y:         100,
		MapID:     4,
		ClanID:    7,
		MP:        100,
		MaxMP:     100,
	})
	member := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "member",
		X:         33000,
		Y:         33000,
		MapID:     304,
		ClanID:    7,
	})
	s := newSkillTestSystem(t, ws)

	s.executeBuffSkill(caster.Session, caster, &data.SkillInfo{SkillID: 116, BuffDuration: 0, Target: "buff", ActionID: 19}, member.CharID)

	if member.PendingYesNoType != 729 || member.PendingYesNoData != caster.CharID {
		t.Fatalf("呼喚盟友應可跨地圖對同血盟成員送 729 確認，Pending=(%d,%d)", member.PendingYesNoType, member.PendingYesNoData)
	}
}

func TestSkillClanShockStunRunClanTeleportsToAllowedClanMap(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "leader",
		X:         100,
		Y:         100,
		MapID:     4,
		ClanID:    7,
		MP:        100,
		MaxMP:     100,
	})
	member := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "member",
		X:         32710,
		Y:         32810,
		MapID:     304,
		ClanID:    7,
	})
	s := newSkillTestSystem(t, ws)

	s.executeBuffSkill(caster.Session, caster, &data.SkillInfo{SkillID: 118, BuffDuration: 0, Target: "buff", ActionID: 19}, member.CharID)

	if caster.X != member.X || caster.Y != member.Y || caster.MapID != member.MapID {
		t.Fatalf("援護盟友應傳送到 0/4/304 的同血盟目標位置，got=(%d,%d,%d) want=(%d,%d,%d)",
			caster.X, caster.Y, caster.MapID, member.X, member.Y, member.MapID)
	}
}

func TestSkillClanShockStunRunClanRejectsDisallowedTargetMap(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "leader",
		X:         100,
		Y:         100,
		MapID:     100,
		ClanID:    7,
	})
	member := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "member",
		X:         101,
		Y:         100,
		MapID:     100,
		ClanID:    7,
	})
	s := newSkillTestSystem(t, ws)

	s.executeBuffSkill(caster.Session, caster, &data.SkillInfo{SkillID: 118, BuffDuration: 0, Target: "buff", ActionID: 19}, member.CharID)

	if caster.X != 100 || caster.Y != 100 || caster.MapID != 100 {
		t.Fatalf("援護盟友不應傳送到 Java 禁止的目標地圖，got=(%d,%d,%d)", caster.X, caster.Y, caster.MapID)
	}
}

// TestSkillClanRunClanConsumesMpEvenOnRejectionLikeJava — Java `L1SkillUse.java:481-482` TYPE_NORMAL
// 流程 `runSkill() → useConsume()`，runSkill 進入 skillmode.RUN_CLAN.start 後即使送 647/1192 拒絕仍正常返回，
// useConsume 依然執行。Go 將 consume 提到 canRunClanTeleport 檢查之前對齊 Java——失敗路徑也消耗 MP。
func TestSkillClanRunClanConsumesMpEvenOnRejectionLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "leader",
		X:         100,
		Y:         100,
		MapID:     100, // 非 {0, 4, 304}，會被 canRunClanTeleport 拒絕（其實是 caster's map 對應 escapable check）
		ClanID:    7,
		MP:        50,
		MaxMP:     100,
	})
	member := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "member",
		X:         101,
		Y:         100,
		MapID:     100, // 非 {0, 4, 304} → canRunClanTeleport 失敗
		ClanID:    7,
	})
	s := newSkillTestSystem(t, ws)

	s.executeClanTargetSkill(caster.Session, caster, &data.SkillInfo{
		SkillID:   118,
		MpConsume: 30,
		Target:    "buff",
		ActionID:  19,
	}, member.CharID, "", true)

	if caster.MP != 20 {
		t.Fatalf("Java useConsume 在 RUN_CLAN 失敗路徑仍消耗 MP，預期 50-30=20，got=%d", caster.MP)
	}
	if caster.X != 100 || caster.Y != 100 || caster.MapID != 100 {
		t.Fatalf("RUN_CLAN 失敗時不應傳送，但 caster 位置變更=(%d,%d,%d)", caster.X, caster.Y, caster.MapID)
	}
}

func TestSkillClanShockStunShockStunRequiresTwoHandSwordForNpcTarget(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:        1,
		Session:          newSkillTestSession(t, 1),
		CharID:           1001,
		Name:             "knight",
		X:                100,
		Y:                100,
		MapID:            4,
		Level:            99,
		OriginalMagicHit: 100,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "stun target",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 1,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, npc.ID)
	if npc.HasDebuff(87) || npc.Paralyzed {
		t.Fatalf("Java SHOCK_STUN.start() 在 `getWeapon().getItem().getType1() != 50` 時直接返回，未裝備雙手劍對 NPC 目標不應套用 87，debuff=%v Paralyzed=%v", npc.HasDebuff(87), npc.Paralyzed)
	}
	packets := drainSkillTestPackets(caster.Session)
	if !hasGlobalSystemMessageText(packets, "請使用雙手劍") {
		t.Fatal("Java SHOCK_STUN 未裝備雙手劍時對 NPC 目標也應送 S_SystemMessage(\"請使用雙手劍\")")
	}
	if effects := ws.GetNearbyGroundEffects(npc.X, npc.Y, npc.MapID); len(effects) != 0 {
		t.Fatalf("未裝備雙手劍時不應 spawnEffect(81162)，got %d 筆地面效果", len(effects))
	}

	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	s.executeBuffSkill(caster.Session, caster, skill, npc.ID)
	if !npc.HasDebuff(87) || !npc.Paralyzed {
		t.Fatalf("裝備雙手劍且高等級必中時對 NPC 目標應套用 87，debuff=%v Paralyzed=%v", npc.HasDebuff(87), npc.Paralyzed)
	}
}

func TestSkillClanShockStunShockStunRequiresTwoHandSwordForPlayerTarget(t *testing.T) {
	disablePlayerDebuffMRForStatusTest(t, 87)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)
	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("未裝備雙手劍時衝擊之暈不應套用，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	packets := drainSkillTestPackets(caster.Session)
	if !hasGlobalSystemMessageText(packets, "請使用雙手劍") {
		t.Fatal("Java SHOCK_STUN 未裝備雙手劍時應送 S_SystemMessage(\"請使用雙手劍\")，不可夾帶色碼或額外標點")
	}

	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 16, Equipped: true})
	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)
	if !target.HasBuff(87) || !target.Paralyzed {
		t.Fatalf("裝備雙手劍時衝擊之暈應套用暈眩，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
}

// Java SHOCK_STUN.start(L1PcInstance,...) 第 34 行 `getType1() != 50` 對 NPC 目標
// 同樣排除非 2H 劍。既有 `TestSkillClanShockStunShockStunRequiresTwoHandSwordForNpcTarget`
// 只覆蓋「無武器」與「正確 2H 劍」兩端，本步補上「裝備 1H 劍」中間案例的 NPC
// 目標 negative parallel：weapon_list.yaml `item_id=15`（失去魔力的克特之劍）
// type=sword，鎖定 Go NPC 目標路徑同樣以 `hasTwoHandSwordEquipped` 嚴格判斷
// （不套 87、不送目標 4434、不 spawnEffect、仍送「請使用雙手劍」），與玩家
// 目標的 `OneHandSwordRejectedLikeJava` 配對。
func TestSkillClanShockStunOneHandSwordRejectedForNpcTargetLikeJava(t *testing.T) {
	rand.Seed(1)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:        1,
		Session:          newSkillTestSession(t, 1),
		CharID:           1001,
		Name:             "wrong-weapon-npc",
		X:                100,
		Y:                100,
		MapID:            4,
		Level:            99,
		OriginalMagicHit: 100,
	})
	npc := &world.NpcInfo{
		ID:    world.NextNpcID(),
		NpcID: 45000,
		Impl:  "L1Monster",
		Name:  "stun target",
		X:     101,
		Y:     100,
		MapID: 4,
		Level: 1,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	attachShockStunNpcTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 15, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, npc.ID)

	if npc.HasDebuff(87) || npc.Paralyzed {
		t.Fatalf("Java type!=50 對 NPC 目標同樣嚴格排除 1H 劍，不應套用 87，debuff=%v Paralyzed=%v", npc.HasDebuff(87), npc.Paralyzed)
	}
	if !hasGlobalSystemMessageText(drainSkillTestPackets(caster.Session), "請使用雙手劍") {
		t.Fatal("Java type!=50 對 NPC 目標 1H 劍同樣送 S_SystemMessage(\"請使用雙手劍\")")
	}
	if effects := ws.GetNearbyGroundEffects(npc.X, npc.Y, npc.MapID); len(effects) != 0 {
		t.Fatalf("1H 劍施放 87 對 NPC 目標不應 spawnEffect(81162)，got %d 筆地面效果", len(effects))
	}
}

// Java SHOCK_STUN.start(L1PcInstance,...) 第 34 行 `getType1() != 50` 排除所有
// 非 2H 劍類武器（type 50 = 2H sword）。既有測試只覆蓋「無武器」與「正確 2H 劍」
// 兩端，本步補上「裝備 1H 劍」中間案例：weapon_list.yaml 中 item_id=15
// （失去魔力的克特之劍）type=sword（1H 劍）並非 tohandsword，應被視為「未裝備
// 雙手劍」。鎖定 Go `hasTwoHandSwordEquipped` 對 `type=sword` 嚴格回傳 false，
// 避免未來把判斷誤改為「有劍即可」（type 字串包含 "sword" 寬鬆比對）。
func TestSkillClanShockStunOneHandSwordRejectedLikeJava(t *testing.T) {
	disablePlayerDebuffMRForStatusTest(t, 87)
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "wrong-weapon",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	caster.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: 15, Equipped: true})
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	s.executeBuffSkill(caster.Session, caster, skill, target.CharID)

	if target.HasBuff(87) || target.Paralyzed {
		t.Fatalf("Java type!=50 嚴格排除非 2H 劍，1H 劍（type=sword）施放 87 不應套用，buff=%v Paralyzed=%v", target.GetBuff(87), target.Paralyzed)
	}
	if !hasGlobalSystemMessageText(drainSkillTestPackets(caster.Session), "請使用雙手劍") {
		t.Fatal("Java type!=50 對 1H 劍同樣送 S_SystemMessage(\"請使用雙手劍\")")
	}
}

// Java SHOCK_STUN.start(L1PcInstance,...) 第 31-33 行 `if (srcpc.getId() == cha.getId()) return 0;`
// 會在雙手劍檢查與 setSkillEffect / spawnEffect / S_Paralysis / L1PinkName 之前先返回，
// 對自己施放 87 完全沒有副作用，也不會送 "請使用雙手劍" 系統訊息。
func TestSkillClanShockStunPlayerSelfTargetReturnsImmediatelyLikeJava(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "knight",
		AccessLevel: 200,
		X:           100,
		Y:           100,
		MapID:       4,
	})
	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)
	skill := &data.SkillInfo{SkillID: 87, BuffDuration: 6, Target: "buff", ActionID: 19, CastGfx: 4434}

	_ = drainSkillTestPackets(caster.Session)
	s.executeBuffSkill(caster.Session, caster, skill, caster.CharID)

	if caster.HasBuff(87) {
		t.Fatal("Java SHOCK_STUN 對自己施放會在 setSkillEffect(87) 前返回，不可套用 87 buff")
	}
	if caster.Paralyzed {
		t.Fatal("Java SHOCK_STUN 對自己施放不會 setParalyzed(true)")
	}
	if effects := ws.GetNearbyGroundEffects(caster.X, caster.Y, caster.MapID); len(effects) != 0 {
		t.Fatalf("Java SHOCK_STUN 對自己施放不會 spawnEffect(81162)，got effects=%d", len(effects))
	}
	packets := drainSkillTestPackets(caster.Session)
	if hasGlobalSystemMessageText(packets, "請使用雙手劍") {
		t.Fatal("Java SHOCK_STUN 自我檢查在雙手劍檢查之前，未裝備雙手劍時自我施放也不送 \"請使用雙手劍\"")
	}
	if hasShockStunGMDurationMessage(packets) {
		t.Fatal("Java SHOCK_STUN 對自己施放不會送 GM 秒數訊息")
	}
}

func TestSkillClanShockStunBounceAttackAddsHitOnlyOnce(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "knight",
		X:         100,
		Y:         100,
		MapID:     4,
		HitMod:    2,
	})
	s := newSkillTestSystem(t, ws)
	skill := &data.SkillInfo{SkillID: 89, BuffDuration: 64, Target: "none", ActionID: 19}

	s.executeSelfSkill(player.Session, player, skill)
	s.executeSelfSkill(player.Session, player, skill)

	if !player.HasBuff(89) || player.HitMod != 8 {
		t.Fatalf("尖刺盔甲應依 Java 給 Hit +6 且重放不疊加，buff=%v HitMod=%d", player.GetBuff(89), player.HitMod)
	}
}
