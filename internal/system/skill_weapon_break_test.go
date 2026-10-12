package system

import (
	"testing"

	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

func TestCalcWeaponBreakDurabilityDamageKeepsAtLeastOne(t *testing.T) {
	caster := &world.PlayerInfo{Intel: 2}

	damage := calcWeaponBreakDurabilityDamage(caster)

	if damage != 1 {
		t.Fatalf("低 INT 施放壞物術仍應至少造成 1 點耐久傷害，got=%d", damage)
	}
}

func TestCalcWeaponBreakDurabilityDamageUsesIntDividedByThreeRange(t *testing.T) {
	caster := &world.PlayerInfo{Intel: 12}

	for i := 0; i < 50; i++ {
		damage := calcWeaponBreakDurabilityDamage(caster)
		if damage < 1 || damage > 4 {
			t.Fatalf("INT 12 的壞物術耐久傷害應落在 1..4，got=%d", damage)
		}
	}
}

func TestApplyWeaponBreakDurabilityCapsAtJavaEnchantPlusFive(t *testing.T) {
	weapon := &world.InvItem{Durability: 4}

	changed := applyWeaponBreakDurability(weapon, 4)

	if !changed {
		t.Fatal("耐久未滿時應回報有變更")
	}
	if weapon.Durability != 5 {
		t.Fatalf("Java 武器損壞應封頂在 enchant+5，got=%d", weapon.Durability)
	}
}

func TestApplyWeaponBreakDurabilityIgnoresMissingWeapon(t *testing.T) {
	if applyWeaponBreakDurability(nil, 3) {
		t.Fatal("沒有裝備武器時不應回報耐久變更")
	}
}

func TestSkillWeaponBreakPlayerWeaponCapsAtJavaEnchantPlusFive(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   1,
		Session:     newSkillTestSession(t, 1),
		CharID:      1001,
		Name:        "weapon-break-caster",
		AccessLevel: 200, // 固定命中，僅驗證耐久上限與成功後封包。
		X:           100,
		Y:           100,
		MapID:       4,
		Level:       99,
		MP:          100,
		MaxMP:       100,
		Intel:       3,
		KnownSpells: []int32{27},
	})
	caster.Inv.AddItemWithID(6001, 40318, 1, "魔法寶石", 0, 0, true, 0)
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "weapon-break-target",
		X:         101,
		Y:         100,
		MapID:     4,
		Level:     1,
	})
	weapon := target.Inv.AddItemWithID(7001, 1, 1, "sword", 0, 0, false, 0)
	weapon.Equipped = true
	weapon.Durability = 5
	target.Equip.Set(world.SlotWeapon, weapon)
	s := newSkillBuffTestSystem(t, ws)
	s.deps.Items = loadDurabilityTestItems(t)

	s.processSkill(handler.SkillRequest{
		SessionID: caster.SessionID,
		SkillID:   27,
		TargetID:  target.CharID,
	})

	if weapon.Durability != 5 {
		t.Fatalf("Java receiveDamage 應將 +0 武器損壞封頂在 enchant+5=5，got=%d", weapon.Durability)
	}
	packets := drainSkillTestPackets(target.Session)
	if !hasServerMessage(packets, 268) {
		t.Fatal("壞物術對玩家目標即使武器已達損壞上限仍應送 Java 訊息 268")
	}
	if !hasDurabilityStatusPacket(packets, weapon.ObjectID, 5) {
		t.Fatal("壞物術應送 S_ItemStatus 更新損壞度，而不是只送數量更新")
	}
}

func TestApplyNpcWeaponBreakDamageHalvesBrokenNpcMeleeDamage(t *testing.T) {
	npc := &world.NpcInfo{WeaponBroken: true}

	damage := applyNpcWeaponBreakDamage(npc, 15)

	if damage != 7 {
		t.Fatalf("NPC 被壞物術後近戰傷害應減半並向下取整，got=%d", damage)
	}
}

func TestClearNpcCancellationStateClearsWeaponBreak(t *testing.T) {
	npc := &world.NpcInfo{
		WeaponBroken: true,
		Paralyzed:    true,
		Sleeped:      true,
	}

	clearNpcCancellationState(npc)

	if npc.WeaponBroken {
		t.Fatal("相消術應清除 NPC 壞物術狀態")
	}
}

// Java `L1AttackPc.damagePcWeaponDurability()` 第 3400-3410 行排除三類武器：
//
//	_weaponType == 0（赤手）/ == 20（弓）/ == 62（鐵手甲）
//
// 三者皆 return 不做破壞判定。Go 原本只排除 bow，缺少 claw（鐵手甲）排除，
// 導致鋼爪攻擊 buff 89 玩家時錯誤觸發武器破壞。本測試鎖定 PvP 路徑下 claw
// 不會進入破壞判定，攻擊者武器耐久維持 0。
func TestDamageEquippedWeaponDurabilityExcludesClawInPvpLikeJava(t *testing.T) {
	ws := world.NewState()
	attacker := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "claw-attacker",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	// item_id 152 = 青銅鋼爪，type=claw，對應 Java _weaponType=62
	claw := attacker.Inv.AddItemWithID(5001, 152, 1, "青銅鋼爪", 0, 0, false, 0)
	claw.Equipped = true
	claw.Durability = 0
	attacker.Equip.Set(world.SlotWeapon, claw)

	s := newSkillTestSystem(t, ws)
	attachShockStunItemTable(t, s)

	// pvp=true 模擬攻擊 buff 89 玩家的 PvP 路徑
	damageEquippedWeaponDurability(attacker, s.deps, true)

	if claw.Durability != 0 {
		t.Fatalf("Java 排除 _weaponType=62（鐵手甲），PvP 攻擊 buff 89 玩家時鋼爪不應損壞，got Durability=%d", claw.Durability)
	}
	if hasServerMessage(drainSkillTestPackets(attacker.Session), 268) {
		t.Fatal("Java 排除 claw 時不應送武器損壞訊息 268")
	}
}
