package system

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/world"
)

func TestNpcMobSkillAreaPoisonSpawnsEffectAndPoisonsVisiblePlayersLikeJava(t *testing.T) {
	ws := world.NewState()
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "target",
		X:         100,
		Y:         100,
		MapID:     900,
		HP:        5000,
		MaxHP:     5000,
	})
	gmInvis := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:   2,
		Session:     newSkillTestSession(t, 2),
		CharID:      1002,
		Name:        "gm_invis",
		X:           101,
		Y:           100,
		MapID:       900,
		HP:          5000,
		MaxHP:       5000,
		AccessLevel: 200,
		GMInvisible: true,
		Invisible:   true,
	})
	npc := &world.NpcInfo{
		ID:            2001,
		NpcID:         45625,
		Impl:          "L1Monster",
		Name:          "area_poison",
		X:             101,
		Y:             100,
		MapID:         900,
		HP:            100,
		MaxHP:         100,
		MP:            0,
		MaxMP:         100,
		Level:         50,
		STR:           30,
		DEX:           30,
		AtkDmg:        20,
		SubMagicSpeed: 1000,
		Ranged:        1,
		AggroTarget:   target.SessionID,
	}
	ws.AddNpc(npc)
	s := newNpcAILOSTestSystem(t, ws)
	withNpcAreaPoisonMobSkill(t, s, 12000)

	s.tickMonsterAI(npc)

	effects := ws.GroundEffectList()
	if len(effects) != 1 {
		t.Fatalf("yiwei area_poison 只應在非 GM 隱身玩家腳下 spawnEffect，got=%d", len(effects))
	}
	effect := effects[0]
	if effect.NpcID != 86125 || effect.GfxID != 1263 || effect.Type != world.GroundEffectType(11) {
		t.Fatalf("area_poison 應生成毒霧 L1Effect(86125/gfx 1263/type 11)，got npc=%d gfx=%d type=%d", effect.NpcID, effect.GfxID, effect.Type)
	}
	if effect.X != target.X || effect.Y != target.Y || effect.MapID != target.MapID {
		t.Fatalf("Java spawnEffect 第 3-4 個參數使用 pc.getX()/pc.getY()，got=(%d,%d,%d) want=(%d,%d,%d)", effect.X, effect.Y, effect.MapID, target.X, target.Y, target.MapID)
	}
	if effect.TicksLeft != 25 {
		t.Fatalf("mobskill leverage=5 應換算為 5 秒、25 tick，got=%d", effect.TicksLeft)
	}
	if gmInvis.PoisonType != 0 {
		t.Fatalf("GM 隱身玩家不應被毒霧套毒，PoisonType=%d", gmInvis.PoisonType)
	}
	if npc.AttackTimer != 5 {
		t.Fatalf("yiwei area_poison 施放後應吃 sub_magic_speed 冷卻，got=%d want=5", npc.AttackTimer)
	}
	if got := npc.MobSkillUseCounts[5]; got != 1 {
		t.Fatalf("yiwei area_poison 成功後應消耗 TriCount，got=%d want=1", got)
	}

	ground := NewGroundEffectSystem(ws, s.deps)
	for i := 0; i < 5; i++ {
		ground.Update(0)
	}
	if target.PoisonType != 1 || target.PoisonDmgAmount != 100 {
		t.Fatalf("Java 86125 PoisonTimer 應對毒霧同格玩家套 100 傷害毒，PoisonType=%d PoisonDmgAmount=%d", target.PoisonType, target.PoisonDmgAmount)
	}
	if gmInvis.PoisonType != 0 {
		t.Fatalf("GM 隱身玩家不應被毒霧 tick 套毒，PoisonType=%d", gmInvis.PoisonType)
	}
}

func withNpcAreaPoisonMobSkill(t *testing.T, s *NpcAISystem, reuseDelay int) {
	t.Helper()
	dir := t.TempDir()
	npcPath := filepath.Join(dir, "npc_list.yaml")
	if err := os.WriteFile(npcPath, []byte(`npcs:
  - npc_id: 86125
    name: poison-cloud
    nameid: ''
    impl: L1Effect
    gfx_id: 1263
    level: 0
    hp: 0
    mp: 0
    ac: 0
    str: 0
    dex: 0
    con: 0
    wis: 0
    intel: 0
    mr: 0
    exp: 0
    lawful: 0
    size: small
    ranged: 0
    atk_speed: 0
    passive_speed: 0
    undead: false
    agro: false
    tameable: false
`), 0o644); err != nil {
		t.Fatalf("寫入 NPC 測試資料失敗: %v", err)
	}
	npcs, err := data.LoadNpcTable(npcPath)
	if err != nil {
		t.Fatalf("載入 NPC 測試資料失敗: %v", err)
	}

	mobSkillPath := filepath.Join(dir, "mob_skill_list.yaml")
	raw := []byte(fmt.Sprintf(`mob_skills:
  - mob_id: 45625
    skills:
      - act_no: 5
        name: area-poison
        type: 15
        mp_consume: 25
        trigger_random: 1
        trigger_hp: 0
        trigger_companion_hp: 0
        trigger_range: -14
        trigger_count: 1
        change_target: 0
        range: 0
        area_width: 0
        area_height: 0
        leverage: 5
        skill_id: 0
        skill_area: 0
        gfx_id: 0
        act_id: 19
        reuse_delay: %d
        summon_id: 86125
        summon_min: 0
        summon_max: 0
        poly_id: 0
`, reuseDelay))
	if err := os.WriteFile(mobSkillPath, raw, 0o644); err != nil {
		t.Fatalf("寫入 mobskill 測試資料失敗: %v", err)
	}
	mobSkills, err := data.LoadMobSkillTable(mobSkillPath)
	if err != nil {
		t.Fatalf("載入 mobskill 測試資料失敗: %v", err)
	}

	s.deps.Npcs = npcs
	s.deps.MobSkills = mobSkills
}
