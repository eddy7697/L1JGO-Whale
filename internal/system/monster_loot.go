package system

import (
	"fmt"
	"sort"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

func (s *ItemUseSystem) collectLootAttackers(killer *world.PlayerInfo, npc *world.NpcInfo) []*world.PlayerInfo {
	var result []*world.PlayerInfo
	for sid, hate := range npc.HateList {
		p := s.deps.World.GetBySession(sid)
		if hate > 0 && p != nil && !p.Dead && p.MapID == npc.MapID && p.ShowID == npc.ShowID && chebyshevDist(p.X, p.Y, npc.X, npc.Y) <= lootingRange {
			result = append(result, p)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CharID < result[j].CharID })
	if len(npc.HateList) == 0 && killer != nil && !killer.Dead && killer.MapID == npc.MapID && killer.ShowID == npc.ShowID && chebyshevDist(killer.X, killer.Y, npc.X, npc.Y) <= lootingRange {
		result = append(result, killer)
	}
	return result
}

func (s *ItemUseSystem) canReceiveMonsterDrop(p *world.PlayerInfo, drop data.DropItem, qty int32) bool {
	info := s.deps.Items.Get(drop.ItemID)
	if info == nil || p == nil || p.Inv == nil || qty <= 0 {
		return false
	}
	existing := p.Inv.FindByItemID(drop.ItemID)
	stackable := info.Stackable || drop.ItemID == world.AdenaItemID
	if existing != nil && int64(existing.Count)+int64(qty) > 2000000000 {
		return false
	}
	slots := int(qty)
	if stackable {
		slots = 1
		if existing != nil {
			slots = 0
		}
	}
	if p.Inv.Size()+slots > world.MaxInventorySize {
		return false
	}
	weight := int64(info.Weight) * int64(qty)
	return weight <= 2147483647 && !p.Inv.IsOverWeight(int32(weight), world.PlayerMaxWeight(p))
}

// Eddy DropTable.dropShare: a full receiver's loot falls at their feet. Auto-share
// splits a sufficiently large stack across living same-map party members.
func (s *ItemUseSystem) distributeMonsterDrop(receiver *world.PlayerInfo, npc *world.NpcInfo, drop data.DropItem, qty int32) {
	if !s.canReceiveMonsterDrop(receiver, drop, qty) {
		s.dropMonsterLootOnGround(receiver, npc, drop, qty)
		return
	}
	party := s.deps.World.Parties.GetParty(receiver.CharID)
	var members []*world.PlayerInfo
	if party != nil && party.PartyType == world.PartyTypeAutoShare {
		for _, id := range party.Members {
			p := s.deps.World.GetByCharID(id)
			if p != nil && !p.Dead && p.HP > 0 && p.MapID == npc.MapID && p.ShowID == npc.ShowID {
				members = append(members, p)
			}
		}
	}
	if len(members) > 1 && qty >= int32(len(members)) {
		share, remainder := qty/int32(len(members)), qty%int32(len(members))
		for i, p := range members {
			count := share
			if i == 0 {
				count += remainder
			}
			s.awardMonsterDrop(p, npc, drop, count)
		}
		return
	}
	s.awardMonsterDrop(receiver, npc, drop, qty)
}

func (s *ItemUseSystem) awardMonsterDrop(p *world.PlayerInfo, npc *world.NpcInfo, drop data.DropItem, qty int32) {
	if !s.canReceiveMonsterDrop(p, drop, qty) || !s.giveDropToPlayer(p, drop, qty) {
		s.dropMonsterLootOnGround(p, npc, drop, qty)
		return
	}
	info := s.deps.Items.Get(drop.ItemID)
	name := info.Name
	p.Dirty = true
	if drop.EnchantLevel != 0 {
		name = fmt.Sprintf("%+d %s", drop.EnchantLevel, name)
	}
	if qty > 1 {
		name = fmt.Sprintf("%s (%d)", name, qty)
	}
	party := s.deps.World.Parties.GetParty(p.CharID)
	if party == nil {
		handler.SendServerMessageArgs(p.Session, 143, npc.Name, name)
		return
	}
	for _, id := range party.Members {
		if member := s.deps.World.GetByCharID(id); member != nil && member.Session != nil {
			handler.SendServerMessageArgs(member.Session, 813, npc.Name, name, p.Name)
		}
	}
}

func (s *ItemUseSystem) dropMonsterLootOnGround(p *world.PlayerInfo, npc *world.NpcInfo, drop data.DropItem, qty int32) {
	info := s.deps.Items.Get(drop.ItemID)
	if info == nil || qty <= 0 {
		return
	}
	if !info.Stackable && drop.ItemID != world.AdenaItemID && qty > 1 {
		for i := int32(0); i < qty; i++ {
			s.dropMonsterLootOnGround(p, npc, drop, 1)
		}
		return
	}
	x, y := npc.X, npc.Y
	if p != nil {
		x, y = p.X, p.Y
	}
	item := &world.InvItem{
		ItemID: drop.ItemID, Count: qty, Name: info.Name,
		InvGfx: info.InvGfx, Weight: info.Weight,
		Stackable:  info.Stackable || drop.ItemID == world.AdenaItemID,
		EnchantLvl: int8(drop.EnchantLevel), Bless: byte(info.Bless),
		Identified: info.Category == data.CategoryEtcItem,
	}
	applyItemTemplate(item, info)
	g := &world.GroundItem{
		ID: world.NextGroundItemID(), ItemID: drop.ItemID, Count: qty,
		EnchantLvl: int8(drop.EnchantLevel), Item: item,
		Name: info.Name, GrdGfx: info.GrdGfx,
		X: x, Y: y, MapID: npc.MapID, ShowID: npc.ShowID,
		TTL: 5 * 60 * 5,
	}
	s.deps.World.AddGroundItem(g)
	for _, viewer := range s.deps.World.GetNearbyPlayersInShow(x, y, g.MapID, 0, g.ShowID) {
		handler.SendDropItem(viewer.Session, g)
		if viewer.Known != nil {
			viewer.Known.GroundItems[g.ID] = world.KnownPos{X: x, Y: y}
		}
	}
}
