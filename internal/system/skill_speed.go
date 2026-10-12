package system

import "github.com/l1jgo/server/internal/world"

// Explicit removal and natural expiry must both update server speed and S_SkillHaste/Brave.
// Retain permanent equipment haste and any still-active effect when removing an old timer.
func (s *SkillSystem) restoreSpeedAfterBuffRemoved(p *world.PlayerInfo, old *world.ActiveBuff) {
	if old.SetMoveSpeed > 0 {
		p.MoveSpeed, p.HasteTicks = 0, 0
		if p.HasteItemEquipped > 0 {
			p.MoveSpeed = 1
		} else {
			for _, b := range p.ActiveBuffs {
				if b.SetMoveSpeed > 0 && (b.TicksLeft > p.HasteTicks || b.TicksLeft == 0) {
					p.MoveSpeed, p.HasteTicks = b.SetMoveSpeed, b.TicksLeft
				}
			}
		}
		s.sendSpeedToAll(p, p.MoveSpeed, uint16(p.HasteTicks/5))
	}
	if old.SetBraveSpeed > 0 {
		p.BraveSpeed, p.BraveTicks = 0, 0
		for _, b := range p.ActiveBuffs {
			if b.SetBraveSpeed > 0 && (b.TicksLeft > p.BraveTicks || b.TicksLeft == 0) {
				p.BraveSpeed, p.BraveTicks = b.SetBraveSpeed, b.TicksLeft
			}
		}
		s.sendBraveToAll(p, p.BraveSpeed, uint16(p.BraveTicks/5))
	}
}
