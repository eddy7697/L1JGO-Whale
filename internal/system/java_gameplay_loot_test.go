package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

func newJavaLootTest(t *testing.T) (*ItemUseSystem, *world.PlayerInfo, *world.NpcInfo) {
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, Name: "receiver", X: 100, Y: 100, MapID: 4, Str: 20, Con: 20})
	s := newPacketBoxDropItemUseSystem(t, ws)
	s.deps.ItemCreate = NewItemCreateSystem(s.deps)
	return s, p, &world.NpcInfo{ID: 2001, Name: "orc", X: 101, Y: 100, MapID: 4}
}

func TestJavaMonsterLootSendsMonsterMessage(t *testing.T) {
	s, p, npc := newJavaLootTest(t)
	s.distributeMonsterDrop(p, npc, data.DropItem{ItemID: world.AdenaItemID}, 100)
	want := packet.NewWriterWithOpcode(packet.S_OPCODE_MESSAGE_CODE)
	want.WriteH(143)
	want.WriteC(2)
	want.WriteS(npc.Name)
	want.WriteS(s.deps.Items.Get(world.AdenaItemID).Name + " (100)")
	if !hasExactPacket(drainSkillTestPackets(p.Session), want.Bytes()) {
		t.Fatal("missing monster-gave-item message with monster name and quantity")
	}
	if got := p.Inv.FindByItemID(world.AdenaItemID); got == nil || got.Count != 100 {
		t.Fatal("missing adena")
	}
}

func TestJavaMonsterLootFullInventoryKeepsStackAndDropsEquipment(t *testing.T) {
	s, p, npc := newJavaLootTest(t)
	p.Inv.AddItem(world.AdenaItemID, 10, "adena", 0, 0, true, 1)
	for p.Inv.Size() < world.MaxInventorySize {
		p.Inv.AddItem(1, 1, "filler", 0, 0, false, 1)
	}
	s.distributeMonsterDrop(p, npc, data.DropItem{ItemID: world.AdenaItemID}, 100)
	if p.Inv.FindByItemID(world.AdenaItemID).Count != 110 {
		t.Fatal("full inventory rejected an existing stack")
	}
	drainSkillTestPackets(p.Session)
	s.distributeMonsterDrop(p, npc, data.DropItem{ItemID: 1, EnchantLevel: 3}, 1)
	ground := s.deps.World.GetNearbyGroundItems(p.X, p.Y, p.MapID)
	if len(ground) != 1 || ground[0].Count != 1 || ground[0].EnchantLvl != 3 || ground[0].Item.Identified {
		t.Fatalf("equipment should land intact and unidentified: %+v", ground)
	}
	if hasServerMessage(drainSkillTestPackets(p.Session), 143) {
		t.Fatal("ground drop must not announce an inventory award")
	}
	// After making room, pickup must preserve the template and enchantment.
	p.Inv.RemoveItem(p.Inv.Items[len(p.Inv.Items)-1].ObjectID, 1)
	NewItemGroundSystem(s.deps).PickupItem(p.Session, p, ground[0].ID)
	var picked *world.InvItem
	for _, item := range p.Inv.Items {
		if item.ItemID == 1 && item.EnchantLvl == 3 {
			picked = item
		}
	}
	info := s.deps.Items.Get(1)
	if picked == nil || picked.Name != info.Name || picked.Weight != info.Weight || picked.InvGfx != info.InvGfx || picked.Identified {
		t.Fatalf("ground pickup lost item metadata: %+v", picked)
	}
}

func TestJavaGiveDropsUsesEligibleAttackerAndRates(t *testing.T) {
	for _, tc := range []struct {
		name               string
		dropRate, goldRate float64
		eligible           bool
		want               int32
	}{
		{"single attacker differs from killer", 1, 2, true, 20},
		{"drop rate zero", 0, 1, true, 0},
		{"gold rate zero", 1, 0, true, 0},
		{"no nearby attacker", 1, 1, false, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, killer, npc := newJavaLootTest(t)
			npc.NpcID = 45000
			path := filepath.Join(t.TempDir(), "drops.yaml")
			if err := os.WriteFile(path, []byte("drops:\n  - mob_id: 45000\n    items:\n      - item_id: 40308\n        min: 10\n        max: 10\n        chance: 1000000\n"), 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			s.deps.Drops, err = data.LoadDropTable(path)
			if err != nil {
				t.Fatal(err)
			}
			s.deps.Config = &config.Config{Rates: config.RatesConfig{DropRate: tc.dropRate, GoldRate: tc.goldRate}}
			attacker := addSkillTestPlayer(s.deps.World, &world.PlayerInfo{SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, X: 100, Y: 100, MapID: 4, Str: 20, Con: 20})
			if !tc.eligible {
				attacker.ShowID = 99
			}
			npc.HateList = map[uint64]int32{attacker.SessionID: 100}
			s.GiveDrops(killer, npc)
			if killer.Inv.FindByItemID(world.AdenaItemID) != nil {
				t.Fatal("ineligible killer stole loot")
			}
			got := int32(0)
			if item := attacker.Inv.FindByItemID(world.AdenaItemID); item != nil {
				got = item.Count
			}
			if !tc.eligible {
				if got != 0 {
					t.Fatal("different-show attacker received loot")
				}
				for _, g := range s.deps.World.GetNearbyGroundItems(npc.X, npc.Y, npc.MapID) {
					got += g.Count
				}
			}
			if got != tc.want {
				t.Fatalf("quantity=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestJavaMonsterLootOverweightFallsAtReceiver(t *testing.T) {
	s, p, npc := newJavaLootTest(t)
	p.Inv.AddItem(1, 1, "heavy", 0, 2147483000, false, 1)
	s.distributeMonsterDrop(p, npc, data.DropItem{ItemID: 40010}, 2)
	ground := s.deps.World.GetNearbyGroundItems(p.X, p.Y, p.MapID)
	if len(ground) != 1 || ground[0].Count != 2 || ground[0].X != p.X {
		t.Fatal("overweight loot was lost instead of falling at receiver")
	}
}

func TestJavaMonsterLootAutoShareConservesRemainderAndAnnounces(t *testing.T) {
	s, p, npc := newJavaLootTest(t)
	q := addSkillTestPlayer(s.deps.World, &world.PlayerInfo{SessionID: 2, Session: newSkillTestSession(t, 2), CharID: 1002, Name: "member", X: 180, Y: 100, MapID: 4, Str: 20, Con: 20})
	s.deps.World.Parties.CreateParty(p.CharID, q.CharID, world.PartyTypeAutoShare)
	p.PartyID, q.PartyID = p.CharID, p.CharID
	s.distributeMonsterDrop(p, npc, data.DropItem{ItemID: world.AdenaItemID}, 101)
	for _, member := range []*world.PlayerInfo{p, q} {
		if !hasServerMessage(drainSkillTestPackets(member.Session), 813) {
			t.Fatal("party member missing recipient message")
		}
	}
	if p.Inv.FindByItemID(world.AdenaItemID).Count != 51 || q.Inv.FindByItemID(world.AdenaItemID).Count != 50 {
		t.Fatal("party split lost or duplicated loot")
	}
}
