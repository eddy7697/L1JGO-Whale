package handler

import (
	"time"

	"github.com/l1jgo/server/internal/persist"
	"github.com/l1jgo/server/internal/world"
)

// BuffRowsFromPlayer converts a player's active buffs into persist.BuffRow slice for DB storage.
// Exported so system/input.go can call it on disconnect.
func BuffRowsFromPlayer(p *world.PlayerInfo) []persist.BuffRow {
	if len(p.ActiveBuffs) == 0 {
		return nil
	}

	rows := make([]persist.BuffRow, 0, len(p.ActiveBuffs))
	now := time.Now()
	for _, buff := range p.ActiveBuffs {
		// Skip state-only buffs that shouldn't persist across login
		if buff.SetInvisible || buff.SetParalyzed || buff.SetSleeped {
			continue
		}

		remainSec := buff.TicksLeft / 5 // ticks → seconds
		if !buff.ExpiresAt.IsZero() {
			remaining := buff.ExpiresAt.Sub(now)
			if remaining <= 0 {
				continue
			}
			remainSec = min(remainSec, int((remaining+time.Second-1)/time.Second))
		}
		if remainSec <= 0 {
			continue // expired
		}

		row := persist.BuffRow{
			CharID:        p.CharID,
			SkillID:       buff.SkillID,
			RemainingTime: remainSec,
			DeltaAC:       buff.DeltaAC,
			DeltaStr:      buff.DeltaStr,
			DeltaDex:      buff.DeltaDex,
			DeltaCon:      buff.DeltaCon,
			DeltaWis:      buff.DeltaWis,
			DeltaIntel:    buff.DeltaIntel,
			DeltaCha:      buff.DeltaCha,
			DeltaMaxHP:    buff.DeltaMaxHP,
			DeltaMaxMP:    buff.DeltaMaxMP,
			DeltaHitMod:   buff.DeltaHitMod,
			DeltaDmgMod:   buff.DeltaDmgMod,
			DeltaSP:       buff.DeltaSP,
			DeltaMR:       buff.DeltaMR,
			DeltaHPR:      buff.DeltaHPR,
			DeltaMPR:      buff.DeltaMPR,
			DeltaBowHit:   buff.DeltaBowHit,
			DeltaBowDmg:   buff.DeltaBowDmg,
			DeltaFireRes:  buff.DeltaFireRes,
			DeltaWaterRes: buff.DeltaWaterRes,
			DeltaWindRes:  buff.DeltaWindRes,
			DeltaEarthRes: buff.DeltaEarthRes,
			DeltaDodge:    buff.DeltaDodge,
			SetMoveSpeed:  buff.SetMoveSpeed,
			SetBraveSpeed: buff.SetBraveSpeed,
		}

		// Store polymorph ID for Shape Change buff
		if buff.SkillID == SkillShapeChange {
			row.PolyID = p.PolyID
		}

		rows = append(rows, row)
	}
	return rows
}
