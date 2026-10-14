package system

import (
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/scripting"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

type fakePvPManager struct {
	meleeCalls int
	farCalls   int
}

func (m *fakePvPManager) HandlePvPAttack(_, _ *world.PlayerInfo) {
	m.meleeCalls++
}

func (m *fakePvPManager) HandlePvPFarAttack(_, _ *world.PlayerInfo) {
	m.farCalls++
}

func (m *fakePvPManager) TriggerPinkName(_, _ *world.PlayerInfo) {}

func (m *fakePvPManager) AddLawfulFromNpc(_ *world.PlayerInfo, _ int32) {}

func (m *fakePvPManager) BeginHell(_ *world.PlayerInfo, _ bool) {}

func newCombatLOSTestSystem(t *testing.T, ws *world.State, pvp *fakePvPManager) *CombatSystem {
	t.Helper()
	engine, err := scripting.NewEngine("../../scripts", zap.NewNop())
	if err != nil {
		t.Fatalf("建立 Lua engine 失敗: %v", err)
	}
	items, err := data.LoadItemTable(
		"../../data/yaml/weapon_list.yaml",
		"../../data/yaml/armor_list.yaml",
		"../../data/yaml/etcitem_list.yaml",
	)
	if err != nil {
		t.Fatalf("載入物品表失敗: %v", err)
	}
	return &CombatSystem{deps: &handler.Deps{
		World:     ws,
		Scripting: engine,
		MapData:   newSkillLOSTestMap(t),
		Items:     items,
		Log:       zap.NewNop(),
		PvP:       pvp,
	}}
}

func TestMeleeAttackSkipsNpcBehindWall(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "attacker",
		X:         101,
		Y:         100,
		MapID:     900,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		Impl:  "L1Monster",
		Name:  "behind_wall",
		X:     103,
		Y:     100,
		MapID: 900,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newCombatLOSTestSystem(t, ws, &fakePvPManager{})

	s.processMeleeAttack(player.SessionID, npc.ID)
	packets := drainSkillTestPackets(player.Session)

	if len(packets) != 0 {
		t.Fatalf("隔牆近戰不應該送出攻擊封包，packets=%d", len(packets))
	}
	if npc.HP != 100 {
		t.Fatalf("隔牆近戰不應該傷害 NPC，HP=%d", npc.HP)
	}
}

func TestMeleeAttackSkipsNpcInDifferentShowLikeJava(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "attacker",
		X:         100,
		Y:         100,
		MapID:     900,
		ShowID:    0,
	})
	npc := &world.NpcInfo{
		ID:     2001,
		Impl:   "L1Monster",
		Name:   "other_show",
		X:      101,
		Y:      100,
		MapID:  900,
		ShowID: 100,
		HP:     100,
		MaxHP:  100,
	}
	ws.AddNpc(npc)
	s := newCombatLOSTestSystem(t, ws, &fakePvPManager{})

	s.processMeleeAttack(player.SessionID, npc.ID)

	if hasOpcodePacket(drainSkillTestPackets(player.Session), packet.S_OPCODE_ATTACK) {
		t.Fatalf("不同 ShowID 近戰不應送出攻擊封包")
	}
	if npc.HP != 100 {
		t.Fatalf("不同 ShowID 近戰不應傷害 NPC，HP=%d", npc.HP)
	}
}

func TestMeleeAttackBroadcastsOnlySameShowLikeJava(t *testing.T) {
	ws := world.NewState()
	attacker := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "attacker",
		X:         100,
		Y:         100,
		MapID:     900,
		ShowID:    100,
	})
	sameShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "same_show",
		X:         101,
		Y:         100,
		MapID:     900,
		ShowID:    100,
	})
	otherShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    1003,
		Name:      "other_show",
		X:         101,
		Y:         100,
		MapID:     900,
		ShowID:    200,
	})
	npc := &world.NpcInfo{
		ID:     2001,
		Impl:   "L1Monster",
		Name:   "dummy",
		X:      101,
		Y:      100,
		MapID:  900,
		ShowID: 100,
		HP:     100,
		MaxHP:  100,
	}
	ws.AddNpc(npc)
	s := newCombatLOSTestSystem(t, ws, &fakePvPManager{})

	s.processMeleeAttack(attacker.SessionID, npc.ID)

	if !hasOpcodePacket(drainSkillTestPackets(sameShow.Session), packet.S_OPCODE_ATTACK) {
		t.Fatalf("同 ShowID 觀眾應收到近戰攻擊封包")
	}
	if hasOpcodePacket(drainSkillTestPackets(otherShow.Session), packet.S_OPCODE_ATTACK) {
		t.Fatalf("不同 ShowID 觀眾不應收到近戰攻擊封包")
	}
}

func TestMeleeAttackFieldObjectBroadcastsOnlySameShowLikeJava(t *testing.T) {
	ws := world.NewState()
	attacker := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "attacker",
		X:         100,
		Y:         100,
		MapID:     900,
		ShowID:    100,
	})
	sameShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "same_show",
		X:         101,
		Y:         100,
		MapID:     900,
		ShowID:    100,
	})
	otherShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    1003,
		Name:      "other_show",
		X:         101,
		Y:         100,
		MapID:     900,
		ShowID:    200,
	})
	npc := &world.NpcInfo{
		ID:     2001,
		NpcID:  90000,
		Impl:   "L1FieldObject",
		Name:   "field_object",
		X:      101,
		Y:      100,
		MapID:  900,
		ShowID: 100,
		HP:     100,
		MaxHP:  100,
	}
	ws.AddNpc(npc)
	s := newCombatLOSTestSystem(t, ws, &fakePvPManager{})

	s.processMeleeAttack(attacker.SessionID, npc.ID)

	if !hasOpcodePacket(drainSkillTestPackets(sameShow.Session), packet.S_OPCODE_ATTACK) {
		t.Fatalf("同 ShowID 觀眾應收到 field object 近戰動畫")
	}
	if hasOpcodePacket(drainSkillTestPackets(otherShow.Session), packet.S_OPCODE_ATTACK) {
		t.Fatalf("不同 ShowID 觀眾不應收到 field object 近戰動畫")
	}
}

func TestRangedAttackSkipsNpcBehindWall(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "attacker",
		X:         101,
		Y:         100,
		MapID:     900,
	})
	player.Equip.Set(world.SlotWeapon, &world.InvItem{ItemID: 190})
	npc := &world.NpcInfo{
		ID:    2001,
		Impl:  "L1Monster",
		Name:  "behind_wall",
		X:     103,
		Y:     100,
		MapID: 900,
		HP:    100,
		MaxHP: 100,
	}
	ws.AddNpc(npc)
	s := newCombatLOSTestSystem(t, ws, &fakePvPManager{})

	s.processRangedAttack(player.SessionID, npc.ID)
	packets := drainSkillTestPackets(player.Session)

	if len(packets) != 0 {
		t.Fatalf("隔牆遠攻不應該送出攻擊封包，packets=%d", len(packets))
	}
	if npc.HP != 100 {
		t.Fatalf("隔牆遠攻不應該傷害 NPC，HP=%d", npc.HP)
	}
}

func TestRangedAttackSkipsNpcInDifferentShowLikeJava(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:     1,
		Session:       newSkillTestSession(t, 1),
		CharID:        1001,
		Name:          "attacker",
		X:             100,
		Y:             100,
		MapID:         900,
		ShowID:        0,
		CurrentWeapon: 20,
	})
	player.Equip.Set(world.SlotWeapon, &world.InvItem{ItemID: 190, ObjectID: 5001, Equipped: true})
	npc := &world.NpcInfo{
		ID:     2001,
		Impl:   "L1Monster",
		Name:   "other_show",
		X:      101,
		Y:      100,
		MapID:  900,
		ShowID: 100,
		HP:     100,
		MaxHP:  100,
	}
	ws.AddNpc(npc)
	s := newCombatLOSTestSystem(t, ws, &fakePvPManager{})

	s.processRangedAttack(player.SessionID, npc.ID)

	if hasOpcodePacket(drainSkillTestPackets(player.Session), packet.S_OPCODE_ATTACK) {
		t.Fatalf("不同 ShowID 遠攻不應送出攻擊封包")
	}
	if npc.HP != 100 {
		t.Fatalf("不同 ShowID 遠攻不應傷害 NPC，HP=%d", npc.HP)
	}
}

func TestRangedAttackBroadcastsOnlySameShowLikeJava(t *testing.T) {
	ws := world.NewState()
	attacker := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:     1,
		Session:       newSkillTestSession(t, 1),
		CharID:        1001,
		Name:          "attacker",
		X:             100,
		Y:             100,
		MapID:         900,
		ShowID:        100,
		CurrentWeapon: 20,
	})
	attacker.Equip.Set(world.SlotWeapon, &world.InvItem{ItemID: 190, ObjectID: 5001, Equipped: true})
	sameShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "same_show",
		X:         101,
		Y:         100,
		MapID:     900,
		ShowID:    100,
	})
	otherShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    1003,
		Name:      "other_show",
		X:         101,
		Y:         100,
		MapID:     900,
		ShowID:    200,
	})
	npc := &world.NpcInfo{
		ID:     2001,
		Impl:   "L1Monster",
		Name:   "dummy",
		X:      101,
		Y:      100,
		MapID:  900,
		ShowID: 100,
		HP:     100,
		MaxHP:  100,
	}
	ws.AddNpc(npc)
	s := newCombatLOSTestSystem(t, ws, &fakePvPManager{})

	s.processRangedAttack(attacker.SessionID, npc.ID)

	if !hasOpcodePacket(drainSkillTestPackets(sameShow.Session), packet.S_OPCODE_ATTACK) {
		t.Fatalf("同 ShowID 觀眾應收到遠攻封包")
	}
	if hasOpcodePacket(drainSkillTestPackets(otherShow.Session), packet.S_OPCODE_ATTACK) {
		t.Fatalf("不同 ShowID 觀眾不應收到遠攻封包")
	}
}

func TestRangedAttackFieldObjectBroadcastsOnlySameShowLikeJava(t *testing.T) {
	ws := world.NewState()
	attacker := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:     1,
		Session:       newSkillTestSession(t, 1),
		CharID:        1001,
		Name:          "attacker",
		X:             100,
		Y:             100,
		MapID:         900,
		ShowID:        100,
		CurrentWeapon: 20,
	})
	attacker.Equip.Set(world.SlotWeapon, &world.InvItem{ItemID: 190, ObjectID: 5001, Equipped: true})
	sameShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "same_show",
		X:         101,
		Y:         100,
		MapID:     900,
		ShowID:    100,
	})
	otherShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    1003,
		Name:      "other_show",
		X:         101,
		Y:         100,
		MapID:     900,
		ShowID:    200,
	})
	npc := &world.NpcInfo{
		ID:     2001,
		NpcID:  90000,
		Impl:   "L1FieldObject",
		Name:   "field_object",
		X:      101,
		Y:      100,
		MapID:  900,
		ShowID: 100,
		HP:     100,
		MaxHP:  100,
	}
	ws.AddNpc(npc)
	s := newCombatLOSTestSystem(t, ws, &fakePvPManager{})

	s.processRangedAttack(attacker.SessionID, npc.ID)

	if !hasOpcodePacket(drainSkillTestPackets(sameShow.Session), packet.S_OPCODE_ATTACK) {
		t.Fatalf("同 ShowID 觀眾應收到 field object 遠攻動畫")
	}
	if hasOpcodePacket(drainSkillTestPackets(otherShow.Session), packet.S_OPCODE_ATTACK) {
		t.Fatalf("不同 ShowID 觀眾不應收到 field object 遠攻動畫")
	}
}

func TestScarecrowHitBroadcastsOnlySameShowLikeJava(t *testing.T) {
	ws := world.NewState()
	attacker := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "attacker",
		X:         100,
		Y:         100,
		MapID:     900,
		ShowID:    100,
	})
	sameShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "same_show",
		X:         101,
		Y:         100,
		MapID:     900,
		ShowID:    100,
	})
	otherShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    1003,
		Name:      "other_show",
		X:         101,
		Y:         100,
		MapID:     900,
		ShowID:    200,
	})
	npc := &world.NpcInfo{
		ID:     2001,
		Impl:   "L1Scarecrow",
		Name:   "scarecrow",
		X:      101,
		Y:      100,
		MapID:  900,
		ShowID: 100,
		HP:     100,
		MaxHP:  100,
	}
	ws.AddNpc(npc)
	s := newCombatLOSTestSystem(t, ws, &fakePvPManager{})

	s.processMeleeAttack(attacker.SessionID, npc.ID)

	if !hasOpcodePacket(drainSkillTestPackets(sameShow.Session), packet.S_OPCODE_ATTACK) {
		t.Fatalf("同 ShowID 觀眾應收到木人攻擊動畫")
	}
	if hasOpcodePacket(drainSkillTestPackets(otherShow.Session), packet.S_OPCODE_ATTACK) {
		t.Fatalf("不同 ShowID 觀眾不應收到木人攻擊動畫")
	}
}

func TestMeleeAttackSkipsPlayerBehindWall(t *testing.T) {
	ws := world.NewState()
	attacker := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "attacker",
		X:         101,
		Y:         100,
		MapID:     900,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         103,
		Y:         100,
		MapID:     900,
	})
	pvp := &fakePvPManager{}
	s := newCombatLOSTestSystem(t, ws, pvp)

	s.processMeleeAttack(attacker.SessionID, target.CharID)

	if pvp.meleeCalls != 0 {
		t.Fatalf("隔牆近戰 PvP 不應該委派傷害，calls=%d", pvp.meleeCalls)
	}
}

func TestRangedAttackSkipsPlayerBehindWall(t *testing.T) {
	ws := world.NewState()
	attacker := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "attacker",
		X:         101,
		Y:         100,
		MapID:     900,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         103,
		Y:         100,
		MapID:     900,
	})
	pvp := &fakePvPManager{}
	s := newCombatLOSTestSystem(t, ws, pvp)

	s.processRangedAttack(attacker.SessionID, target.CharID)

	if pvp.farCalls != 0 {
		t.Fatalf("隔牆遠攻 PvP 不應該委派傷害，calls=%d", pvp.farCalls)
	}
}

// TestTripleArrowDirectInvocationDamage 直接呼叫 ExecuteRangedAttackOnNpc 3 次模擬 skill 132 主迴圈，
// 量測每次擊中的傷害值與總 HP 損失，確認每次 call 套用一發完整 bow 攻擊（不帶 ×5 倍率，
// Go 設計刻意捨棄 Java ConfigSkill.TRIPLE_ARROW_DMG=5 倍率以維持遊戲平衡）。
func TestTripleArrowDirectInvocationDamage(t *testing.T) {
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:     1,
		Session:       newSkillTestSession(t, 1),
		CharID:        1001,
		Name:          "archer",
		X:             100,
		Y:             100,
		MapID:         900,
		Level:         70,
		Str:           30,
		Dex:           30,
		BowHitMod:     50,
		BowDmgMod:     0,
		CurrentWeapon: 20,
	})
	player.Equip.Set(world.SlotWeapon, &world.InvItem{ItemID: 190, ObjectID: 5001, Equipped: true}) // 沙哈之弓（無箭可發魔法箭）

	// NPC 放 x=101 避開測試地圖 x=102 的箭矢牆（tile=3）。
	npc := &world.NpcInfo{
		ID:    2001,
		Impl:  "L1Monster",
		Name:  "dummy",
		X:     101,
		Y:     100,
		MapID: 900,
		HP:    100000,
		MaxHP: 100000,
		AC:    0,
		Size:  "small",
		Level: 1,
	}
	ws.AddNpc(npc)
	s := newCombatLOSTestSystem(t, ws, &fakePvPManager{})

	// 先量測「單次普通遠程攻擊」基底傷害取樣（30 次取最大值）作為上限參考
	baseStartHP := npc.HP
	maxSingleHit := int32(0)
	for i := 0; i < 30; i++ {
		hpBefore := npc.HP
		s.processRangedAttackForPlayer(player, npc.ID)
		hit := hpBefore - npc.HP
		if hit > maxSingleHit {
			maxSingleHit = hit
		}
	}
	t.Logf("單次普通遠程攻擊取樣最大傷害 = %d（30 次取樣）", maxSingleHit)
	npc.HP = baseStartHP // 回復測量基準

	// 模擬 skill 132 主迴圈：3× ExecuteRangedAttackOnNpc。
	hpBefore := npc.HP
	perHitDamages := make([]int32, 0, 3)
	for i := 0; i < 3; i++ {
		before := npc.HP
		s.ExecuteRangedAttackOnNpc(player, npc.ID)
		perHitDamages = append(perHitDamages, before-npc.HP)
	}
	totalLoss := hpBefore - npc.HP

	t.Logf("三重矢 3 次擊中傷害分布 = %v，總 HP 損失 = %d", perHitDamages, totalLoss)

	// 上限檢查：每次擊中應與單次普通遠程攻擊同範圍（不套 ×5 倍率），
	// 給 1.5 倍 buffer 涵蓋取樣未覆蓋的高骰結果。
	upper := maxSingleHit * 3 / 2
	for i, d := range perHitDamages {
		if d > upper {
			t.Fatalf("第 %d 次擊中傷害 %d 超過合理上限 %d（單次普通 × 1.5 buffer），疑似套用 ×5 倍率或重複計算",
				i+1, d, upper)
		}
	}

	// 總損失上限：3 × 單次普通 × 1.5 buffer。
	totalUpper := maxSingleHit * 3 * 3 / 2
	if totalLoss > totalUpper {
		t.Fatalf("總 HP 損失 %d 超過合理上限 %d，疑似套用 ×5 倍率", totalLoss, totalUpper)
	}
}
