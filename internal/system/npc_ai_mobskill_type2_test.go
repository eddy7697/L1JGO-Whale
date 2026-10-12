package system

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/world"
)

func TestNpcMobSkillTypeTwoAttackUsesAtkMagicSpeedCooldownLikeJava(t *testing.T) {
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45197,
		Impl:          "L1Monster",
		Name:          "vampiric caster",
		X:             100,
		Y:             100,
		MapID:         900,
		HP:            100,
		MaxHP:         100,
		MP:            100,
		MaxMP:         100,
		Level:         50,
		STR:           30,
		DEX:           30,
		Intel:         18,
		AtkDmg:        20,
		Ranged:        1,
		AtkSpeed:      200,
		AtkMagicSpeed: 1200,
		SubMagicSpeed: 1800,
		AggroTarget:   target.SessionID,
	}
	ws.AddNpc(npc)
	s := newNpcAILOSTestSystem(t, ws)
	withNpcTypeTwoMobSkill(t, s, 28, 0)

	s.tickMonsterAI(npc)

	if npc.AttackTimer != 6 {
		t.Fatalf("Yiwei type 2 target=attack 應使用 atk_magic_speed 冷卻，AttackTimer=%d want=6", npc.AttackTimer)
	}
	if npc.MoveTimer != npc.AttackTimer {
		t.Fatalf("Yiwei NpcAI 使用同一個 sleepTime 阻擋下一輪移動，MoveTimer=%d AttackTimer=%d", npc.MoveTimer, npc.AttackTimer)
	}
	if target.HP >= target.MaxHP {
		t.Fatalf("測試前置失敗：type 2 攻擊魔法應命中目標，HP=%d MaxHP=%d", target.HP, target.MaxHP)
	}
}

func TestNpcMobSkillTypeTwoBuffUsesSubMagicSpeedCooldownLikeJava(t *testing.T) {
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45197,
		Impl:          "L1Monster",
		Name:          "self healer",
		X:             100,
		Y:             100,
		MapID:         900,
		HP:            10,
		MaxHP:         100,
		MP:            100,
		MaxMP:         100,
		Level:         50,
		STR:           30,
		DEX:           30,
		Intel:         18,
		AtkDmg:        20,
		Ranged:        1,
		AtkSpeed:      200,
		AtkMagicSpeed: 1200,
		SubMagicSpeed: 1400,
		AggroTarget:   target.SessionID,
	}
	ws.AddNpc(npc)
	s := newNpcAILOSTestSystem(t, ws)
	withNpcTypeTwoMobSkill(t, s, 1, 2)

	s.tickMonsterAI(npc)

	if npc.AttackTimer != 7 {
		t.Fatalf("Yiwei type 2 target!=attack 應使用 sub_magic_speed 冷卻，AttackTimer=%d want=7", npc.AttackTimer)
	}
	if npc.MoveTimer != npc.AttackTimer {
		t.Fatalf("Yiwei NpcAI 使用同一個 sleepTime 阻擋下一輪移動，MoveTimer=%d AttackTimer=%d", npc.MoveTimer, npc.AttackTimer)
	}
	if npc.HP <= 10 {
		t.Fatalf("測試前置失敗：type 2 自補應成功，HP=%d", npc.HP)
	}
}

func TestNpcMobSkillTypeTwoAttackLeverageScalesMagicDamageLikeJava(t *testing.T) {
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45197,
		Impl:          "L1Monster",
		Name:          "leveraged caster",
		X:             100,
		Y:             100,
		MapID:         900,
		HP:            100,
		MaxHP:         100,
		MP:            100,
		MaxMP:         100,
		Level:         50,
		STR:           30,
		DEX:           30,
		AtkDmg:        20,
		Ranged:        1,
		AtkSpeed:      200,
		AtkMagicSpeed: 1200,
		SubMagicSpeed: 1400,
		AggroTarget:   target.SessionID,
	}
	ws.AddNpc(npc)
	s := newNpcAILOSTestSystem(t, ws)
	withNpcTypeTwoFixedDamageMobSkill(t, s, 990001, 20)

	s.tickMonsterAI(npc)

	gotDamage := target.MaxHP - target.HP
	if gotDamage != 86 {
		t.Fatalf("Yiwei type 2 magicAttack leverage=20 應把魔法傷害加倍，damage=%d want=86", gotDamage)
	}
}

func TestNpcMobSkillTypeTwoAttackRespectsSkillRangedLikeJava(t *testing.T) {
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         105,
		Y:         100,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45197,
		Impl:          "L1Monster",
		Name:          "ranged gate caster",
		X:             100,
		Y:             100,
		MapID:         900,
		HP:            100,
		MaxHP:         100,
		MP:            100,
		MaxMP:         100,
		Level:         50,
		STR:           30,
		DEX:           30,
		AtkDmg:        20,
		Ranged:        1,
		AtkSpeed:      200,
		AtkMagicSpeed: 1200,
		SubMagicSpeed: 1400,
		AggroTarget:   target.SessionID,
	}
	ws.AddNpc(npc)
	s := newNpcAIPhysicalAreaTestSystem(t, ws)
	withNpcTypeTwoMobSkill(t, s, 28, 0)

	s.tickMonsterAI(npc)

	if target.HP != target.MaxHP {
		t.Fatalf("Yiwei makeTargetList 應用 skill.ranged=4 擋掉距離 5 的 type 2 魔法，HP=%d want=%d", target.HP, target.MaxHP)
	}
	if npc.MP != 100 {
		t.Fatalf("射程外 checkUseSkill=false 時不應消耗技能 MP，MP=%d want=100", npc.MP)
	}
	if npc.AttackTimer != 0 {
		t.Fatalf("射程外 type 2 失敗不應進入施法冷卻，AttackTimer=%d want=0", npc.AttackTimer)
	}
	if npc.MobSkillUseCounts != nil && npc.MobSkillUseCounts[0] != 0 {
		t.Fatalf("射程外 type 2 失敗不應累計 TriCount，count=%d want=0", npc.MobSkillUseCounts[0])
	}
}

func TestNpcMobSkillTypeTwoRangedMinusOneRequiresScreenLikeJava(t *testing.T) {
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         110,
		Y:         109,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45197,
		Impl:          "L1Monster",
		Name:          "screen gate caster",
		X:             100,
		Y:             100,
		MapID:         900,
		HP:            100,
		MaxHP:         100,
		MP:            100,
		MaxMP:         100,
		Level:         50,
		STR:           30,
		DEX:           30,
		AtkDmg:        20,
		Ranged:        1,
		AtkSpeed:      200,
		AtkMagicSpeed: 1200,
		SubMagicSpeed: 1400,
		AggroTarget:   target.SessionID,
	}
	ws.AddNpc(npc)
	s := newNpcAIPhysicalAreaTestSystem(t, ws)
	withNpcTypeTwoFixedDamageRangedMobSkill(t, s, 990002, 0, -1, -30)

	s.tickMonsterAI(npc)

	if target.HP != target.MaxHP {
		t.Fatalf("Yiwei ranged=-1 應用 isInScreen 擋掉畫面邊界外的 type 2 魔法，HP=%d want=%d", target.HP, target.MaxHP)
	}
	if npc.AttackTimer != 0 {
		t.Fatalf("畫面外 type 2 失敗不應進入施法冷卻，AttackTimer=%d want=0", npc.AttackTimer)
	}
	if npc.MobSkillUseCounts != nil && npc.MobSkillUseCounts[0] != 0 {
		t.Fatalf("畫面外 type 2 失敗不應累計 TriCount，count=%d want=0", npc.MobSkillUseCounts[0])
	}
}

func TestNpcMobSkillTypeTwoRejectsInvalidPlayerTargetLikeJava(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*world.PlayerInfo)
	}{
		{
			name: "dead target",
			mutate: func(target *world.PlayerInfo) {
				target.Dead = true
			},
		},
		{
			name: "different map",
			mutate: func(target *world.PlayerInfo) {
				target.MapID = 901
			},
		},
		{
			name: "different show",
			mutate: func(target *world.PlayerInfo) {
				target.ShowID = 7
			},
		},
		{
			name: "gm invisible",
			mutate: func(target *world.PlayerInfo) {
				target.AccessLevel = 200
				target.GMInvisible = true
				target.Invisible = true
			},
		},
		{
			name: "absolute barrier",
			mutate: func(target *world.PlayerInfo) {
				target.AbsoluteBarrier = true
				target.AddBuff(&world.ActiveBuff{SkillID: 78, TicksLeft: 100, SetAbsoluteBarrier: true})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws := world.NewState()
			target := addSkillTestPlayer(ws, &world.PlayerInfo{
				SessionID: 1,
				Session:   newSkillTestSession(t, 1),
				CharID:    1001,
				Name:      "target",
				X:         101,
				Y:         100,
				MapID:     900,
				HP:        5000,
				MaxHP:     5000,
			})
			npc := &world.NpcInfo{
				ID:            2001,
				NpcID:         45197,
				Impl:          "L1Monster",
				Name:          "invalid target caster",
				X:             100,
				Y:             100,
				MapID:         900,
				HP:            100,
				MaxHP:         100,
				MP:            100,
				MaxMP:         100,
				Level:         50,
				STR:           30,
				DEX:           30,
				AtkDmg:        20,
				Ranged:        1,
				AtkSpeed:      200,
				AtkMagicSpeed: 1200,
				SubMagicSpeed: 1400,
				AggroTarget:   target.SessionID,
			}
			ws.AddNpc(npc)
			tc.mutate(target)
			s := newNpcAIPhysicalAreaTestSystem(t, ws)
			withNpcTypeTwoFixedDamageMobSkill(t, s, 990003, 0)

			used := s.executeNpcSkill(npc, target, 990003, 18, 0, 0)

			if used {
				t.Fatalf("Yiwei checkUseSkill/isTarget 應拒絕 %s 的非 87 type 2 目標", tc.name)
			}
			if target.HP != target.MaxHP {
				t.Fatalf("非法目標不應受到 type 2 魔法傷害，HP=%d want=%d", target.HP, target.MaxHP)
			}
			if target.Dirty {
				t.Fatal("非法目標不應被標記 Dirty")
			}
			if npc.AggroTarget != target.SessionID {
				t.Fatalf("checkUseSkill=false 只應讓技能失敗，不應清掉怪物仇恨目標，AggroTarget=%d want=%d", npc.AggroTarget, target.SessionID)
			}
		})
	}
}

func TestNpcMobSkillTypeTwoAreaFiltersExtraTargetsLikeJava(t *testing.T) {
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     900,
		ShowID:    3,
		HP:        5000,
		MaxHP:     5000,
	})
	sameShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "same-show",
		X:         101,
		Y:         101,
		MapID:     900,
		ShowID:    3,
		HP:        5000,
		MaxHP:     5000,
	})
	otherShow := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    1003,
		Name:      "other-show",
		X:         102,
		Y:         100,
		MapID:     900,
		ShowID:    8,
		HP:        5000,
		MaxHP:     5000,
	})
	gmInvisible := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   4,
		Session:     newSkillTestSession(t, 4),
		CharID:      1004,
		Name:        "gm-invisible",
		X:           102,
		Y:           101,
		MapID:       900,
		ShowID:      3,
		HP:          5000,
		MaxHP:       5000,
		AccessLevel: 200,
		GMInvisible: true,
		Invisible:   true,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45197,
		Impl:          "L1Monster",
		Name:          "area caster",
		X:             100,
		Y:             100,
		MapID:         900,
		ShowID:        3,
		HP:            100,
		MaxHP:         100,
		MP:            100,
		MaxMP:         100,
		Level:         50,
		STR:           30,
		DEX:           30,
		AtkDmg:        20,
		Ranged:        1,
		AtkSpeed:      200,
		AtkMagicSpeed: 1200,
		SubMagicSpeed: 1400,
		AggroTarget:   target.SessionID,
	}
	ws.AddNpc(npc)
	s := newNpcAIPhysicalAreaTestSystem(t, ws)
	withNpcTypeTwoFixedDamageAreaMobSkill(t, s, 990004, 2)

	used := s.executeNpcSkill(npc, target, 990004, 18, 0, 0)

	if !used {
		t.Fatal("合法主目標的 type 2 AoE 魔法應施放成功")
	}
	if target.HP >= target.MaxHP || sameShow.HP >= sameShow.MaxHP {
		t.Fatalf("同 showId 且範圍內玩家應受 AoE 傷害，targetHP=%d sameShowHP=%d", target.HP, sameShow.HP)
	}
	if otherShow.HP != otherShow.MaxHP {
		t.Fatalf("Yiwei AoE makeTargetList/isTarget 應排除不同 showId 玩家，HP=%d want=%d", otherShow.HP, otherShow.MaxHP)
	}
	if gmInvisible.HP != gmInvisible.MaxHP {
		t.Fatalf("Yiwei AoE makeTargetList/isTarget 應排除 GM 隱身玩家，HP=%d want=%d", gmInvisible.HP, gmInvisible.MaxHP)
	}
}

func TestNpcMobSkillTypeTwoAreaSkipsWallBlockedExtraTargetLikeJava(t *testing.T) {
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	wallBlocked := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "wall-blocked",
		X:         103,
		Y:         100,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45197,
		Impl:          "L1Monster",
		Name:          "area los caster",
		X:             100,
		Y:             100,
		MapID:         900,
		HP:            100,
		MaxHP:         100,
		MP:            100,
		MaxMP:         100,
		Level:         50,
		STR:           30,
		DEX:           30,
		AtkDmg:        20,
		Ranged:        1,
		AtkSpeed:      200,
		AtkMagicSpeed: 1200,
		SubMagicSpeed: 1400,
		AggroTarget:   target.SessionID,
	}
	ws.AddNpc(npc)
	s := newNpcAILOSTestSystem(t, ws)
	withNpcTypeTwoFixedDamageAreaMobSkill(t, s, 990005, 2)

	used := s.executeNpcSkill(npc, target, 990005, 18, 0, 0)

	if !used {
		t.Fatal("主目標有 LOS 時 type 2 AoE 魔法應施放成功")
	}
	if target.HP >= target.MaxHP {
		t.Fatalf("主目標應受到 AoE 傷害，HP=%d MaxHP=%d", target.HP, target.MaxHP)
	}
	if wallBlocked.HP != wallBlocked.MaxHP {
		t.Fatalf("Yiwei AoE isTarget 應排除對 NPC 隔牆且 non-through 的額外目標，HP=%d want=%d", wallBlocked.HP, wallBlocked.MaxHP)
	}
}

func TestNpcMobSkillTypeTwoTargetNoneAreaCentersOnCasterLikeJava(t *testing.T) {
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         106,
		Y:         100,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	nearCaster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "near-caster",
		X:         100,
		Y:         101,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45197,
		Impl:          "L1Monster",
		Name:          "self area caster",
		X:             100,
		Y:             100,
		MapID:         900,
		HP:            100,
		MaxHP:         100,
		MP:            100,
		MaxMP:         100,
		Level:         50,
		STR:           30,
		DEX:           30,
		AtkDmg:        20,
		Ranged:        1,
		AtkSpeed:      200,
		AtkMagicSpeed: 1200,
		SubMagicSpeed: 1400,
		AggroTarget:   target.SessionID,
	}
	ws.AddNpc(npc)
	s := newNpcAIPhysicalAreaTestSystem(t, ws)
	withNpcTypeTwoFixedDamageTargetMobSkill(t, s, 990006, "none", 0, 1)

	used := s.executeNpcSkill(npc, target, 990006, 18, 0, 0)

	if !used {
		t.Fatal("Yiwei target=none type 2 應以 NPC 自身通過 ranged=0 並成功施放")
	}
	if nearCaster.HP >= nearCaster.MaxHP {
		t.Fatalf("target=none AoE 應以 NPC 為中心傷害附近玩家，HP=%d MaxHP=%d", nearCaster.HP, nearCaster.MaxHP)
	}
	if target.HP != target.MaxHP {
		t.Fatalf("仇恨目標不在 NPC-centered area 內時不應受傷，HP=%d want=%d", target.HP, target.MaxHP)
	}
}

func TestNpcMobSkillTypeTwoTargetNoneIgnoresAggroTargetWallLikeJava(t *testing.T) {
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "wall-target",
		X:         103,
		Y:         100,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	nearCaster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "near-caster",
		X:         101,
		Y:         100,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45197,
		Impl:          "L1Monster",
		Name:          "self area los caster",
		X:             100,
		Y:             100,
		MapID:         900,
		HP:            100,
		MaxHP:         100,
		MP:            100,
		MaxMP:         100,
		Level:         50,
		STR:           30,
		DEX:           30,
		AtkDmg:        20,
		Ranged:        1,
		AtkSpeed:      200,
		AtkMagicSpeed: 1200,
		SubMagicSpeed: 1400,
		AggroTarget:   target.SessionID,
	}
	ws.AddNpc(npc)
	s := newNpcAILOSTestSystem(t, ws)
	withNpcTypeTwoFixedDamageTargetMobSkill(t, s, 990007, "none", 0, 1)

	used := s.executeNpcSkill(npc, target, 990007, 18, 0, 0)

	if !used {
		t.Fatal("Yiwei target=none 會把技能目標重設為 NPC 自己，不應因仇恨玩家隔牆而施放失敗")
	}
	if nearCaster.HP >= nearCaster.MaxHP {
		t.Fatalf("仇恨玩家隔牆時，target=none AoE 仍應以 NPC 為中心傷害身旁玩家，HP=%d MaxHP=%d", nearCaster.HP, nearCaster.MaxHP)
	}
	if target.HP != target.MaxHP {
		t.Fatalf("隔牆仇恨玩家不在 NPC-centered area 內時不應受傷，HP=%d want=%d", target.HP, target.MaxHP)
	}
}

func withNpcTypeTwoMobSkill(t *testing.T, s *NpcAISystem, skillID, changeTarget int) {
	t.Helper()
	skills, err := data.LoadSkillTable(filepath.Join("..", "..", "data", "yaml", "skill_list.yaml"))
	if err != nil {
		t.Fatalf("載入技能資料失敗: %v", err)
	}

	dir := t.TempDir()
	mobSkillPath := filepath.Join(dir, "mob_skill_list.yaml")
	raw := []byte(fmt.Sprintf(`mob_skills:
  - mob_id: 45197
    skills:
      - act_no: 0
        name: type-two-magic
        type: 2
        mp_consume: 0
        trigger_random: 1
        trigger_hp: 0
        trigger_companion_hp: 0
        trigger_range: -14
        trigger_count: 0
        change_target: %d
        range: 0
        area_width: 0
        area_height: 0
        leverage: 0
        skill_id: %d
        skill_area: 0
        gfx_id: 0
        act_id: 18
        summon_id: 0
        summon_min: 0
        summon_max: 0
        poly_id: 0
`, changeTarget, skillID))
	if err := os.WriteFile(mobSkillPath, raw, 0o644); err != nil {
		t.Fatalf("寫入 mob skill 測試資料失敗: %v", err)
	}
	mobSkills, err := data.LoadMobSkillTable(mobSkillPath)
	if err != nil {
		t.Fatalf("載入 mob skill 測試資料失敗: %v", err)
	}

	s.deps.Skills = skills
	s.deps.MobSkills = mobSkills
}

func withNpcTypeTwoFixedDamageMobSkill(t *testing.T, s *NpcAISystem, skillID, leverage int) {
	t.Helper()
	withNpcTypeTwoFixedDamageRangedMobSkill(t, s, skillID, leverage, 4, -14)
}

func withNpcTypeTwoFixedDamageAreaMobSkill(t *testing.T, s *NpcAISystem, skillID, area int) {
	t.Helper()
	withNpcTypeTwoFixedDamageConfigMobSkill(t, s, skillID, 0, 4, -14, area)
}

func withNpcTypeTwoFixedDamageTargetMobSkill(t *testing.T, s *NpcAISystem, skillID int, target string, ranged, area int) {
	t.Helper()
	withNpcTypeTwoFixedDamageFullMobSkill(t, s, skillID, 0, ranged, -14, area, target)
}

func withNpcTypeTwoFixedDamageRangedMobSkill(t *testing.T, s *NpcAISystem, skillID, leverage, ranged, triggerRange int) {
	t.Helper()
	withNpcTypeTwoFixedDamageConfigMobSkill(t, s, skillID, leverage, ranged, triggerRange, 0)
}

func withNpcTypeTwoFixedDamageConfigMobSkill(t *testing.T, s *NpcAISystem, skillID, leverage, ranged, triggerRange, area int) {
	t.Helper()
	withNpcTypeTwoFixedDamageFullMobSkill(t, s, skillID, leverage, ranged, triggerRange, area, "attack")
}

func withNpcTypeTwoFixedDamageFullMobSkill(t *testing.T, s *NpcAISystem, skillID, leverage, ranged, triggerRange, area int, target string) {
	t.Helper()
	dir := t.TempDir()

	skillPath := filepath.Join(dir, "skill_list.yaml")
	skillRaw := []byte(fmt.Sprintf(`skills:
  - skill_id: %d
    name: fixed-damage-magic
    skill_level: 1
    skill_number: 1
    mp_consume: 0
    hp_consume: 0
    target: %s
    target_to: 0
    damage_value: 40
    damage_dice: 0
    damage_dice_count: 0
    attr: 0
    type: 32
    ranged: %d
    area: %d
    action_id: 18
    cast_gfx: 0
`, skillID, target, ranged, area))
	if err := os.WriteFile(skillPath, skillRaw, 0o644); err != nil {
		t.Fatalf("寫入技能測試資料失敗: %v", err)
	}
	skills, err := data.LoadSkillTable(skillPath)
	if err != nil {
		t.Fatalf("載入技能測試資料失敗: %v", err)
	}

	mobSkillPath := filepath.Join(dir, "mob_skill_list.yaml")
	raw := []byte(fmt.Sprintf(`mob_skills:
  - mob_id: 45197
    skills:
      - act_no: 0
        name: leveraged-type-two-magic
        type: 2
        mp_consume: 0
        trigger_random: 1
        trigger_hp: 0
        trigger_companion_hp: 0
        trigger_range: %d
        trigger_count: 0
        change_target: 0
        range: 0
        area_width: 0
        area_height: 0
        leverage: %d
        skill_id: %d
        skill_area: 0
        gfx_id: 0
        act_id: 18
        summon_id: 0
        summon_min: 0
        summon_max: 0
        poly_id: 0
`, triggerRange, leverage, skillID))
	if err := os.WriteFile(mobSkillPath, raw, 0o644); err != nil {
		t.Fatalf("寫入 mob skill 測試資料失敗: %v", err)
	}
	mobSkills, err := data.LoadMobSkillTable(mobSkillPath)
	if err != nil {
		t.Fatalf("載入 mob skill 測試資料失敗: %v", err)
	}

	s.deps.Skills = skills
	s.deps.MobSkills = mobSkills
}
func TestNpcMobSkillTypeTwoKirtasBarrierOneAppliesNpcSelfBarrierLikeJava(t *testing.T) {
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	npc := &world.NpcInfo{
		ID:          2001,
		NpcID:       81163,
		Impl:        "L1Monster",
		Name:        "kirtas",
		X:           100,
		Y:           100,
		MapID:       900,
		HP:          80,
		MaxHP:       100,
		MP:          100,
		MaxMP:       100,
		Level:       80,
		STR:         30,
		DEX:         30,
		AggroTarget: target.SessionID,
	}
	ws.AddNpc(npc)
	s := newNpcAILOSTestSystem(t, ws)
	withNpcTypeTwoMobSkill(t, s, 11060, 0)

	if !s.executeNpcSkill(npc, target, 11060, 0, 0, 0) {
		t.Fatal("yiwei KIRTAS_BARRIER1(11060) type 2 mobskill 應可施放")
	}

	if !npc.HasDebuff(11060) {
		t.Fatal("yiwei KIRTAS_BARRIER1(11060) skillmode 會對 NPC 自己 setSkillEffect(11060)")
	}
	if npc.HiddenStatus != world.NpcHiddenKirtas || npc.HiddenActionStatus != 20 {
		t.Fatalf("yiwei KIRTAS_BARRIER1(11060) 會 setHiddenStatus(4)/setStatus(20)，HiddenStatus=%d Action=%d", npc.HiddenStatus, npc.HiddenActionStatus)
	}
	if npc.MP != 95 {
		t.Fatalf("yiwei 11060 mp_consume=5，NPC MP=%d want=95", npc.MP)
	}
}

func TestNpcKirtasBarrierOneAppearsAfterBarrierTimerLikeJava(t *testing.T) {
	ws := world.NewState()
	trigger := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	npc := &world.NpcInfo{
		ID:                 2001,
		NpcID:              81163,
		Impl:               "L1Monster",
		Name:               "kirtas",
		X:                  100,
		Y:                  100,
		MapID:              900,
		HP:                 80,
		MaxHP:              100,
		HiddenStatus:       world.NpcHiddenKirtas,
		HiddenActionStatus: 20,
		KirtasBarrierTicks: 76,
	}
	npc.AddDebuff(11060, 1<<30)
	ws.AddNpc(npc)

	if !npcShouldAppearForPlayerLikeJava(npc, trigger) {
		t.Fatal("yiwei HIDDEN_STATUS_KIRTAS 在 barrierTime > 15 後，玩家接近視野更新時應顯形")
	}
	if !npcAppearOnGroundLikeJava(npc, ws, trigger) {
		t.Fatal("KIRTAS hidden 顯形應成功")
	}
	if npc.HiddenStatus != world.NpcHiddenNone || npc.HiddenActionStatus != 0 {
		t.Fatalf("KIRTAS 顯形後應清除 hidden/status，HiddenStatus=%d Action=%d", npc.HiddenStatus, npc.HiddenActionStatus)
	}
	if npc.HasDebuff(11060) {
		t.Fatal("KIRTAS_BARRIER1 顯形時 yiwei 會 killSkillEffectTimer(11060)")
	}
	if npc.KirtasBarrierTicks != 0 {
		t.Fatalf("KIRTAS 顯形後 barrierTime 應歸零，got=%d", npc.KirtasBarrierTicks)
	}
}
