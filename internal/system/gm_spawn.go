package system

import (
	"fmt"

	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

// SpawnGMNPC shares the normal NPC builder, preserving poison, family, light,
// attack timing and instance visibility rather than constructing partial NPCs.
func (s *GMCommandSystem) SpawnGMNPC(p *world.PlayerInfo, id int32, count, radius int) (int, error) {
	if p == nil || s.deps.Npcs == nil || count < 1 || count > 500 || radius < 0 || radius > 100 {
		return 0, fmt.Errorf("無效的召怪參數")
	}
	tmpl := s.deps.Npcs.Get(id)
	if tmpl == nil {
		return 0, fmt.Errorf("找不到 NPC 模板 %d", id)
	}
	spawned := 0
	for i := 0; i < count; i++ {
		x, y := p.X, p.Y
		if radius > 0 {
			found := false
			for attempt := 0; attempt < 20; attempt++ {
				x = p.X + int32(world.RandInt(radius*2+1)-radius)
				y = p.Y + int32(world.RandInt(radius*2+1)-radius)
				if s.deps.MapData == nil || s.deps.MapData.IsPassablePoint(p.MapID, x, y) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		npc := newRuntimeNpcFromTemplate(tmpl, x, y, p.MapID, p.Heading, p.ShowID, 0, s.deps.SprTable)
		s.deps.World.AddNpc(npc)
		for _, viewer := range s.deps.World.GetNearbyPlayersInShow(x, y, p.MapID, 0, p.ShowID) {
			handler.SendNpcPack(viewer.Session, npc)
		}
		spawned++
	}
	return spawned, nil
}
