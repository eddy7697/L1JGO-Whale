package system

import (
	"encoding/binary"
	"math/rand"
	"testing"
	"time"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

func newJavaPotionTest(t *testing.T) (*SkillSystem, *ItemUseSystem, *world.PlayerInfo) {
	t.Helper()
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, ClassType: 1, X: 100, Y: 100, MapID: 4})
	s := newSkillBuffTestSystem(t, ws)
	s.deps.Skill = s
	return s, NewItemUseSystem(s.deps), p
}

func hasJavaSpeedState(packets [][]byte, opcode byte, charID int32, speed byte) bool {
	for _, p := range packets {
		if len(p) >= 8 && p[0] == opcode && int32(binary.LittleEndian.Uint32(p[1:5])) == charID && p[5] == speed && binary.LittleEndian.Uint16(p[6:8]) == 0 {
			return true
		}
	}
	return false
}

func TestJavaPotionsExpireThroughWorldTick(t *testing.T) {
	s, items, p := newJavaPotionTest(t)
	for _, id := range []int32{40013, 40014} {
		item := p.Inv.AddItem(id, 1, "potion", 0, 0, true, 1)
		if !items.UseConsumable(p.Session, p, item, &data.ItemInfo{ItemID: id}) {
			t.Fatalf("potion %d not consumed", id)
		}
		drainSkillTestPackets(p.Session)
	}
	ticker := NewBuffTickSystem(s.deps.World, s.deps)
	for i := 0; i < 1499; i++ {
		ticker.Update(200 * time.Millisecond)
	}
	if p.MoveSpeed != 1 || p.BraveSpeed != 1 {
		t.Fatal("potions ended before 300 seconds")
	}
	ticker.Update(200 * time.Millisecond)
	if p.MoveSpeed != 0 || p.BraveSpeed != 0 || p.HasBuff(1000) || p.HasBuff(1001) {
		t.Fatalf("expired speed remains: move=%d brave=%d buffs=%v", p.MoveSpeed, p.BraveSpeed, p.ActiveBuffs)
	}
	packets := drainSkillTestPackets(p.Session)
	if !hasJavaSpeedState(packets, packet.S_OPCODE_SPEED, p.CharID, 0) || !hasJavaSpeedState(packets, packet.S_OPCODE_SKILLBRAVE, p.CharID, 0) {
		t.Fatalf("missing Java speed cancellation packets: %v", packets)
	}
}

func TestJavaPotionsExpireAfterDelayedWorldTick(t *testing.T) {
	s, items, p := newJavaPotionTest(t)
	items.ApplyHaste(p.Session, p, 300, 191)
	drainSkillTestPackets(p.Session)
	items.applyBrave(p.Session, p, 300, 1, 751)
	drainSkillTestPackets(p.Session)
	deadline := p.GetBuff(1000).ExpiresAt.Add(time.Second)
	s.tickPlayerBuffsAt(p, deadline)
	if p.MoveSpeed != 0 || p.BraveSpeed != 0 || p.HasBuff(1000) || p.HasBuff(1001) {
		t.Fatal("wall-clock expiry must not wait for 1500 delivered ticks")
	}
	packets := drainSkillTestPackets(p.Session)
	if !hasJavaSpeedState(packets, packet.S_OPCODE_SPEED, p.CharID, 0) || !hasJavaSpeedState(packets, packet.S_OPCODE_SKILLBRAVE, p.CharID, 0) {
		t.Fatal("delayed expiry did not cancel client speed")
	}
}

func TestJavaSpeedExplicitRemovalAndHasteEquipment(t *testing.T) {
	s, items, p := newJavaPotionTest(t)
	items.ApplyHaste(p.Session, p, 300, 191)
	drainSkillTestPackets(p.Session)
	s.removeBuffAndRevert(p, 1001)
	if p.MoveSpeed != 0 || !hasJavaSpeedState(drainSkillTestPackets(p.Session), packet.S_OPCODE_SPEED, p.CharID, 0) {
		t.Fatal("explicit removal left haste active")
	}
	p.HasteItemEquipped, p.MoveSpeed = 1, 1
	items.ApplyHaste(p.Session, p, 300, 191)
	if p.HasBuff(1001) || p.MoveSpeed != 1 {
		t.Fatal("haste equipment must not acquire a potion timer")
	}
	item := p.Inv.AddItem(40013, 1, "potion", 0, 0, true, 1)
	if items.UseConsumable(p.Session, p, item, &data.ItemInfo{ItemID: 40013}) || p.Inv.FindByObjectID(item.ObjectID) == nil {
		t.Fatal("equipment haste must not consume a redundant green potion")
	}
}

func TestJavaNpcCurseTransitionsAndCures(t *testing.T) {
	s, _, p := newJavaPotionTest(t)
	s.ApplyNpcDebuff(p, s.deps.Skills.Get(33))
	if p.CurseType != 1 || p.CurseTicksLeft != 40 || p.Paralyzed {
		t.Fatalf("missing 8-second curse delay: type=%d ticks=%d", p.CurseType, p.CurseTicksLeft)
	}
	drainSkillTestPackets(p.Session)
	for i := 0; i < 40; i++ {
		TickPlayerCurse(p, s.deps)
	}
	if !p.Paralyzed || p.CurseTicksLeft != 80 {
		t.Fatal("curse must paralyze for 16 seconds")
	}
	drainSkillTestPackets(p.Session)
	for i := 0; i < 80; i++ {
		TickPlayerCurse(p, s.deps)
	}
	if p.Paralyzed || p.CurseType != 0 {
		t.Fatal("curse did not expire")
	}
}

func TestJavaNpcPoisonSpellActuallyDamages(t *testing.T) {
	s, _, p := newJavaPotionTest(t)
	s.ApplyNpcDebuff(p, s.deps.Skills.Get(11))
	if p.PoisonType != 1 {
		t.Fatal("poison spell only registered an icon")
	}
	drainSkillTestPackets(p.Session)
	hp := p.HP
	for i := 0; i < 15; i++ {
		TickPlayerPoison(p, s.deps)
	}
	if p.HP != hp-5 {
		t.Fatalf("poison HP=%d want=%d", p.HP, hp-5)
	}
}

func TestJavaMonsterCurseThroughNpcSkillDispatch(t *testing.T) {
	s, _, p := newJavaPotionTest(t)
	t.Setenv("GODEBUG", "randseednop=0")
	rand.Seed(7)
	npc := &world.NpcInfo{ID: 2001, Level: 80, Intel: 40, HP: 100, MP: 100, X: 101, Y: 100, MapID: 4}
	ai := &NpcAISystem{world: s.deps.World, deps: s.deps}
	// Java caps success at 90%; repeated seeded casts verify the real dispatch
	// without pretending every cast succeeds.
	for i := 0; i < 10 && p.CurseType == 0; i++ {
		ai.executeNpcSkill(npc, p, 33, 0, 0, 10)
		drainSkillTestPackets(p.Session)
	}
	if p.CurseType != 1 || p.CurseTicksLeft != curseDelayTicks {
		t.Fatal("NPC skill dispatch failed to start curse paralysis")
	}
}

func TestJavaNpcBlindChangesClientVision(t *testing.T) {
	for _, id := range []int32{20, 40} {
		s, _, p := newJavaPotionTest(t)
		s.ApplyNpcDebuff(p, s.deps.Skills.Get(id))
		if typ, ok := findCurseBlindPacketType(drainSkillTestPackets(p.Session)); !ok || typ != 1 {
			t.Fatalf("skill %d did not blind client vision", id)
		}
		s.tickPlayerBuffsAt(p, p.GetBuff(skillCurseBlindEffect).ExpiresAt.Add(time.Second))
		if typ, ok := findCurseBlindPacketType(drainSkillTestPackets(p.Session)); !ok || typ != 0 {
			t.Fatalf("skill %d did not restore client vision", id)
		}
	}
}

func TestJavaBlessedPotionDurations(t *testing.T) {
	s, _, _ := newJavaPotionTest(t)
	for id, want := range map[int]int{140015: 1200, 140016: 600} {
		if got := s.deps.Scripting.GetPotionEffect(id).Duration; got != want {
			t.Fatalf("potion %d duration=%d want=%d", id, got, want)
		}
	}
}

func TestJavaAntidoteClearsRealPoison(t *testing.T) {
	for _, typ := range []byte{1, 2, 3} {
		_, items, p := newJavaPotionTest(t)
		p.PoisonType, p.PoisonTicksLeft = typ, 100
		p.Silenced = typ == 2
		item := p.Inv.AddItem(40017, 1, "antidote", 0, 0, true, 1)
		if !items.UseConsumable(p.Session, p, item, &data.ItemInfo{ItemID: 40017}) {
			t.Fatal("antidote not consumed")
		}
		if p.PoisonType != 0 || p.Silenced || p.Paralyzed {
			t.Fatalf("poison %d was not cured", typ)
		}
	}
}

func TestJavaGreenPotionOnlyCancelsSlow(t *testing.T) {
	s, items, p := newJavaPotionTest(t)
	s.applyBuffEffect(p, &data.SkillInfo{SkillID: 29, BuffDuration: 30})
	drainSkillTestPackets(p.Session)
	items.ApplyHaste(p.Session, p, 300, 191)
	if p.MoveSpeed != 0 || p.HasBuff(29) || p.HasBuff(1001) {
		t.Fatalf("green potion should only neutralize slow: speed=%d buffs=%v", p.MoveSpeed, p.ActiveBuffs)
	}
}

func TestJavaBravePotionPreservesStrengthAndRemovesBloodlust(t *testing.T) {
	s, items, p := newJavaPotionTest(t)
	s.applyBuffEffect(p, &data.SkillInfo{SkillID: 42, BuffDuration: 1200})
	drainSkillTestPackets(p.Session)
	s.applyBuffEffect(p, &data.SkillInfo{SkillID: 186, BuffDuration: 600})
	drainSkillTestPackets(p.Session)
	strength := p.Str
	items.applyBrave(p.Session, p, 300, 1, 751)
	if !p.HasBuff(42) || p.Str != strength || p.HasBuff(186) {
		t.Fatalf("wrong brave conflicts: STR=%d want=%d buffs=%v", p.Str, strength, p.ActiveBuffs)
	}
}
