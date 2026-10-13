package system

import (
	"time"

	coresys "github.com/l1jgo/server/internal/core/system"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

// BuffTickSystem decrements spell buff durations and item magic enchant timers
// for all online players every tick. Phase 2 (Update).
type BuffTickSystem struct {
	world *world.State
	deps  *handler.Deps
}

func NewBuffTickSystem(ws *world.State, deps *handler.Deps) *BuffTickSystem {
	return &BuffTickSystem{world: ws, deps: deps}
}

func (s *BuffTickSystem) Phase() coresys.Phase { return coresys.PhaseUpdate }

func (s *BuffTickSystem) Update(_ time.Duration) {
	s.world.AllPlayers(func(p *world.PlayerInfo) {
		// Track buff count to detect expirations (dirty flag for persistence).
		prevBuffCount := len(p.ActiveBuffs)
		handler.TickPlayerBuffs(p, s.deps)
		tickItemMagicEnchants(p, s.deps)
		// 交易、丟棄、銷毀光源後，同一 tick 重新同步人物光圈。
		handler.UpdatePlayerLight(p, s.world)
		TickPlayerPoison(p, s.deps)
		TickPlayerCurse(p, s.deps)
		TickCatapultSilence(p)
		if len(p.ActiveBuffs) < prevBuffCount {
			p.Dirty = true
		}
	})
}

// tickItemMagicEnchants 遞減裝備魔法附魔計時器。
// 到期時歸零並重新計算裝備屬性。
func tickItemMagicEnchants(p *world.PlayerInfo, deps *handler.Deps) {
	equippedChanged := false
	if p.Inv == nil {
		return
	}
	for _, item := range p.Inv.Items {
		if item == nil {
			continue
		}
		changed := tickItemMagicEnchant(item)
		if changed && item.Equipped {
			equippedChanged = true
		}
	}

	if equippedChanged && p.Session != nil && deps.Equip != nil {
		deps.Equip.RecalcEquipStats(p.Session, p)
	}
}

func tickItemMagicEnchant(item *world.InvItem) bool {
	changed := false
	if item.DmgMagicExpiry > 0 {
		item.DmgMagicExpiry--
		if item.DmgMagicExpiry <= 0 {
			item.DmgByMagic = 0
			item.HitByMagic = 0
			item.DmgMagicExpiry = 0
			changed = true
		}
	}
	if item.AcMagicExpiry > 0 {
		item.AcMagicExpiry--
		if item.AcMagicExpiry <= 0 {
			item.AcByMagic = 0
			item.AcMagicExpiry = 0
			changed = true
		}
	}
	return changed
}
