package system

import (
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/world"
)

// Eddy L1Magic.calcProbability: no minimum success floor. Status resistances
// are deducted before applying the 90% cap in calcProbabilityMagic.
func javaDebuffProbability(skill *data.SkillInfo, attackLevel, defenseLevel, magicLevel, magicBonus, classDiceAdjustment, originalHit, mr, leverage int, roll func(int) int) int {
	if skill == nil {
		return 0
	}
	id := skill.SkillID
	prob := 0
	switch id {
	case 133, 145, 152, 153, 161, 167, 174, 173, 157:
		return int(float64(skill.ProbabilityDice)/10*float64(attackLevel-defenseLevel)) + skill.ProbabilityValue + 2*originalHit
	case 87:
		return skill.ProbabilityValue + 2*(attackLevel-defenseLevel) + 2*originalHit
	case 91:
		return skill.ProbabilityValue + attackLevel - defenseLevel + 2*originalHit
	}
	count := magicLevel + magicBonus
	if id != 183 && id != 188 && id != 193 {
		count += classDiceAdjustment
	}
	if skill.ProbabilityDice <= 0 {
		return 0
	}
	for i := 0; i < max(1, count); i++ {
		prob += roll(skill.ProbabilityDice) + 1
		if id == 183 || id == 188 || id == 193 {
			prob += skill.ProbabilityValue
		}
	}
	prob = prob*leverage/10 + 2*originalHit
	if id == 183 || id == 188 || id == 193 {
		if prob >= mr {
			return 100
		}
		return 0
	}
	return prob - mr
}

func javaPlayerDebuffProbability(p *world.PlayerInfo, defenseLevel, mr int, skill *data.SkillInfo, roll func(int) int) int {
	adjustment := -1
	if p.ClassType == 3 {
		adjustment = 1
	}
	return javaDebuffProbability(skill, int(p.Level), defenseLevel, eddyDebuffMagicLevel(int(p.ClassType), int(p.Level)), eddyDebuffMagicBonus(int(p.Intel)), adjustment, int(p.OriginalMagicHit), mr, 10, roll)
}

// The Eddy reference has different wizard/illusionist caps from the existing
// damage formulas. Keep those unrelated combat formulas outside this change.
func eddyDebuffMagicLevel(classType, level int) int {
	switch classType {
	case 3:
		return min(10, max(0, level)/4)
	case 6:
		return min(6, max(0, level)/8)
	default:
		return calcMagicLevel(classType, level)
	}
}

func eddyDebuffMagicBonus(intel int) int {
	if intel > 50 {
		return intel - 25
	}
	return calcMagicBonusLikeJava(intel)
}

func javaApplyStatusResistance(prob int, skillID int32, target *world.PlayerInfo) int {
	switch skillID {
	case 157:
		prob -= int(target.RegistSustain)
	case 87:
		prob -= 2 * int(target.RegistStun)
	case 33:
		prob -= int(target.RegistStone)
	case 66:
		prob -= int(target.RegistSleep)
	case 50, 80, 194, 15003, 15004:
		for _, id := range []int32{78, 50, 80, 194, 157, 15003, 15004} {
			if target.HasBuff(id) {
				return 0
			}
		}
		prob -= int(target.RegistFreeze)
	case 20, 40, 103:
		prob -= int(target.RegistBlind)
	}
	return prob
}

// General monster spell casting uses the same Java probability calculation.
// Scripted area attacks call ApplyNpcDebuff directly because their trigger owns success.
func (s *SkillSystem) ApplyNpcDebuffFrom(npc *world.NpcInfo, target *world.PlayerInfo, skill *data.SkillInfo, leverage int) {
	if skill.Type&5 != 0 && skill.SkillID != 44 {
		if target.HasBuff(157) && skill.SkillID != 27 {
			return
		}
		if leverage <= 0 {
			leverage = 10
		}
		prob := javaDebuffProbability(skill, int(npc.Level), int(target.Level), int(npc.Level)/4, eddyDebuffMagicBonus(int(npc.Intel)), 0, 0, int(target.MR), leverage, world.RandInt)
		prob = javaApplyStatusResistance(prob, skill.SkillID, target)
		if world.RandInt(100) >= min(90, prob) {
			return
		}
	}
	if s.tryCounterMagic(target, skill.SkillID) {
		return
	}
	s.ApplyNpcDebuff(target, skill)
}
