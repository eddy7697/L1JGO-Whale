package system

import (
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

const (
	skillElementalFallDown = int32(133)
	skillCounterMirror     = int32(134)
	skillAreaOfSilence     = int32(161)
	skillWaterLife         = int32(170)
	skillElementalFire     = int32(171)
	skillPolluteWater      = int32(173)
	skillStrikerGale       = int32(174)
	skillSoulOfFlame       = int32(175)

	elementalFallDownDelta = int16(-50)
	elementalFireChance    = 33
)

func (s *SkillSystem) applyElementalFallDownToPlayer(caster, target *world.PlayerInfo, skill *data.SkillInfo) {
	if caster == nil || target == nil || skill == nil || caster.ElfAttr == 0 {
		return
	}
	buff := &world.ActiveBuff{
		SkillID:   skillElementalFallDown,
		TicksLeft: skill.BuffDuration * 5,
	}
	old := target.RemoveBuff(skillElementalFallDown)
	if old != nil {
		s.revertBuffStats(target, old)
	}
	setElementalResDelta(caster.ElfAttr, buff, elementalFallDownDelta)
	target.FireRes += buff.DeltaFireRes
	target.WaterRes += buff.DeltaWaterRes
	target.WindRes += buff.DeltaWindRes
	target.EarthRes += buff.DeltaEarthRes
	target.AddBuff(buff)
}

func applyElementalFallDownToNpc(caster *world.PlayerInfo, npc *world.NpcInfo, durationSec int) {
	if caster == nil || npc == nil || caster.ElfAttr == 0 {
		return
	}
	removeElementalFallDownFromNpc(npc)
	switch caster.ElfAttr {
	case 1:
		npc.EarthRes += elementalFallDownDelta
	case 2:
		npc.FireRes += elementalFallDownDelta
	case 4:
		npc.WaterRes += elementalFallDownDelta
	case 8:
		npc.WindRes += elementalFallDownDelta
	default:
		return
	}
	npc.ElementalFallDownAttr = caster.ElfAttr
	npc.AddDebuff(skillElementalFallDown, durationSec*5)
}

func removeElementalFallDownFromNpc(npc *world.NpcInfo) {
	if npc == nil || npc.ElementalFallDownAttr == 0 {
		return
	}
	switch npc.ElementalFallDownAttr {
	case 1:
		npc.EarthRes -= elementalFallDownDelta
	case 2:
		npc.FireRes -= elementalFallDownDelta
	case 4:
		npc.WaterRes -= elementalFallDownDelta
	case 8:
		npc.WindRes -= elementalFallDownDelta
	}
	npc.ElementalFallDownAttr = 0
}

func setElementalResDelta(attr int16, buff *world.ActiveBuff, delta int16) {
	if buff == nil {
		return
	}
	switch attr {
	case 1:
		buff.DeltaEarthRes = delta
	case 2:
		buff.DeltaFireRes = delta
	case 4:
		buff.DeltaWaterRes = delta
	case 8:
		buff.DeltaWindRes = delta
	}
}

func (s *SkillSystem) applyElfWaterHealingModifiers(target *world.PlayerInfo, heal int32) int32 {
	if target == nil || heal <= 0 {
		return heal
	}
	if target.HasBuff(skillWaterLife) {
		heal <<= 1
		s.removeBuffAndRevert(target, skillWaterLife)
	}
	if target.HasBuff(skillPolluteWater) {
		heal >>= 1
	}
	if target.HasBuff(mobSkillPolluteWater) {
		heal >>= 1
	}
	if target.HasBuff(mobSkillHealTurnToDamage) {
		heal *= -1
	}
	return heal
}

func applyPlayerHealDeltaLikeJava(deps *handler.Deps, target *world.PlayerInfo, heal int32) bool {
	if target == nil || heal == 0 {
		return false
	}
	if heal > 0 {
		if target.HP >= target.MaxHP {
			return false
		}
		target.HP += heal
		if target.HP > target.MaxHP {
			target.HP = target.MaxHP
		}
		sendHpUpdate(target.Session, target)
		return true
	}

	target.HP += heal
	if target.HP <= 0 {
		target.HP = 0
		if deps != nil && deps.Death != nil {
			deps.Death.KillPlayer(target)
			return true
		}
	}
	sendHpUpdate(target.Session, target)
	return true
}

func (s *SkillSystem) applyAreaOfSilence(caster *world.PlayerInfo, skill *data.SkillInfo, nearby []*world.PlayerInfo) {
	if caster == nil || skill == nil {
		return
	}
	duration := skill.BuffDuration
	if duration <= 0 {
		duration = 16
	}
	for _, target := range nearby {
		if target == nil || target.CharID == caster.CharID || target.Dead {
			continue
		}
		if playerDebuffSkills[skillAreaOfSilence] && !s.checkPlayerMRResist(caster, target) {
			continue
		}
		old := target.RemoveBuff(skillAreaOfSilence)
		if old != nil {
			s.revertBuffStats(target, old)
		}
		buff := &world.ActiveBuff{
			SkillID:     skillAreaOfSilence,
			TicksLeft:   duration * 5,
			SetSilenced: true,
		}
		target.Silenced = true
		target.AddBuff(buff)
		s.sendBuffIcon(target, skillAreaOfSilence, uint16(duration))
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(target.CharID, skill.CastGfx))
		}
		if skill.SysMsgHappen > 0 {
			handler.SendServerMessage(target.Session, uint16(skill.SysMsgHappen))
		}
	}
}

func (s *SkillSystem) applyCounterMirrorMagicDamage(attacker, target *world.PlayerInfo, damage int32, roll int, nearby []*world.PlayerInfo) int32 {
	if attacker == nil || target == nil || damage <= 0 || !target.HasBuff(skillCounterMirror) {
		return damage
	}
	if int(target.Wis) <= roll {
		return damage
	}
	s.removeBuffAndRevert(target, skillCounterMirror)
	if attacker.HP > 0 {
		attacker.ApplyDamage(damage)
		attacker.Dirty = true
		if attacker.HP < 0 {
			attacker.HP = 0
		}
		sendHpUpdate(attacker.Session, attacker)
	}
	if len(nearby) > 0 {
		handler.BroadcastToPlayers(nearby, handler.BuildActionGfx(attacker.CharID, 2))
		handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(target.CharID, 4395))
	}
	if attacker.HP <= 0 && s.deps != nil && s.deps.Death != nil {
		s.deps.Death.KillPlayer(attacker)
	}
	return 0
}

func elfMeleeDamage(attacker *world.PlayerInfo, damage int32, weaponType string) int32 {
	return elfMeleeDamageWithRoll(attacker, damage, weaponType, world.RandInt(100))
}

func elfMeleeDamageWithRoll(attacker *world.PlayerInfo, damage int32, weaponType string, elementalFireRoll int) int32 {
	if attacker == nil || damage <= 0 || isRangedWeaponType(weaponType) {
		return damage
	}
	if attacker.HasBuff(skillSoulOfFlame) {
		// Java `L1AttackPc.java:1455-1457 / 1945-1947`：近戰非暴擊時
		// `_weaponDamage = weaponMaxDamage * SOUL_OF_FLAME_DAMAGE`，yiwei 配置 1.5。
		// Go 簡化：以 1.5x 套用於已計算的傷害（weaponMax 取代屬於 broader 武器傷害管線缺口）。
		damage = damage * 3 / 2
	}
	if attacker.HasBuff(skillElementalFire) && elementalFireRoll < elementalFireChance {
		damage = damage * 3 / 2
	}
	return damage
}

func strikerGaleRangedDamage(target *world.PlayerInfo, damage int32) int32 {
	if target == nil || damage <= 0 || !target.HasBuff(skillStrikerGale) {
		return damage
	}
	return damage * 11 / 10
}

func strikerGaleRangedDamageToNpc(npc *world.NpcInfo, damage int32) int32 {
	if npc == nil || damage <= 0 || !npc.HasDebuff(skillStrikerGale) {
		return damage
	}
	return damage * 11 / 10
}

func isRangedWeaponType(weaponType string) bool {
	return weaponType == "bow" || weaponType == "gauntlet"
}
