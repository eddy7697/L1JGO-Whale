package system

import (
	"encoding/binary"
	stdnet "net"
	"os"
	"path/filepath"
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/scripting"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

func newSkillTestSession(t *testing.T, id uint64) *l1net.Session {
	t.Helper()
	client, server := stdnet.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
	})
	sess := l1net.NewSession(server, id, 8, 8, 0, zap.NewNop())
	t.Cleanup(sess.Close)
	return sess
}

func newSkillTestSystem(t *testing.T, ws *world.State) *SkillSystem {
	t.Helper()
	engine, err := scripting.NewEngine("../../scripts", zap.NewNop())
	if err != nil {
		t.Fatalf("建立 Lua engine 失敗: %v", err)
	}
	return &SkillSystem{deps: &handler.Deps{
		World:     ws,
		Scripting: engine,
		Log:       zap.NewNop(),
	}}
}

func newSkillLOSTestMap(t *testing.T) *data.MapDataTable {
	t.Helper()
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "map_list.yaml")
	tileDir := filepath.Join(dir, "tiles")
	if err := os.Mkdir(tileDir, 0o755); err != nil {
		t.Fatalf("建立測試地圖目錄失敗: %v", err)
	}
	yaml := []byte(`maps:
  - map_id: 900
    name: los_test
    start_x: 100
    end_x: 105
    start_y: 100
    end_y: 100
`)
	if err := os.WriteFile(yamlPath, yaml, 0o644); err != nil {
		t.Fatalf("寫入測試地圖清單失敗: %v", err)
	}
	// 15 = 可走且箭矢可通過，3 = 可走但箭矢不可通過；102,100 是牆。
	if err := os.WriteFile(filepath.Join(tileDir, "900.txt"), []byte("15,15,3,15,15,15\n"), 0o644); err != nil {
		t.Fatalf("寫入測試地圖格資料失敗: %v", err)
	}
	maps, err := data.LoadMapData(yamlPath, tileDir)
	if err != nil {
		t.Fatalf("載入測試地圖失敗: %v", err)
	}
	if maps.Count() != 1 {
		t.Fatalf("測試地圖未載入")
	}
	return maps
}

func addSkillTestPlayer(ws *world.State, p *world.PlayerInfo) *world.PlayerInfo {
	if p.Level == 0 {
		p.Level = 50
	}
	if p.HP == 0 {
		p.HP = 100
	}
	if p.MaxHP == 0 {
		p.MaxHP = 100
	}
	if p.Intel == 0 {
		p.Intel = 18
	}
	if p.Inv == nil {
		p.Inv = world.NewInventory()
	}
	ws.AddPlayer(p)
	return p
}

func drainSkillTestPackets(sess *l1net.Session) [][]byte {
	sess.FlushOutput()
	var packets [][]byte
	for {
		select {
		case pkt := <-sess.OutQueue:
			packets = append(packets, pkt)
		default:
			return packets
		}
	}
}

func hasOpcodePacket(packets [][]byte, opcode byte) bool {
	for _, pkt := range packets {
		if len(pkt) > 0 && pkt[0] == opcode {
			return true
		}
	}
	return false
}

func hasSkillEffectPacket(packets [][]byte, objectID int32, gfxID int32) bool {
	for _, pkt := range packets {
		if len(pkt) < 7 || pkt[0] != packet.S_OPCODE_EFFECT {
			continue
		}
		if int32(binary.LittleEndian.Uint32(pkt[1:5])) == objectID &&
			int32(binary.LittleEndian.Uint16(pkt[5:7])) == gfxID {
			return true
		}
	}
	return false
}

func TestDeathTombttackSkillDamagesPlayerTarget(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "caster",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         103,
		Y:         100,
		MapID:     4,
		HP:        100,
		MaxHP:     100,
	})
	s := newSkillTestSystem(t, ws)
	skill := &data.SkillInfo{
		SkillID:         4,
		SkillLevel:      1,
		Target:          "attack",
		Type:            64,
		DamageValue:     20,
		DamageDice:      1,
		DamageDiceCount: 1,
		Ranged:          10,
		ActionID:        18,
		CastGfx:         167,
	}

	s.executeAttackSkill(caster.Session, caster, skill, target.CharID)

	if target.HP >= 100 {
		t.Fatalf("攻擊技能應傷害玩家目標，HP=%d", target.HP)
	}
	if !target.Dirty {
		t.Fatal("玩家目標受傷後應標記 Dirty")
	}
}

func TestSkillDamageHealChillTouchLeechesFromPlayerTarget(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "caster",
		X:         100,
		Y:         100,
		MapID:     4,
		HP:        50,
		MaxHP:     100,
		Intel:     12,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         101,
		Y:         100,
		MapID:     4,
		HP:        100,
		MaxHP:     100,
	})
	s := newSkillTestSystem(t, ws)
	skill := &data.SkillInfo{
		SkillID:     10,
		Target:      "attack",
		Type:        64,
		DamageValue: 20,
		Ranged:      3,
		ActionID:    18,
		CastGfx:     252,
	}

	s.executeAttackSkill(caster.Session, caster, skill, target.CharID)

	lostHP := int32(100) - target.HP
	if lostHP <= 0 {
		t.Fatalf("寒冷戰慄應先對玩家目標造成傷害，targetHP=%d", target.HP)
	}
	if caster.HP != 50+lostHP {
		t.Fatalf("寒冷戰慄應依傷害回復施法者，lostHP=%d casterHP=%d", lostHP, caster.HP)
	}
}

// tripleArrowPvPSpy 記錄 HandlePvPFarAttack 被呼叫的次數，
// 用來驗證三重矢（skill 132）走完整 PvP 弓箭路徑 × 3 而非舊的單次傷害複製 3 次的退化路徑。
type tripleArrowPvPSpy struct {
	farCalled int
	attacker  *world.PlayerInfo
	target    *world.PlayerInfo
}

func (s *tripleArrowPvPSpy) HandlePvPAttack(_, _ *world.PlayerInfo) {}
func (s *tripleArrowPvPSpy) HandlePvPFarAttack(attacker, target *world.PlayerInfo) {
	s.farCalled++
	s.attacker = attacker
	s.target = target
}
func (s *tripleArrowPvPSpy) AddLawfulFromNpc(_ *world.PlayerInfo, _ int32) {}
func (s *tripleArrowPvPSpy) BeginHell(_ *world.PlayerInfo, _ bool)         {}
func (s *tripleArrowPvPSpy) TriggerPinkName(_, _ *world.PlayerInfo)        {}

// TestSkillTripleArrowRoutesThroughPvPFarAttackThreeTimes 驗證三重矢（132）PvP 路徑
// 對齊 Java `TRIPLE_ARROW.start()` 第 39-41 行 `for (int i = 0; i < 3; i++) cha.onAction(srcpc)`：
// 走 3 次完整 PvP 弓箭流程（HandlePvPFarAttack）。
func TestSkillTripleArrowRoutesThroughPvPFarAttackThreeTimes(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID:     1,
		Session:       newSkillTestSession(t, 1),
		CharID:        1001,
		Name:          "caster",
		X:             100,
		Y:             100,
		MapID:         4,
		Level:         90,
		Str:           35,
		Dex:           35,
		CurrentWeapon: 20,
	})
	target := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "target",
		X:         103,
		Y:         100,
		MapID:     4,
		HP:        1000,
		MaxHP:     1000,
		AC:        100,
	})
	s := newSkillTestSystem(t, ws)
	pvp := &tripleArrowPvPSpy{}
	s.deps.PvP = pvp
	skill := &data.SkillInfo{
		SkillID:  132,
		Target:   "attack",
		Type:     64,
		Ranged:   10,
		ActionID: 18,
	}

	s.executeAttackSkill(caster.Session, caster, skill, target.CharID)

	if pvp.farCalled != 3 {
		t.Fatalf("三重矢 PvP 應呼叫 HandlePvPFarAttack 3 次，got=%d", pvp.farCalled)
	}
	if pvp.attacker != caster || pvp.target != target {
		t.Fatalf("HandlePvPFarAttack 接到的 (attacker, target) 不符 expected=(%p,%p) got=(%p,%p)",
			caster, target, pvp.attacker, pvp.target)
	}
	pkts := drainSkillTestPackets(caster.Session)
	if !containsSkillEffect(pkts, caster.CharID, 4394) || !containsSkillEffect(pkts, caster.CharID, 11764) {
		t.Fatalf("三重矢收尾應廣播 S_SkillSound 4394 + 11764（Java TRIPLE_ARROW.start() 第 45-46 行）")
	}
}

// containsSkillEffect 檢查封包流是否包含指定 objectID/gfxID 的 S_SkillSound（opcode S_OPCODE_EFFECT）。
// 格式：[opcode 1B][objectID 4B LE][gfxID 2B LE]。
func containsSkillEffect(packets [][]byte, objectID, gfxID int32) bool {
	for _, pkt := range packets {
		if len(pkt) < 7 {
			continue
		}
		if pkt[0] != packet.S_OPCODE_EFFECT {
			continue
		}
		gotObj := int32(binary.LittleEndian.Uint32(pkt[1:5]))
		gotGfx := int32(binary.LittleEndian.Uint16(pkt[5:7]))
		if gotObj == objectID && gotGfx == gfxID {
			return true
		}
	}
	return false
}

func TestSkillDamageHealSelfAreaAttackDamagesNearbyPlayersAndNpcs(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "caster",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	nearPlayer := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "near",
		X:         102,
		Y:         100,
		MapID:     4,
		HP:        100,
		MaxHP:     100,
	})
	farPlayer := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    1003,
		Name:      "far",
		X:         120,
		Y:         100,
		MapID:     4,
		HP:        100,
		MaxHP:     100,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		Name:  "npc",
		X:     101,
		Y:     100,
		MapID: 4,
		HP:    100,
		MaxHP: 100,
		MR:    0,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	skill := &data.SkillInfo{
		SkillID:         53,
		SkillLevel:      7,
		Target:          "none",
		Type:            64,
		DamageValue:     20,
		DamageDice:      1,
		DamageDiceCount: 1,
		Ranged:          0,
		Area:            4,
		ActionID:        18,
		CastGfx:         758,
	}

	s.executeSelfSkill(caster.Session, caster, skill)

	if nearPlayer.HP >= 100 {
		t.Fatalf("範圍攻擊應傷害附近玩家，HP=%d", nearPlayer.HP)
	}
	if farPlayer.HP != 100 {
		t.Fatalf("範圍外玩家不應受傷，HP=%d", farPlayer.HP)
	}
	if caster.HP != 100 {
		t.Fatalf("攻擊型範圍技能不應傷害施法者自己，HP=%d", caster.HP)
	}
	if npc.HP >= 100 {
		t.Fatalf("範圍攻擊仍應傷害附近 NPC，HP=%d", npc.HP)
	}
}

func TestSelfAreaAttackSkillUsesRangeSkillVisualInsteadOfNpcEffect(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "caster",
		X:         100,
		Y:         100,
		MapID:     4,
	})
	npc := &world.NpcInfo{
		ID:    2001,
		Name:  "npc",
		X:     101,
		Y:     100,
		MapID: 4,
		HP:    100,
		MaxHP: 100,
		MR:    0,
	}
	ws.AddNpc(npc)
	s := newSkillTestSystem(t, ws)
	skill := &data.SkillInfo{
		SkillID:         53,
		SkillLevel:      7,
		Target:          "none",
		Type:            64,
		DamageValue:     20,
		DamageDice:      1,
		DamageDiceCount: 1,
		Area:            4,
		ActionID:        18,
		CastGfx:         758,
	}

	s.executeSelfSkill(caster.Session, caster, skill)
	packets := drainSkillTestPackets(caster.Session)

	if !hasOpcodePacket(packets, packet.S_OPCODE_RANGESKILLS) {
		t.Fatalf("自體範圍攻擊技能應送出 S_RangeSkill opcode=%d", packet.S_OPCODE_RANGESKILLS)
	}
	if hasSkillEffectPacket(packets, npc.ID, skill.CastGfx) {
		t.Fatalf("自體範圍攻擊技能不應在被打到的 NPC 身上播放 S_SkillSoundGFX")
	}
}

func TestTargetAreaAttackSkillSkipsNpcBehindWall(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "caster",
		X:         100,
		Y:         100,
		MapID:     900,
	})
	front := &world.NpcInfo{
		ID:    2001,
		Name:  "front",
		X:     101,
		Y:     100,
		MapID: 900,
		HP:    100,
		MaxHP: 100,
		MR:    0,
	}
	behindWall := &world.NpcInfo{
		ID:    2002,
		Name:  "behind_wall",
		X:     104,
		Y:     100,
		MapID: 900,
		HP:    100,
		MaxHP: 100,
		MR:    0,
	}
	ws.AddNpc(front)
	ws.AddNpc(behindWall)
	s := newSkillTestSystem(t, ws)
	s.deps.MapData = newSkillLOSTestMap(t)
	skill := &data.SkillInfo{
		SkillID:         53,
		SkillLevel:      7,
		Target:          "attack",
		Type:            64,
		DamageValue:     20,
		DamageDice:      1,
		DamageDiceCount: 1,
		Ranged:          10,
		Area:            4,
		ActionID:        18,
		CastGfx:         758,
	}

	s.executeAttackSkill(caster.Session, caster, skill, front.ID)

	if front.HP >= 100 {
		t.Fatalf("牆前主目標應該受到範圍攻擊傷害，HP=%d", front.HP)
	}
	if behindWall.HP != 100 {
		t.Fatalf("牆後範圍候選不應該被打到，HP=%d", behindWall.HP)
	}
}

func TestTargetAreaAttackSkillOnPlayerSkipsTargetsBehindWall(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "caster",
		X:         100,
		Y:         100,
		MapID:     900,
	})
	frontPlayer := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "front",
		X:         101,
		Y:         100,
		MapID:     900,
		HP:        100,
		MaxHP:     100,
	})
	behindWallPlayer := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    1003,
		Name:      "behind_wall_player",
		X:         104,
		Y:         100,
		MapID:     900,
		HP:        100,
		MaxHP:     100,
	})
	behindWallNpc := &world.NpcInfo{
		ID:    2001,
		Name:  "behind_wall_npc",
		X:     104,
		Y:     100,
		MapID: 900,
		HP:    100,
		MaxHP: 100,
		MR:    0,
	}
	ws.AddNpc(behindWallNpc)
	s := newSkillTestSystem(t, ws)
	s.deps.MapData = newSkillLOSTestMap(t)
	skill := &data.SkillInfo{
		SkillID:         53,
		SkillLevel:      7,
		Target:          "attack",
		Type:            64,
		DamageValue:     20,
		DamageDice:      1,
		DamageDiceCount: 1,
		Ranged:          10,
		Area:            4,
		ActionID:        18,
		CastGfx:         758,
	}

	s.executeAttackSkill(caster.Session, caster, skill, frontPlayer.CharID)

	if frontPlayer.HP >= 100 {
		t.Fatalf("牆前主玩家應該受到範圍攻擊傷害，HP=%d", frontPlayer.HP)
	}
	if behindWallPlayer.HP != 100 {
		t.Fatalf("牆後範圍玩家不應該被打到，HP=%d", behindWallPlayer.HP)
	}
	if behindWallNpc.HP != 100 {
		t.Fatalf("牆後範圍 NPC 不應該被打到，HP=%d", behindWallNpc.HP)
	}
}

func TestSelfAreaAttackSkillSkipsTargetsBehindWall(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "caster",
		X:         100,
		Y:         100,
		MapID:     900,
	})
	frontPlayer := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "front",
		X:         101,
		Y:         100,
		MapID:     900,
		HP:        100,
		MaxHP:     100,
	})
	behindWallPlayer := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    1003,
		Name:      "behind_wall_player",
		X:         104,
		Y:         100,
		MapID:     900,
		HP:        100,
		MaxHP:     100,
	})
	behindWallNpc := &world.NpcInfo{
		ID:    2001,
		Name:  "behind_wall_npc",
		X:     104,
		Y:     100,
		MapID: 900,
		HP:    100,
		MaxHP: 100,
		MR:    0,
	}
	ws.AddNpc(behindWallNpc)
	s := newSkillTestSystem(t, ws)
	s.deps.MapData = newSkillLOSTestMap(t)
	skill := &data.SkillInfo{
		SkillID:         53,
		SkillLevel:      7,
		Target:          "none",
		Type:            64,
		DamageValue:     20,
		DamageDice:      1,
		DamageDiceCount: 1,
		Area:            4,
		ActionID:        18,
		CastGfx:         758,
	}

	s.executeSelfSkill(caster.Session, caster, skill)

	if frontPlayer.HP >= 100 {
		t.Fatalf("牆前範圍玩家應該受到傷害，HP=%d", frontPlayer.HP)
	}
	if behindWallPlayer.HP != 100 {
		t.Fatalf("牆後範圍玩家不應該被打到，HP=%d", behindWallPlayer.HP)
	}
	if behindWallNpc.HP != 100 {
		t.Fatalf("牆後範圍 NPC 不應該被打到，HP=%d", behindWallNpc.HP)
	}
}

func TestSkillDamageHealSingleHealCapsAtMaxHP(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "caster",
		X:         100,
		Y:         100,
		MapID:     4,
		HP:        95,
		MaxHP:     100,
	})
	s := newSkillTestSystem(t, ws)
	skill := &data.SkillInfo{
		SkillID:         57,
		SkillLevel:      8,
		Target:          "buff",
		Type:            16,
		DamageValue:     100,
		DamageDice:      1,
		DamageDiceCount: 1,
		Ranged:          -1,
		ActionID:        19,
		CastGfx:         832,
	}

	s.executeBuffSkill(caster.Session, caster, skill, caster.CharID)

	if caster.HP != caster.MaxHP {
		t.Fatalf("治癒不可超過 MaxHP，HP=%d MaxHP=%d", caster.HP, caster.MaxHP)
	}
}

func TestSkillDamageHealHealAllHealsNearbyPlayers(t *testing.T) {
	ws := world.NewState()
	caster := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "caster",
		X:         100,
		Y:         100,
		MapID:     4,
		HP:        50,
		MaxHP:     100,
	})
	nearPlayer := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 2,
		Session:   newSkillTestSession(t, 2),
		CharID:    1002,
		Name:      "near",
		X:         103,
		Y:         100,
		MapID:     4,
		HP:        50,
		MaxHP:     100,
	})
	farPlayer := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 3,
		Session:   newSkillTestSession(t, 3),
		CharID:    1003,
		Name:      "far",
		X:         130,
		Y:         100,
		MapID:     4,
		HP:        50,
		MaxHP:     100,
	})
	s := newSkillTestSystem(t, ws)
	skill := &data.SkillInfo{
		SkillID:         49,
		SkillLevel:      7,
		Target:          "none",
		Type:            16,
		DamageValue:     20,
		DamageDice:      1,
		DamageDiceCount: 1,
		Area:            -1,
		ActionID:        19,
		CastGfx:         759,
	}

	s.executeSelfSkill(caster.Session, caster, skill)

	if caster.HP <= 50 {
		t.Fatalf("全部治癒術應治癒施法者，HP=%d", caster.HP)
	}
	if nearPlayer.HP <= 50 {
		t.Fatalf("全部治癒術應治癒附近玩家，HP=%d", nearPlayer.HP)
	}
	if farPlayer.HP != 50 {
		t.Fatalf("全部治癒術不應治癒範圍外玩家，HP=%d", farPlayer.HP)
	}
}
