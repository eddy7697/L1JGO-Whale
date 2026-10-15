package system

import (
	"context"
	"time"

	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// ChangeElfAttribute implements C_NPCAction fire/water/air/earth/init.
// A player must remove the old element before choosing a new one.
func (s *SkillSystem) ChangeElfAttribute(sess *net.Session, player *world.PlayerInfo, npcID int32, action string) {
	if player == nil || player.ClassType != 2 || player.Dead {
		return
	}
	var attr int16
	var message byte
	switch action {
	case "fire":
		attr, message = 2, 1
	case "water":
		attr, message = 4, 2
	case "air":
		attr, message = 8, 3
	case "earth":
		attr, message = 1, 4
	case "init":
	default:
		return
	}
	reset := action == "init"
	if reset && player.ElfAttr == 0 || !reset && player.ElfAttr != 0 {
		return
	}
	spells := player.KnownSpells
	if reset {
		if s.deps.Skills == nil {
			return
		}
		spells = make([]int32, 0, len(player.KnownSpells))
		for _, id := range player.KnownSpells {
			if id >= 129 && id <= 176 {
				skill := s.deps.Skills.Get(id)
				if skill != nil && skill.Attr != 0 {
					continue
				}
			}
			spells = append(spells, id)
		}
	}
	// Persist the attribute and retained spell list together before acknowledging
	// the change, so a failed save cannot leave a half-reset character on relog.
	if s.deps.CharRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := s.deps.CharRepo.SaveElfAttribute(ctx, player.Name, attr, spells)
		cancel()
		if err != nil {
			s.deps.Log.Error("儲存精靈屬性失敗", zap.String("name", player.Name), zap.Error(err))
			return
		}
	}
	if reset {
		s.removeBuffAndRevert(player, 147) // ELEMENTAL_PROTECTION uses the old element.
		player.KnownSpells = spells
		sess.Send(handler.BuildDeleteElfAttributeSkills())
	}
	player.ElfAttr = attr
	player.Dirty = true
	if reset {
		handler.SendServerMessage(sess, 678)
	} else {
		sess.Send(handler.BuildElfAttributeMessage(message))
	}
	handler.SendHypertext(sess, npcID, "")
}
