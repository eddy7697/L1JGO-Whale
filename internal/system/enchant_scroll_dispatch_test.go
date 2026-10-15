package system

import (
	"fmt"
	"testing"

	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

func TestSpecialEnchantScrollsThroughClientPacket(t *testing.T) {
	for _, scrollID := range []int32{41429, 41430, 41431, 41432, 49312, 49311, 49148} {
		t.Run(fmt.Sprint(scrollID), func(t *testing.T) {
			ws := world.NewState()
			p := newEnchantScrollTestPlayer(t, ws)
			s := newEnchantScrollTestSystem(t, ws)
			s.deps.ItemUse = s
			s.deps.Config.Enchant.AttrChance = 1
			scroll := addEnchantScrollTestItem(s, p, 7001, scrollID, 2)
			targetID := int32(31)
			switch scrollID {
			case 49312:
				targetID = 7
			case 49311:
				targetID = 20028 // Production data has safe_enchant=0: reject without consuming.
			case 49148:
				targetID = 20282
			}
			target := addEnchantScrollTestItem(s, p, 7002, targetID, 1)
			w := packet.NewWriterWithOpcode(packet.C_OPCODE_USE_ITEM)
			w.WriteD(scroll.ObjectID)
			w.WriteD(target.ObjectID)
			handler.HandleUseItem(p.Session, packet.NewReader(w.Bytes()), s.deps)
			if scrollID == 49311 {
				if scroll.Count != 2 || target.EnchantLvl != 0 || !hasEnchantDispatchMessage(drainSkillTestPackets(p.Session), 79) {
					t.Fatal("ivory armor rejected outside the special scroll route or consumed")
				}
				return
			}
			if scroll.Count != 1 {
				t.Fatal("client request did not consume exactly one scroll")
			}
			if kind := world.AttrEnchantScrollKind(scrollID); kind != 0 {
				if target.AttrEnchantKind != kind || target.AttrEnchantLevel != 1 || target.EnchantLvl != 0 {
					t.Fatalf("attribute scroll routed to ordinary enchant: %+v", target)
				}
			} else if scrollID == 49312 {
				if target.EnchantLvl != 1 {
					t.Fatal("ivory weapon was not upgraded")
				}
			} else if got := p.Inv.FindByObjectID(target.ObjectID); got != nil && (got.EnchantLvl != 1 || got.Accessory.AddMP != 1) {
				t.Fatal("accessory request neither upgraded with its grade bonus nor destroyed the target")
			}
			for _, pkt := range drainSkillTestPackets(p.Session) {
				if pkt[0] == packet.S_OPCODE_EFFECT {
					t.Fatal("special enchant emitted an extra animation absent from Java")
				}
			}
		})
	}
}

func hasEnchantDispatchMessage(packets [][]byte, id uint16) bool {
	for _, pkt := range packets {
		if len(pkt) >= 3 && pkt[0] == packet.S_OPCODE_MESSAGE_CODE && uint16(pkt[1])|uint16(pkt[2])<<8 == id {
			return true
		}
	}
	return false
}
