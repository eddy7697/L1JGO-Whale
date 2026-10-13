package system

import (
	"encoding/binary"
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

func TestPlayerAttackAnimationUsesAttackerVisibilityAndRepliesOnce(t *testing.T) {
	items, err := data.LoadItemTable("../../data/yaml/weapon_list.yaml", "../../data/yaml/armor_list.yaml", "../../data/yaml/etcitem_list.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, ranged := range []bool{false, true} {
		t.Run(map[bool]string{false: "melee", true: "ranged"}[ranged], func(t *testing.T) {
			s, p := newClientAttackTestSystem(t)
			s.deps.Scripting = newSkillTestSystem(t, s.deps.World).deps.Scripting
			s.deps.Items = items
			p.Level, p.Str, p.Dex, p.HitMod, p.BowHitMod = 80, 35, 35, 100, 100
			p.DmgMod, p.BowDmgMod = 100, 100
			opcode := packet.C_OPCODE_ATTACK
			if ranged {
				opcode = packet.C_OPCODE_FAR_ATTACK
				p.Equip.Set(world.SlotWeapon, p.Inv.AddItem(167, 1, "bow", 0, 0, false, 1))
				p.Inv.AddItem(40741, 20, "arrow", 0, 0, true, 1)
			}
			npc := &world.NpcInfo{ID: 2001, Impl: "L1Monster", X: 103, Y: 100, MapID: p.MapID, ShowID: p.ShowID, HP: 100000, MaxHP: 100000, Level: 1, AC: 10}
			s.deps.World.AddNpc(npc)
			// The observer sees the attacker at exactly 20 tiles but is 22 tiles from the target.
			observer := addSkillTestPlayer(s.deps.World, &world.PlayerInfo{SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, X: 81, Y: 100, MapID: p.MapID, ShowID: p.ShowID})
			other := addSkillTestPlayer(s.deps.World, &world.PlayerInfo{SessionID: 3, Session: newSkillTestSession(t, 3), CharID: 1003, X: 101, Y: 100, MapID: p.MapID, ShowID: p.ShowID + 1})
			for i := 0; i < 8; i++ {
				before := npc.HP
				sendClientAttackForTest(s, p, opcode, npc.ID, npc.X, npc.Y)
				s.Update(0)
				for _, recipient := range []*world.PlayerInfo{p, observer} {
					packets := clientAttackPackets(drainSkillTestPackets(recipient.Session))
					if len(packets) != 1 {
						t.Fatalf("recipient=%d attacks=%d (target HP %d -> %d)", recipient.CharID, len(packets), before, npc.HP)
					}
					atk := packets[0]
					if atk[1] != 1 || binary.LittleEndian.Uint32(atk[2:6]) != uint32(p.CharID) || binary.LittleEndian.Uint32(atk[6:10]) != uint32(npc.ID) {
						t.Fatalf("invalid action packet: %x", atk)
					}
					if got := int32(binary.LittleEndian.Uint16(atk[10:12])); got != before-npc.HP {
						t.Fatalf("animation damage=%d HP loss=%d", got, before-npc.HP)
					}
				}
				if got := drainSkillTestPackets(other.Session); len(got) != 0 {
					t.Fatalf("cross-instance packets %x", got)
				}
			}
			if npc.HP >= npc.MaxHP {
				t.Fatal("test did not exercise damaging attacks")
			}
			s.Update(0)
			if got := clientAttackPackets(drainSkillTestPackets(p.Session)); len(got) != 0 {
				t.Fatal("unexpected attack without client request")
			}
		})
	}
}

func TestGMInvisibleAttackDoesNotExposeActorToObservers(t *testing.T) {
	s, p := newClientAttackTestSystem(t)
	p.GMInvisible = true
	observer := addSkillTestPlayer(s.deps.World, &world.PlayerInfo{SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, X: p.X, Y: p.Y, MapID: p.MapID, ShowID: p.ShowID})
	broadcastPlayerMeleeAttack(s.deps.World, p, 2001, 1)
	if got := clientAttackPackets(drainSkillTestPackets(p.Session)); len(got) != 1 {
		t.Fatal("actor must receive its own attack")
	}
	if got := clientAttackPackets(drainSkillTestPackets(observer.Session)); len(got) != 0 {
		t.Fatal("GM invisible actor exposed")
	}
}

func TestQueuedAttackRechecksWeightBeforeDamage(t *testing.T) {
	s, p := newClientAttackTestSystem(t)
	p.Str, p.Con, p.ClassType = 1, 1, 1
	npc := &world.NpcInfo{ID: 2001, Impl: "L1Monster", X: 102, Y: 100, MapID: p.MapID, ShowID: p.ShowID, HP: 100, MaxHP: 100}
	s.deps.World.AddNpc(npc)
	sendClientAttackForTest(s, p, packet.C_OPCODE_ATTACK, npc.ID, npc.X, npc.Y)
	if len(s.requests) != 1 {
		t.Fatal("initial light attack should queue")
	}
	p.Inv.AddItem(40010, 300, "weight", 0, 1000, true, 1)
	s.Update(0)
	if npc.HP != 100 || len(clientAttackPackets(drainSkillTestPackets(p.Session))) != 0 {
		t.Fatal("queued attack must not damage or animate after becoming overweight")
	}
}
