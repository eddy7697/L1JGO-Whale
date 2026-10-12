package system

import (
	"encoding/binary"
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

func newClientAttackTestSystem(t *testing.T) (*CombatSystem, *world.PlayerInfo) {
	t.Helper()
	ws := world.NewState()
	player := addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001,
		X: 101, Y: 100, MapID: 900, ShowID: 10,
	})
	deps := &handler.Deps{World: ws}
	s := NewCombatSystem(deps)
	deps.Combat = s
	return s, player
}

func sendClientAttackForTest(s *CombatSystem, player *world.PlayerInfo, opcode byte, targetID, x, y int32) {
	w := packet.NewWriterWithOpcode(opcode)
	w.WriteD(targetID)
	w.WriteH(uint16(x))
	w.WriteH(uint16(y))
	if opcode == packet.C_OPCODE_FAR_ATTACK {
		handler.HandleFarAttack(player.Session, packet.NewReader(w.Bytes()), s.deps)
	} else {
		handler.HandleAttack(player.Session, packet.NewReader(w.Bytes()), s.deps)
	}
}

func clientAttackPackets(packets [][]byte) [][]byte {
	var attacks [][]byte
	for _, p := range packets {
		if len(p) > 0 && p[0] == packet.S_OPCODE_ATTACK {
			attacks = append(attacks, p)
		}
	}
	return attacks
}

func TestClientAttackMissingAndDeadTargetsReplyOncePerRequest(t *testing.T) {
	for _, targetKind := range []string{"missing", "dead_npc", "dead_player"} {
		t.Run(targetKind, func(t *testing.T) {
			s, player := newClientAttackTestSystem(t)
			const targetID int32 = 2001
			if targetKind == "dead_npc" {
				s.deps.World.AddNpc(&world.NpcInfo{ID: targetID, X: 100, Y: 100, MapID: 900, ShowID: 10, Dead: true})
			} else if targetKind == "dead_player" {
				addSkillTestPlayer(s.deps.World, &world.PlayerInfo{SessionID: 4, Session: newSkillTestSession(t, 4), CharID: targetID, X: 100, Y: 100, MapID: 900, ShowID: 10, Dead: true})
			}
			observer := addSkillTestPlayer(s.deps.World, &world.PlayerInfo{SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, X: 102, Y: 100, MapID: 900, ShowID: 10})
			otherShow := addSkillTestPlayer(s.deps.World, &world.PlayerInfo{SessionID: 3, Session: newSkillTestSession(t, 3), CharID: 1003, X: 102, Y: 100, MapID: 900, ShowID: 20})
			for _, opcode := range []byte{packet.C_OPCODE_ATTACK, packet.C_OPCODE_FAR_ATTACK, packet.C_OPCODE_ATTACK} {
				sendClientAttackForTest(s, player, opcode, targetID, 101, 102)
			}
			s.Update(0)
			for _, recipient := range []*world.PlayerInfo{player, observer} {
				attacks := clientAttackPackets(drainSkillTestPackets(recipient.Session))
				if len(attacks) != 3 {
					t.Fatalf("每個完整客戶端攻擊應回覆一次動作，recipient=%d got=%d", recipient.CharID, len(attacks))
				}
				for _, p := range attacks {
					// Eddy S_AttackPacket: opcode/action/D actor/D target/H damage/C heading/D 0/C effect.
					if len(p) != 24 || p[1] != 1 || binary.LittleEndian.Uint32(p[2:6]) != uint32(player.CharID) || binary.LittleEndian.Uint32(p[6:10]) != uint32(targetID) || binary.LittleEndian.Uint16(p[10:12]) != 0 || p[12] != 4 {
						t.Fatalf("空揮應使用封包座標朝南且不產生傷害: %x", p)
					}
				}
			}
			if got := clientAttackPackets(drainSkillTestPackets(otherShow.Session)); len(got) != 0 {
				t.Fatalf("不得向其他副本廣播空揮: %x", got)
			}
			s.Update(0)
			if got := clientAttackPackets(drainSkillTestPackets(player.Session)); len(got) != 0 {
				t.Fatalf("沒有新客戶端封包就不能自行連續攻擊: %x", got)
			}
		})
	}
}

func TestClientAttackRangeAndLineOfSightMissStillReplyWithoutDamage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		x      int32
		opcode byte
		wall   bool
	}{
		{"melee_range", 104, packet.C_OPCODE_ATTACK, false},
		{"ranged_range", 112, packet.C_OPCODE_FAR_ATTACK, false},
		{"melee_wall", 103, packet.C_OPCODE_ATTACK, true},
		{"ranged_wall", 103, packet.C_OPCODE_FAR_ATTACK, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, player := newClientAttackTestSystem(t)
			if tc.wall {
				s.deps.MapData = newSkillLOSTestMap(t)
			}
			npc := &world.NpcInfo{ID: 2001, Impl: "L1Monster", X: tc.x, Y: 100, MapID: 900, ShowID: 10, HP: 100, MaxHP: 100}
			s.deps.World.AddNpc(npc)
			sendClientAttackForTest(s, player, tc.opcode, npc.ID, npc.X, npc.Y)
			s.Update(0)
			attacks := clientAttackPackets(drainSkillTestPackets(player.Session))
			if len(attacks) != 1 || binary.LittleEndian.Uint16(attacks[0][10:12]) != 0 {
				t.Fatalf("射程／視線 MISS 仍應回覆本人零傷害動作: %x", attacks)
			}
			if npc.HP != 100 || len(npc.HateList) != 0 || player.HP != 100 {
				t.Fatalf("空揮不能改變 HP 或仇恨: npcHP=%d playerHP=%d hate=%v", npc.HP, player.HP, npc.HateList)
			}
		})
	}
}

func TestClientAttackMissDoesNotBypassTargetOrAttackerGuards(t *testing.T) {
	for _, tc := range []string{"dead_attacker", "paralyzed", "sleep", "other_map", "other_show", "hidden_sink", "hidden_fly", "beyond_twenty", "dead_other_map", "dead_other_show"} {
		t.Run(tc, func(t *testing.T) {
			s, player := newClientAttackTestSystem(t)
			npc := &world.NpcInfo{ID: 2001, Impl: "L1Monster", X: 104, Y: 100, MapID: 900, ShowID: 10, HP: 100, MaxHP: 100}
			switch tc {
			case "dead_attacker":
				player.Dead = true
			case "paralyzed":
				player.Paralyzed = true
			case "sleep":
				player.Sleeped = true
			case "other_map", "dead_other_map":
				npc.MapID = 901
			case "other_show", "dead_other_show":
				npc.ShowID = 11
			case "hidden_sink":
				npc.HiddenStatus = world.NpcHiddenSink
			case "hidden_fly":
				npc.HiddenStatus = world.NpcHiddenFly
			case "beyond_twenty":
				npc.X = 122
			}
			if tc == "dead_other_map" || tc == "dead_other_show" {
				npc.Dead = true
			}
			s.deps.World.AddNpc(npc)
			sendClientAttackForTest(s, player, packet.C_OPCODE_ATTACK, npc.ID, npc.X, npc.Y)
			s.Update(0)
			if got := clientAttackPackets(drainSkillTestPackets(player.Session)); len(got) != 0 {
				t.Fatalf("不合法攻擊不應因空揮補回封包而繞過限制: %x", got)
			}
			if npc.HP != 100 || len(npc.HateList) != 0 {
				t.Fatalf("不合法攻擊不應改變 HP 或仇恨")
			}
		})
	}
}

func TestClientAttackBowMissUsesAndConsumesExistingArrow(t *testing.T) {
	items, err := data.LoadItemTable("../../data/yaml/weapon_list.yaml", "../../data/yaml/armor_list.yaml", "../../data/yaml/etcitem_list.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, weaponID := range []int32{167, 178} { // bow、singlebow 在 Java 都是 weaponType=20。
		for _, count := range []int32{0, 1, 2} {
			s, player := newClientAttackTestSystem(t)
			s.deps.Items = items
			player.Equip.Set(world.SlotWeapon, &world.InvItem{ObjectID: 5001, ItemID: weaponID})
			if count > 0 {
				player.Inv.AddItemWithID(5002, 40741, count, "arrow", 0, 0, true, 0)
			}
			for shot := int32(0); shot <= count; shot++ {
				// 最後一支箭消耗完後改成活目標，確保 no-arrow 也有動作回覆。
				if shot == count {
					s.deps.World.AddNpc(&world.NpcInfo{ID: 2001, Impl: "L1Monster", X: 102, Y: 100, MapID: 900, ShowID: 10, HP: 100, MaxHP: 100})
				}
				sendClientAttackForTest(s, player, packet.C_OPCODE_FAR_ATTACK, 2001, 104, 100)
				s.Update(0)
				attacks := clientAttackPackets(drainSkillTestPackets(player.Session))
				if len(attacks) != 1 || binary.LittleEndian.Uint16(attacks[0][10:12]) != 0 {
					t.Fatalf("弓箭空揮應送一個零傷害動作: weapon=%d shot=%d packets=%x", weaponID, shot, attacks)
				}
				if shot == count {
					if len(attacks[0]) != 24 {
						t.Fatalf("沒有箭應播放一般空揮，不能憑空產生箭矢: %x", attacks[0])
					}
				} else if len(attacks[0]) != 32 || binary.LittleEndian.Uint16(attacks[0][17:19]) != 66 {
					t.Fatalf("有箭時空揮應送箭矢: %x", attacks[0])
				}
				remaining := player.Inv.FindByObjectID(5002)
				wantCount := count - shot - 1
				if wantCount > 0 && (remaining == nil || remaining.Count != wantCount) {
					t.Fatalf("每次空揮應消耗一支箭，want=%d got=%+v", wantCount, remaining)
				}
				if wantCount <= 0 && remaining != nil {
					t.Fatalf("最後一支箭用完應移除物品: %+v", remaining)
				}
			}
		}
	}
}

func TestClientAttackMissDoesNotChangeSkillGeneratedRequests(t *testing.T) {
	s, player := newClientAttackTestSystem(t)
	s.QueueAttack(handler.AttackRequest{AttackerSessionID: player.SessionID, TargetID: 2001, IsMelee: true})
	s.Update(0)
	if got := clientAttackPackets(drainSkillTestPackets(player.Session)); len(got) != 0 {
		t.Fatalf("技能內部的失效目標不應新增原生封包空揮回覆: %x", got)
	}
}
