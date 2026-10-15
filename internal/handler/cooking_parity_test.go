package handler

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// Fixture transcribed from C_ItemUSe.makeCooking, independent of the production table.
var javaCookingMaterials = [][]int32{
	{40057}, {41275}, {41263, 41265}, {41274, 41267},
	{40062, 40069, 40064}, {40056, 40060, 40061}, {41276}, {40499, 40060},
	{49040, 49048}, {49041, 49048}, {49042, 41265, 49048}, {49043, 49048},
	{49044, 49048}, {49045, 49048}, {49046, 49048}, {49047, 40499, 49048},
	{49048, 49243, 49260}, {49048, 49243, 49261}, {49048, 49243, 49262}, {49048, 49243, 49263},
	{49048, 49243, 49264}, {49048, 49243, 49265}, {49048, 49243, 49266}, {49048, 49243, 49267, 40499},
}

type cookingConsumeInventory struct {
	NpcServiceManager
	calls int
}

func (s *cookingConsumeInventory) ConsumeItem(_ *net.Session, player *world.PlayerInfo, objectID, count int32) bool {
	item := player.Inv.FindByObjectID(objectID)
	if item == nil || item.Count < count {
		return false
	}
	s.calls++
	player.Inv.RemoveItem(objectID, count)
	player.Dirty = true
	return true
}

type cookingCreateInventory struct {
	cookingItemCreateStub
	fail bool
}

func (s *cookingCreateInventory) GiveItem(sess *net.Session, p *world.PlayerInfo, id, count int32) (*world.InvItem, bool) {
	if s.fail {
		return nil, false
	}
	s.itemID, s.count = id, count
	return p.Inv.AddItem(id, count, "cooked food", 0, 0, true, 1), true
}

func cookingTestItems(t *testing.T) *data.ItemTable {
	t.Helper()
	dir := t.TempDir()
	ids := map[int32]bool{}
	for id := int32(41255); id <= 41260; id++ {
		ids[id] = true
	}
	for no, materials := range javaCookingMaterials {
		for _, id := range materials {
			ids[id] = true
		}
		base := []int32{41277, 49049, 49244}[no/8] + int32(no%8)
		ids[base], ids[base+8] = true, true
	}
	var text strings.Builder
	text.WriteString("items:\n")
	for id := range ids {
		fmt.Fprintf(&text, "  - item_id: %d\n    name: item%d\n    item_type: other\n    use_type: normal\n    stackable: true\n    bless: 1\n", id, id)
	}
	items, err := data.LoadItemTable(
		writeHandlerTestYAML(t, dir, "weapons.yaml", "weapons: []\n"),
		writeHandlerTestYAML(t, dir, "armors.yaml", "armors: []\n"),
		writeHandlerTestYAML(t, dir, "items.yaml", text.String()))
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func cookingFixture(t *testing.T, items *data.ItemTable, no int) (*net.Session, *world.PlayerInfo, *Deps, *cookingConsumeInventory, *cookingCreateInventory) {
	t.Helper()
	sess := newCookingTestSession(t)
	p := &world.PlayerInfo{SessionID: sess.ID, Session: sess, CharID: 1001, HP: 100, MapID: 4, ShowID: 7, X: 100, Y: 100, Str: 10, Con: 10, Inv: world.NewInventory()}
	for _, id := range javaCookingMaterials[no] {
		p.Inv.AddItem(id, 2, "ingredient", 0, 0, true, 1)
	}
	ws := world.NewState()
	ws.AddPlayer(p)
	ws.AddGroundEffect(&world.GroundEffect{ID: 2001, NpcID: 81170, GfxID: 5943, Type: world.GroundEffectNpcEffect, X: 103, Y: 103, MapID: 4, ShowID: 7, TicksLeft: 3000})
	consume, create := &cookingConsumeInventory{}, &cookingCreateInventory{}
	return sess, p, &Deps{World: ws, Items: items, NpcSvc: consume, ItemCreate: create, Log: zap.NewNop()}, consume, create
}

func cookingHasMessage(packets [][]byte, id uint16) bool {
	for _, b := range packets {
		if len(b) >= 3 && b[0] == packet.S_OPCODE_MESSAGE_CODE && binary.LittleEndian.Uint16(b[1:3]) == id {
			return true
		}
	}
	return false
}

func cookingHasEffect(packets [][]byte, gfx uint16) bool {
	for _, b := range packets {
		if len(b) >= 7 && b[0] == packet.S_OPCODE_EFFECT && binary.LittleEndian.Uint16(b[5:7]) == gfx {
			return true
		}
	}
	return false
}

func TestCookingAllJavaRecipesAndOutcomeBoundaries(t *testing.T) {
	items := cookingTestItems(t)
	for no := range javaCookingMaterials {
		for _, roll := range []int{1, 90, 91, 95, 96, 100} {
			t.Run(fmt.Sprintf("recipe%d/roll%d", no, roll), func(t *testing.T) {
				sess, p, deps, consume, create := cookingFixture(t, items, no)
				observer := newHandlerTestSession(t, 2)
				deps.World.AddPlayer(&world.PlayerInfo{SessionID: 2, Session: observer, CharID: 1002, X: 100, Y: 100, MapID: 4, ShowID: 7})
				handleCookingSelectAt(sess, p, no, deps, roll, time.Now())
				if consume.calls != len(javaCookingMaterials[no]) {
					t.Fatalf("consumed %d ingredients, want %d", consume.calls, len(javaCookingMaterials[no]))
				}
				for _, id := range javaCookingMaterials[no] {
					if p.Inv.FindByItemID(id).Count != 1 {
						t.Fatalf("ingredient %d did not consume exactly one", id)
					}
				}
				wantResult := []int32{41277, 49049, 49244}[no/8] + int32(no%8)
				wantGfx := uint16(6392)
				if roll > 90 {
					wantResult += 8
					wantGfx = 6390
				}
				if roll > 95 {
					wantResult = 0
					wantGfx = 6394
				}
				if create.itemID != wantResult {
					t.Fatalf("result=%d want %d", create.itemID, wantResult)
				}
				selfPackets, others := drainHandlerTestPackets(sess), drainHandlerTestPackets(observer)
				if !cookingHasEffect(others, wantGfx) || cookingHasEffect(selfPackets, wantGfx) != (roll >= 91 && roll <= 95) {
					t.Fatalf("incorrect Java self/observer animation: self=%x other=%x", selfPackets, others)
				}
				if cookingHasMessage(selfPackets, 1101) != (roll > 95) {
					t.Fatal("failure message does not match outcome")
				}
				if cookingHasMessage(selfPackets, 403) != (roll <= 95) {
					t.Fatal("acquisition message does not match outcome")
				}
				for _, b := range selfPackets {
					if len(b) > 1 && b[0] == packet.S_OPCODE_EVENT && b[1] == 53 {
						t.Fatal("crafting sent an eating buff icon")
					}
				}
				if len(p.ActiveBuffs) != 1 || p.GetBuff(2999) == nil {
					t.Fatal("crafting should only set COOKING_NOW")
				}
			})
		}
	}
}

func TestCookingChanceDistributionMatchesJava(t *testing.T) {
	items := cookingTestItems(t)
	counts := map[int32]int{}
	for roll := 1; roll <= 100; roll++ {
		sess, p, deps, _, create := cookingFixture(t, items, 0)
		handleCookingSelectAt(sess, p, 0, deps, roll, time.Now())
		counts[create.itemID]++
	}
	if counts[41277] != 90 || counts[41285] != 5 || counts[0] != 5 {
		t.Fatalf("distribution=%v", counts)
	}
}

func TestCookingRejectsMissingFireMaterialsWeightAndCooldown(t *testing.T) {
	items := cookingTestItems(t)
	cases := []struct {
		name    string
		message uint16
		mutate  func(*world.PlayerInfo, *Deps)
	}{
		{"no fire", 1160, func(p *world.PlayerInfo, d *Deps) { d.World.RemoveGroundEffect(2001) }},
		{"four tiles", 1160, func(p *world.PlayerInfo, d *Deps) { d.World.GetGroundEffect(2001).X = 104 }},
		{"other map", 1160, func(p *world.PlayerInfo, d *Deps) { d.World.GetGroundEffect(2001).MapID = 0 }},
		{"other instance", 1160, func(p *world.PlayerInfo, d *Deps) { d.World.GetGroundEffect(2001).ShowID = 8 }},
		{"wrong gfx", 1160, func(p *world.PlayerInfo, d *Deps) { d.World.GetGroundEffect(2001).GfxID = 5942 }},
		{"expired fire", 1160, func(p *world.PlayerInfo, d *Deps) { d.World.GetGroundEffect(2001).TicksLeft = 0 }},
		{"missing last ingredient", 1102, func(p *world.PlayerInfo, d *Deps) {
			item := p.Inv.FindByItemID(40499)
			p.Inv.RemoveItem(item.ObjectID, 2)
		}},
		{"maximum weight", 1103, func(p *world.PlayerInfo, d *Deps) {
			p.Inv.AddItem(999, 1, "heavy", 0, int32(world.PlayerMaxWeight(p))*1000, true, 1)
		}},
		{"cooldown", 0, func(p *world.PlayerInfo, d *Deps) { p.AddBuff(&world.ActiveBuff{SkillID: 2999, TicksLeft: 15}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess, p, deps, consume, create := cookingFixture(t, items, 23)
			tc.mutate(p, deps)
			handleCookingSelectAt(sess, p, 23, deps, 1, time.Now())
			if consume.calls != 0 || create.itemID != 0 {
				t.Fatal("rejected cooking changed inventory")
			}
			packets := drainHandlerTestPackets(sess)
			if tc.message != 0 && !cookingHasMessage(packets, tc.message) {
				t.Fatalf("missing message %d: %x", tc.message, packets)
			}
		})
	}
	for _, no := range []int{-1, 24, 255} {
		sess, p, deps, consume, _ := cookingFixture(t, items, 0)
		handleCookingSelectAt(sess, p, no, deps, 1, time.Now())
		if consume.calls != 0 || len(p.ActiveBuffs) != 0 {
			t.Fatal("invalid recipe consumed or set cooldown")
		}
	}
	// A delayed world tick must not extend the three-second cooldown.
	sess, p, deps, consume, _ := cookingFixture(t, items, 0)
	now := time.Now()
	handleCookingSelectAt(sess, p, 0, deps, 1, now)
	handleCookingSelectAt(sess, p, 0, deps, 1, now.Add(2999*time.Millisecond))
	if consume.calls != 1 {
		t.Fatal("cooldown accepted an early repeat")
	}
	handleCookingSelectAt(sess, p, 0, deps, 1, now.Add(3*time.Second))
	if consume.calls != 2 {
		t.Fatal("cooldown did not expire at three seconds")
	}
}

func TestCookingUseItemDispatchBookLevelsAndRecipe(t *testing.T) {
	items := cookingTestItems(t)
	sess, p, deps, consume, create := cookingFixture(t, items, 23)
	reg := packet.NewRegistry(zap.NewNop())
	RegisterAll(reg, deps)
	for id := int32(41255); id <= 41259; id++ {
		book := p.Inv.AddItem(id, 1, "book", 0, 0, false, 1)
		w := packet.NewWriterWithOpcode(packet.C_OPCODE_USE_ITEM)
		w.WriteD(book.ObjectID)
		w.WriteC(0)
		w.WriteC(0)
		if err := reg.Dispatch(sess, packet.StateInWorld, w.RawBytes()); err != nil {
			t.Fatal(err)
		}
		packets := drainHandlerTestPackets(sess)
		want := []byte{250, 52, 0xdb, 0x31, 0xdf, 0x02, 0x01, byte(id - 41255)}
		if len(packets) != 1 || !bytes.Equal(packets[0], want) {
			t.Fatalf("book %d menu=%x want=%x", id, packets, want)
		}
	}
	if consume.calls != 0 {
		t.Fatal("opening menus consumed materials")
	}
	book := p.Inv.FindByItemID(41257)
	w := packet.NewWriterWithOpcode(packet.C_OPCODE_USE_ITEM)
	w.WriteD(book.ObjectID)
	w.WriteC(1)
	w.WriteC(23)
	for n := 5; n < len(w.RawBytes()); n++ {
		if err := reg.Dispatch(sess, packet.StateInWorld, w.RawBytes()[:n]); err != nil {
			t.Fatal(err)
		}
	}
	if consume.calls != 0 {
		t.Fatal("truncated cook selection consumed ingredients")
	}
	if err := reg.Dispatch(sess, packet.StateInWorld, w.RawBytes()); err != nil {
		t.Fatal(err)
	}
	if consume.calls != 4 || !p.HasBuff(2999) {
		t.Fatalf("real C_USE_ITEM did not reach recipe 23: calls=%d", consume.calls)
	}
	if create.itemID != 0 && create.itemID != 49251 && create.itemID != 49259 {
		t.Fatalf("wrong recipe dispatched: %d", create.itemID)
	}
	if p.Inv.FindByObjectID(book.ObjectID) == nil {
		t.Fatal("recipe book must not be consumed")
	}
}

func TestCookingFirewoodDispatchLifetimeAndInstanceIsolation(t *testing.T) {
	items := cookingTestItems(t)
	for heading, offset := range [][2]int32{{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}} {
		t.Run(fmt.Sprint(heading), func(t *testing.T) {
			sess, p, deps, consume, _ := cookingFixture(t, items, 0)
			deps.World.RemoveGroundEffect(2001)
			// Another instance must neither block a fire nor receive its spawn.
			other := newHandlerTestSession(t, 2)
			deps.World.AddPlayer(&world.PlayerInfo{SessionID: 2, Session: other, CharID: 1002, X: p.X, Y: p.Y, MapID: p.MapID, ShowID: 8})
			deps.World.AddGroundEffect(&world.GroundEffect{ID: 2002, NpcID: 81170, GfxID: 5943, X: p.X, Y: p.Y, MapID: p.MapID, ShowID: 8, TicksLeft: 3000})
			p.Heading = int16(heading)
			wood := p.Inv.AddItem(41260, 2, "wood", 0, 0, true, 1)
			reg := packet.NewRegistry(zap.NewNop())
			RegisterAll(reg, deps)
			w := packet.NewWriterWithOpcode(packet.C_OPCODE_USE_ITEM)
			w.WriteD(wood.ObjectID)
			if err := reg.Dispatch(sess, packet.StateInWorld, w.RawBytes()); err != nil {
				t.Fatal(err)
			}
			effects := deps.World.GetNearbyGroundEffectsInShow(p.X, p.Y, p.MapID, p.ShowID)
			if len(effects) != 1 {
				t.Fatalf("fire count=%d", len(effects))
			}
			fire := effects[0]
			if fire.NpcID != 81170 || fire.GfxID != 5943 || fire.TicksLeft != 3000 || fire.X != p.X+offset[0] || fire.Y != p.Y+offset[1] {
				t.Fatalf("wrong fire: %+v", fire)
			}
			if consume.calls != 1 || wood.Count != 1 {
				t.Fatal("firewood did not consume exactly one")
			}
			if packets := drainHandlerTestPackets(other); len(packets) != 0 {
				t.Fatalf("fire leaked to another instance: %x", packets)
			}
			drainHandlerTestPackets(sess)
			if err := reg.Dispatch(sess, packet.StateInWorld, w.RawBytes()); err != nil {
				t.Fatal(err)
			}
			if consume.calls != 1 || !cookingHasMessage(drainHandlerTestPackets(sess), 1162) {
				t.Fatal("duplicate fire was not rejected")
			}
			for tick := 0; tick < 2999; tick++ {
				deps.World.TickGroundEffects()
			}
			if deps.World.GetGroundEffect(fire.ID) == nil {
				t.Fatal("fire expired before ten minutes")
			}
			deps.World.TickGroundEffects()
			if deps.World.GetGroundEffect(fire.ID) != nil {
				t.Fatal("fire remained after ten minutes")
			}
		})
	}
}

func TestCookingResultFallsToGroundWhenInventoryRejectsIt(t *testing.T) {
	items := cookingTestItems(t)
	sess, p, deps, consume, create := cookingFixture(t, items, 2)
	create.fail = true
	handleCookingSelectAt(sess, p, 2, deps, 91, time.Now())
	ground := deps.World.GetNearbyGroundItemsInShow(p.X, p.Y, p.MapID, p.ShowID)
	if consume.calls != 2 || len(ground) != 1 || ground[0].ItemID != 41287 || ground[0].Count != 1 || ground[0].ShowID != p.ShowID {
		t.Fatalf("result was lost after consumption: %+v", ground)
	}
	if !cookingHasMessage(drainHandlerTestPackets(sess), 403) {
		t.Fatal("ground result needs Java acquisition message")
	}
}
