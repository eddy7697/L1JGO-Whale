package system

import (
	"encoding/binary"
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

func TestJavaLightItemsToggleThroughClientPacket(t *testing.T) {
	items, err := data.LoadItemTable("../../data/yaml/weapon_list.yaml", "../../data/yaml/armor_list.yaml", "../../data/yaml/etcitem_list.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id     int32
		radius byte
	}{{40001, 11}, {40002, 14}, {40004, 14}, {40005, 8}} {
		t.Run(items.Get(tc.id).Name, func(t *testing.T) {
			ws := world.NewState()
			p := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, X: 100, Y: 100, MapID: 4, ShowID: 7})
			viewer := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, X: 101, Y: 100, MapID: 4, ShowID: 7})
			other := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 3, Session: newSkillTestSession(t, 3), CharID: 1003, X: 101, Y: 100, MapID: 4, ShowID: 8})
			deps := newSkillTestSystem(t, ws).deps
			deps.Items = items
			deps.ItemUse = NewItemUseSystem(deps)
			info := items.Get(tc.id)
			item := p.Inv.AddItem(tc.id, 1, info.Name, info.InvGfx, info.Weight, false, byte(info.Bless))
			for _, radius := range []byte{tc.radius, 0} {
				w := packet.NewWriterWithOpcode(packet.C_OPCODE_USE_ITEM)
				w.WriteD(item.ObjectID)
				handler.HandleUseItem(p.Session, packet.NewReader(w.Bytes()), deps)
				if p.LightSize != radius || item.NowLighting != (radius > 0) || p.Inv.FindByObjectID(item.ObjectID) == nil {
					t.Fatalf("toggle light=%d item=%+v", p.LightSize, item)
				}
				for _, recipient := range []*world.PlayerInfo{p, viewer} {
					count := 0
					for _, pkt := range drainSkillTestPackets(recipient.Session) {
						if pkt[0] == packet.S_OPCODE_CHANGE_LIGHT {
							count++
							if binary.LittleEndian.Uint32(pkt[1:5]) != uint32(p.CharID) || pkt[5] != radius {
								t.Fatalf("bad light packet %x", pkt)
							}
						}
					}
					if count != 1 {
						t.Fatalf("recipient %d light packets=%d", recipient.CharID, count)
					}
				}
				if got := drainSkillTestPackets(other.Session); len(got) != 0 {
					t.Fatalf("light leaked to another instance: %x", got)
				}
			}
		})
	}
}

func TestJavaLightSourcesSurviveSpellExpiryAndUpdateAfterRemoval(t *testing.T) {
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, X: 100, Y: 100, MapID: 4})
	viewer := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, X: 101, Y: 100, MapID: 4})
	lamp := p.Inv.AddItem(40001, 1, "lamp", 0, 20000, false, 1)
	lamp.NowLighting = true
	p.AddBuff(&world.ActiveBuff{SkillID: 2, TicksLeft: 20})
	handler.UpdatePlayerLight(p, ws)
	if p.LightSize != 14 {
		t.Fatal("LIGHT spell should provide radius 14")
	}
	drainSkillTestPackets(p.Session)
	p.RemoveBuff(2)
	handler.UpdatePlayerLight(p, ws)
	if p.LightSize != 11 {
		t.Fatal("spell expiry should keep the active lamp")
	}
	drainSkillTestPackets(p.Session)
	p.Inv.RemoveItem(lamp.ObjectID, 1)
	drainSkillTestPackets(viewer.Session)
	NewBuffTickSystem(ws, newSkillTestSystem(t, ws).deps).Update(0)
	if p.LightSize != 0 {
		t.Fatal("removed lamp left a stale light circle")
	}
	packets := drainSkillTestPackets(viewer.Session)
	if len(packets) != 1 || packets[0][0] != packet.S_OPCODE_CHANGE_LIGHT || packets[0][5] != 0 {
		t.Fatalf("viewer did not receive the extinguished light: %x", packets)
	}
}

func TestJavaLanternOilAndEmptyFuel(t *testing.T) {
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001})
	deps := newSkillTestSystem(t, ws).deps
	items, err := data.LoadItemTable("../../data/yaml/weapon_list.yaml", "../../data/yaml/armor_list.yaml", "../../data/yaml/etcitem_list.yaml")
	if err != nil {
		t.Fatal(err)
	}
	deps.Items = items
	s := NewItemUseSystem(deps)
	lamp := p.Inv.AddItem(40002, 1, "lantern", 0, 0, false, 1)
	lamp.LightFuel = 0
	s.UseConsumable(p.Session, p, lamp, items.Get(40002))
	if lamp.NowLighting {
		t.Fatal("empty lantern must not light")
	}
	oil := p.Inv.AddItem(40003, 1, "oil", 0, 0, true, 1)
	if !s.UseConsumable(p.Session, p, oil, items.Get(40003)) || lamp.LightFuel != 12000 || p.Inv.FindByObjectID(oil.ObjectID) != nil {
		t.Fatal("oil must refill lantern and be consumed")
	}
	drainSkillTestPackets(p.Session)
	s.UseConsumable(p.Session, p, lamp, items.Get(40002))
	if p.LightSize != 14 {
		t.Fatal("refilled lantern must light")
	}
}

func TestMonsterDroppedLanternCanLightImmediatelyAfterPickup(t *testing.T) {
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Str: 18, Con: 18, X: 100, Y: 100, MapID: 4})
	deps := newSkillTestSystem(t, ws).deps
	items, err := data.LoadItemTable("../../data/yaml/weapon_list.yaml", "../../data/yaml/armor_list.yaml", "../../data/yaml/etcitem_list.yaml")
	if err != nil {
		t.Fatal(err)
	}
	deps.Items = items
	s := NewItemUseSystem(deps)
	npc := &world.NpcInfo{ID: 2001, X: p.X, Y: p.Y, MapID: p.MapID}
	s.dropMonsterLootOnGround(p, npc, data.DropItem{ItemID: 40002}, 1)
	ground := ws.GetNearbyGroundItems(p.X, p.Y, p.MapID)
	if len(ground) != 1 {
		t.Fatalf("ground items=%d", len(ground))
	}
	NewItemGroundSystem(deps).PickupItem(p.Session, p, ground[0].ID)
	lamp := p.Inv.FindByItemID(40002)
	if lamp == nil || lamp.LightFuel != 12000 {
		t.Fatalf("new dropped lantern fuel not initialized: %+v", lamp)
	}
	s.UseConsumable(p.Session, p, lamp, items.Get(40002))
	if p.LightSize != 14 || !lamp.NowLighting {
		t.Fatal("picked-up lantern should light without relogging")
	}
}

func TestJavaOrdinaryDeathCannotHealOrDrinkAndZeroMPStaysEmpty(t *testing.T) {
	for _, access := range []int16{0, 100, 200} {
		ws := world.NewState()
		p := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, AccessLevel: access, GMModeOff: true, HP: 5, MaxHP: 100, MP: 5, MaxMP: 100})
		deps := newSkillTestSystem(t, ws).deps
		deps.Death = NewDeathSystem(deps)
		p.SpendMP(5)
		if p.MP != 0 {
			t.Fatalf("access=%d empty MP refilled", access)
		}
		p.SpendMP(1)
		if p.MP != 0 {
			t.Fatalf("access=%d MP underflow=%d", access, p.MP)
		}
		p.ApplyDamage(5)
		if p.HP != 0 {
			t.Fatalf("access=%d lethal damage refilled HP", access)
		}
		deps.Death.KillPlayer(p)
		if !p.Dead || p.HP != 0 {
			t.Fatal("must enter death state")
		}
		deathPackets := drainSkillTestPackets(p.Session)
		deaths := 0
		for _, pkt := range deathPackets {
			if len(pkt) > 5 && pkt[0] == packet.S_OPCODE_ACTION && pkt[5] == 8 {
				deaths++
			}
		}
		if deaths != 1 || !p.Dirty {
			t.Fatalf("death animation=%d dirty=%v", deaths, p.Dirty)
		}
		if applyPlayerHealDeltaLikeJava(deps, p, 100) || p.HP != 0 {
			t.Fatal("ordinary healing resurrected a dead player")
		}
		p.Inv.AddItemWithID(5001, 40010, 2, "potion", 0, 0, true, 1)
		w := packet.NewWriterWithOpcode(packet.C_OPCODE_USE_ITEM)
		w.WriteD(5001)
		// No item table: the dead-player guard must run before item dispatch.
		handler.HandleUseItem(p.Session, packet.NewReader(w.Bytes()), deps)
		if p.HP != 0 || p.Inv.FindByObjectID(5001).Count != 2 {
			t.Fatal("dead character used a potion")
		}
	}
}
