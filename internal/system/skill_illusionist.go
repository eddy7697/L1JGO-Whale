package system

import (
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

const (
	skillConfusion  = int32(202)
	skillMindBreak  = int32(207)
	skillArmBreaker = int32(213)

	skillSilence = int32(64)
)

func calcMindBreakDamage(caster *world.PlayerInfo) int32 {
	if caster == nil {
		return 0
	}
	return int32(float64(calcPlayerSPLikeJava(caster)) * 3.8)
}

func applyMindBreakMPDrain(target *world.PlayerInfo) {
	if target == nil {
		return
	}
	target.SpendMP(5)
	if target.MP < 0 {
		target.MP = 0
	}
	sendMpUpdate(target.Session, target)
}

func (s *SkillSystem) applyIllusionistStatusAttackEffect(target *world.PlayerInfo, skill *data.SkillInfo) {
	if target == nil || skill == nil || target.Dead {
		return
	}
	switch skill.SkillID {
	case skillConfusion:
		s.applyConfusionSilence(target, skill)
	case skillArmBreaker:
		s.revealInvisibleTarget(target)
	}
}

func (s *SkillSystem) applyConfusionSilence(target *world.PlayerInfo, skill *data.SkillInfo) {
	if target.Silenced || target.HasBuff(skillSilence) {
		target.Silenced = true
		return
	}
	dur := skill.BuffDuration
	if dur <= 0 {
		dur = 8
	}
	buff := &world.ActiveBuff{
		SkillID:     skillSilence,
		TicksLeft:   dur * 5,
		SetSilenced: true,
	}
	old := target.AddBuff(buff)
	if old != nil {
		s.revertBuffStats(target, old)
	}
	target.Silenced = true
}

func (s *SkillSystem) revealInvisibleTarget(target *world.PlayerInfo) {
	if !target.Invisible && !target.HasBuff(60) && !target.HasBuff(97) {
		return
	}
	s.removeBuffAndRevert(target, 60)
	s.removeBuffAndRevert(target, 97)
	target.Invisible = false
	handler.SendInvisible(target.Session, target.CharID, target.IsInvisible())

	s.sendPlayerReappearToVisible(target)
}

const (
	skillBoneBreak = int32(208)
	skillJoyOfPain = int32(218)

	joyOfPainTicks   = 16 * 5
	joyOfPainDivisor = 5
	joyOfPainMaxDmg  = 1000

	// Java `L1MagicPc:584-599` + `ConfigIllusionstSkill` 預設值
	// BONE_BREAK_1=5（caster>target）、_2=10（equal）、_3=15（caster<target）。
	// 注意 Java 配置反常識：低等目標基礎機率最低。
	// BONE_BREAK_INT / BONE_BREAK_MR 預設皆 0，所以 INT/MR 對機率沒有影響；PC→PC
	// 末段再 `-= target.RegistStun`（L1MagicPc:958-961）。
	boneBreakHigherLevelProb = 5  // caster.Level > target.Level
	boneBreakEqualLevelProb  = 10 // caster.Level == target.Level
	boneBreakLowerLevelProb  = 15 // caster.Level < target.Level
)

func (s *SkillSystem) applyIllusionistControlAttackEffect(caster, target *world.PlayerInfo, skill *data.SkillInfo) {
	if target == nil || skill == nil || target.Dead {
		return
	}
	if skill.SkillID == skillBoneBreak && checkBoneBreakPlayerSuccess(caster, target) {
		s.applyBoneBreakParalysis(target)
	}
}

func checkBoneBreakPlayerSuccess(caster, target *world.PlayerInfo) bool {
	return world.RandInt(100) < calcBoneBreakPlayerProbability(caster, target)
}

// calcBoneBreakPlayerProbability 對齊 Java `L1MagicPc.calcProbabilityMagic` `case BONE_BREAK`
// （L1MagicPc:584-599）+ PC→PC `case BONE_BREAK: probability -= RegistStun`（L1MagicPc:958-961）。
// 使用 ConfigIllusionstSkill 預設值（_1=5、_2=10、_3=15、_INT=0、_MR=0）。
func calcBoneBreakPlayerProbability(caster, target *world.PlayerInfo) int {
	if caster == nil || target == nil {
		return 0
	}
	probability := boneBreakEqualLevelProb
	if caster.Level > target.Level {
		probability = boneBreakHigherLevelProb
	} else if caster.Level < target.Level {
		probability = boneBreakLowerLevelProb
	}
	probability -= int(target.RegistStun)
	if probability < 0 {
		return 0
	}
	if probability > 100 {
		return 100
	}
	return probability
}

func (s *SkillSystem) applyBoneBreakParalysis(target *world.PlayerInfo) {
	// Java `BONE_BREAK.start():24` 只在「目標未持 208 buff 且 isProbability」時生效。
	// Go 額外守衛 `target.Paralyzed` 避免覆蓋其他更高優先狀態（與 192/87 一致）。
	if target.Paralyzed || target.HasBuff(skillBoneBreak) {
		return
	}
	durTicks := (world.RandInt(2) + 1) * 5
	buff := &world.ActiveBuff{
		SkillID:      skillBoneBreak,
		TicksLeft:    durTicks,
		SetParalyzed: true,
	}
	old := target.AddBuff(buff)
	if old != nil {
		s.revertBuffStats(target, old)
	}
	target.Paralyzed = true
	// Java `BONE_BREAK.start():29` 對 PC 送 `S_Paralysis(5, true)` —— TYPE_STUN (5) 對應
	// wire byte 0x16（StunApply），並非 ParalysisApply(0x02)。S_Paralysis.java:79-85 切換顯示
	// 「衝擊之暈」效果而非「身體完全麻痺」訊息。
	handler.SendParalysis(target.Session, handler.StunApply)
	nearby := s.deps.World.GetNearbyPlayersInShow(target.X, target.Y, target.MapID, 0, target.ShowID)
	handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(target.CharID, 13119))
}

func (s *SkillSystem) applyJoyOfPainReady(caster *world.PlayerInfo) {
	if caster == nil {
		return
	}
	if caster.HasBuff(skillJoyOfPain) {
		handler.SendSystemMessage(caster.Session, "已經準備疼痛的歡愉。")
		return
	}
	buff := &world.ActiveBuff{
		SkillID:   skillJoyOfPain,
		TicksLeft: joyOfPainTicks,
	}
	old := caster.AddBuff(buff)
	if old != nil {
		s.revertBuffStats(caster, old)
	}
}

// ApplyJoyOfPainBacklash 對外提供疼痛的歡愉反傷觸發點（SkillManager 介面）。
// 供 pvp.go 等通用傷害路徑共用，與 skill 路徑（skill_damage.go/skill_self_area.go）走同一實作。
func (s *SkillSystem) ApplyJoyOfPainBacklash(attacker, target *world.PlayerInfo, nearby []*world.PlayerInfo) {
	s.applyJoyOfPainBacklash(attacker, target, nearby)
}

func (s *SkillSystem) applyJoyOfPainBacklash(attacker, target *world.PlayerInfo, nearby []*world.PlayerInfo) {
	if attacker == nil || target == nil || attacker.CharID == target.CharID || !attacker.HasBuff(skillJoyOfPain) {
		return
	}
	damage := (target.MaxHP - target.HP) / joyOfPainDivisor
	if damage <= 0 {
		return
	}
	if damage > joyOfPainMaxDmg {
		damage = joyOfPainMaxDmg
	}
	if attacker.HP-damage <= 0 {
		attacker.HP = 1
	} else {
		attacker.ApplyDamage(damage)
	}
	attacker.Dirty = true
	s.removeBuffAndRevert(attacker, skillJoyOfPain)
	handler.SendWeightUpdate(attacker.Session, attacker)
	sendHpUpdate(attacker.Session, attacker)
	handler.BroadcastToPlayers(nearby, handler.BuildActionGfx(attacker.CharID, 2))
}
