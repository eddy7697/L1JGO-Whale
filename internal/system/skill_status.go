package system

import (
	"fmt"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
)

const (
	skillCurseBlindEffect  = int32(40)
	skillStatusFloatingEye = int32(1012)
	skillStatusHaste       = int32(1001)
)

const (
	weakElementalEarth = int16(1)
	weakElementalFire  = int16(2)
	weakElementalWater = int16(4)
	weakElementalWind  = int16(8)
)

func (s *SkillSystem) applyCurseBlindEffect(target *world.PlayerInfo, skill *data.SkillInfo) {
	if target == nil || skill == nil {
		return
	}
	s.removeBuffAndRevert(target, skillCurseBlindEffect)
	dur := skill.BuffDuration
	if dur <= 0 {
		dur = 8
	}
	target.AddBuff(&world.ActiveBuff{
		SkillID:   skillCurseBlindEffect,
		TicksLeft: dur * 5,
	})
	blindType := uint16(1)
	if target.HasBuff(skillStatusFloatingEye) {
		blindType = 2
	}
	handler.SendCurseBlind(target.Session, blindType)
}

func (s *SkillSystem) removeCurseBlindEffect(target *world.PlayerInfo) {
	if target == nil {
		return
	}
	s.removeBuffAndRevert(target, skillCurseBlindEffect)
	handler.SendCurseBlind(target.Session, 0)
}

func (s *SkillSystem) handleOppositeMoveSpeedSkill(target *world.PlayerInfo, skillID int32) bool {
	if target == nil {
		return false
	}
	switch skillID {
	case 43, 54:
		if target.MoveSpeed != 2 {
			return false
		}
		s.removeMoveSpeedBuffs(target, []int32{29, 76, 152})
		s.sendYiweiPostCastStatusRefresh(target)
		return true
	case 29, 76, 152:
		if target.MoveSpeed != 1 {
			return false
		}
		s.removeMoveSpeedBuffs(target, []int32{43, 54, skillStatusHaste})
		s.sendYiweiPostCastStatusRefresh(target)
		return true
	default:
		return false
	}
}

func slowSkillTargetBlockedLikeYiwei(target *world.PlayerInfo, skillID int32) bool {
	if target == nil {
		return false
	}
	switch skillID {
	case 29, 76, 152:
		// Java L1SkillUse 在 SLOW/MASS_SLOW/ENTANGLE 對急速道具與 BraveSpeed=5 目標直接 continue。
		return target.HasteItemEquipped > 0 || target.BraveSpeed == 5
	default:
		return false
	}
}

func hasteSkillTargetBlockedLikeYiwei(target *world.PlayerInfo, skillID int32) bool {
	if target == nil {
		return false
	}
	switch skillID {
	case 43, 54:
		return target.HasteItemEquipped > 0
	default:
		return false
	}
}

func (s *SkillSystem) removeMoveSpeedBuffs(target *world.PlayerInfo, skillIDs []int32) {
	for _, skillID := range skillIDs {
		s.removeBuffAndRevert(target, skillID)
	}
	target.MoveSpeed = 0
	target.HasteTicks = 0
	s.sendSpeedToAll(target, 0, 0)
}

func (s *SkillSystem) applyShockStunToPlayer(sess *net.Session, caster, target *world.PlayerInfo, skill *data.SkillInfo, nearby []*world.PlayerInfo) bool {
	if caster.CharID == target.CharID {
		return true
	}
	if caster.ShowID != target.ShowID {
		return true
	}
	if !s.hasTwoHandSwordEquipped(caster) {
		handler.SendSystemMessage(sess, "請使用雙手劍")
		return true
	}
	targetViewers := nearby
	if s.deps != nil && s.deps.World != nil {
		targetViewers = s.deps.World.GetNearbyPlayersInShow(target.X, target.Y, target.MapID, 0, target.ShowID)
	}
	if target.HasBuff(87) {
		handler.BroadcastToPlayers(targetViewers, handler.BuildSkillEffect(target.CharID, 4434))
		return true
	}
	dur := shockStunDurationSeconds()
	if caster.IsGM() {
		handler.SendNormalChat(sess, 0, fmt.Sprintf("此次衝暈秒數為%d秒..只有GM看的到", dur))
	}
	stunSkill := *skill
	stunSkill.BuffDuration = dur
	s.applyBuffEffect(target, &stunSkill)
	s.spawnGroundEffect(caster, &stunSkill, shockStunEffectNpcID, world.GroundEffectShockStun, target.X, target.Y)
	if s.deps != nil && s.deps.PvP != nil {
		s.deps.PvP.TriggerPinkName(caster, target)
	}
	handler.BroadcastToPlayers(targetViewers, handler.BuildSkillEffect(target.CharID, 4434))
	if s.deps != nil && s.deps.Log != nil {
		s.deps.Log.Info(fmt.Sprintf("衝擊之暈  施法者=%s  玩家=%s  持續=%d秒", caster.Name, target.Name, dur))
	}
	return true
}

func (s *SkillSystem) ApplyNpcShockStun(caster *world.NpcInfo, target *world.PlayerInfo, skill *data.SkillInfo, leverage int) {
	if caster == nil || target == nil || skill == nil {
		return
	}
	if target.Dead || target.MapID != caster.MapID || target.ShowID != caster.ShowID || isGMInvisible(target) || target.AbsoluteBarrier {
		return
	}
	if skill.Ranged > 0 && chebyshevDist(caster.X, caster.Y, target.X, target.Y) > int32(skill.Ranged) {
		return
	}
	targetGfx := npcShockStunTargetGfx(skill)
	nearby := s.deps.World.GetNearbyPlayersInShow(caster.X, caster.Y, caster.MapID, 0, caster.ShowID)
	s.clearShockStunSleepEffects(target)
	s.clearShockStunEraseMagic(target)
	if target.HasBuff(50) || target.HasBuff(157) || !checkNpcCasterShockStunSuccess(caster, target, leverage) {
		if skill.ActionID > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildActionGfx(caster.ID, byte(skill.ActionID)))
		}
		return
	}
	if target.HasBuff(87) {
		if skill.ActionID > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildActionGfx(caster.ID, byte(skill.ActionID)))
		}
		handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(target.CharID, targetGfx))
		return
	}
	dur := shockStunDurationSeconds()
	stunSkill := *skill
	stunSkill.BuffDuration = dur
	s.applyBuffEffect(target, &stunSkill)
	s.spawnGroundEffectFromNpc(caster, &stunSkill, shockStunEffectNpcID, world.GroundEffectShockStun, target.X, target.Y)
	if skill.ActionID > 0 {
		handler.BroadcastToPlayers(nearby, handler.BuildActionGfx(caster.ID, byte(skill.ActionID)))
	}
	handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(target.CharID, targetGfx))
}

func (s *SkillSystem) clearShockStunSleepEffects(target *world.PlayerInfo) {
	if target == nil {
		return
	}
	// Java L1SkillUse.runSkill() 第 1966-1968 行明確清除 FOG_OF_SLEEPING(66) / PHANTASM(212) / 103；
	// 62 為本專案沉睡視覺輔助 buff，與其他 sleep 路徑（pvp.breakPlayerSleep）保持一致。
	hadSleep := target.Sleeped || target.HasBuff(62) || target.HasBuff(66) || target.HasBuff(103) || target.HasBuff(212)
	target.Sleeped = false
	target.RemoveBuff(62)
	target.RemoveBuff(66)
	target.RemoveBuff(103)
	target.RemoveBuff(212)
	if hadSleep && target.Session != nil {
		handler.SendParalysis(target.Session, handler.SleepRemove)
	}
}

func (s *SkillSystem) clearShockStunEraseMagic(target *world.PlayerInfo) {
	if target == nil {
		return
	}
	s.removeBuffAndRevert(target, 153)
}

func clearShockStunNpcSleepEffects(target *world.NpcInfo) {
	if target == nil {
		return
	}
	// Java L1SkillUse.runSkill() 第 1966-1968 行明確清除 FOG_OF_SLEEPING(66) / PHANTASM(212) / 103。
	target.Sleeped = false
	target.RemoveDebuff(62)
	target.RemoveDebuff(66)
	target.RemoveDebuff(103)
	target.RemoveDebuff(212)
}

func clearShockStunNpcEraseMagic(target *world.NpcInfo) {
	if target == nil {
		return
	}
	target.RemoveDebuff(153)
}

func npcShockStunTargetGfx(skill *data.SkillInfo) int32 {
	if skill != nil && skill.CastGfx > 0 {
		return skill.CastGfx
	}
	return 4434
}

func (s *SkillSystem) ApplyNpcAreaShockStun(caster *world.NpcInfo, targets []*world.PlayerInfo) {
	if caster == nil {
		return
	}
	for _, target := range targets {
		if target == nil || target.ShowID != caster.ShowID || target.HasBuff(87) || isGMInvisible(target) {
			continue
		}
		dur := areaShockStunDurationSeconds()
		stunSkill := data.SkillInfo{SkillID: 87, BuffDuration: dur}
		s.applyBuffEffect(target, &stunSkill)
		s.spawnGroundEffectFromNpc(caster, &stunSkill, shockStunEffectNpcID, world.GroundEffectShockStun, target.X, target.Y)
	}
}

func shockStunDurationSeconds() int {
	return 1 + world.RandInt(5)
}

func areaShockStunDurationSeconds() int {
	return 2 + world.RandInt(4)
}

func shockStunRange(skill *data.SkillInfo) int32 {
	if skill != nil && skill.Ranged > 0 {
		return int32(skill.Ranged)
	}
	return 1
}

func (s *SkillSystem) shockStunLineOfSightBlocked(player *world.PlayerInfo, x, y int32) bool {
	if s == nil || s.deps == nil || s.deps.MapData == nil || player == nil {
		return false
	}
	return !s.deps.MapData.HasLineOfSight(player.MapID, player.X, player.Y, x, y)
}

func (s *SkillSystem) shockStunInvalidTargetBeforeConsume(player *world.PlayerInfo, skill *data.SkillInfo, targetID int32) bool {
	if s == nil || s.deps == nil || s.deps.World == nil || player == nil || skill == nil {
		return false
	}
	if targetID == 0 || targetID == player.CharID {
		return false
	}
	if target := s.deps.World.GetByCharID(targetID); target != nil {
		if target.Dead || target.MapID != player.MapID || target.ShowID != player.ShowID || isGMInvisible(target) || target.AbsoluteBarrier {
			return true
		}
		if chebyshevDist(player.X, player.Y, target.X, target.Y) > shockStunRange(skill) {
			return true
		}
		return s.shockStunLineOfSightBlocked(player, target.X, target.Y)
	}
	if npc := s.deps.World.GetNpc(targetID); npc != nil {
		if npc.Dead || npc.MapID != player.MapID || npc.ShowID != player.ShowID || shockStunRejectedNpcTargetBeforeConsume(npc) || shockStunHiddenNpcTargetBlocked(npc) {
			return true
		}
		if chebyshevDist(player.X, player.Y, npc.X, npc.Y) > shockStunRange(skill) {
			return true
		}
		return s.shockStunLineOfSightBlocked(player, npc.X, npc.Y)
	}
	return true
}

func isGMInvisible(player *world.PlayerInfo) bool {
	return player.IsGMInvisible()
}

func shockStunHiddenNpcTargetBlocked(npc *world.NpcInfo) bool {
	if npc == nil {
		return false
	}
	return npc.HiddenStatus == world.NpcHiddenSink || npc.HiddenStatus == world.NpcHiddenFly
}

func shockStunRejectedNpcTargetBeforeConsume(npc *world.NpcInfo) bool {
	if npc == nil {
		return false
	}
	switch npc.Impl {
	case "L1Effect", "L1Illusory", "L1Doll", "L1Hierarch":
		return true
	case "L1Door":
		return npc.MaxHP == 0 || npc.MaxHP == 1
	}
	return false
}

func shockStunNpcProbabilityTargetFailure(npc *world.NpcInfo) bool {
	if npc == nil {
		return false
	}
	return npc.Impl == "L1Tower" || npc.Impl == "L1Door"
}

const shockStunEffectNpcID int32 = 81162

func (s *SkillSystem) queueShockStunOnAction(sess *net.Session, targetID int32) {
	if sess == nil || s.deps == nil || s.deps.Combat == nil {
		return
	}
	s.deps.Combat.QueueAttack(handler.AttackRequest{
		AttackerSessionID: sess.ID,
		TargetID:          targetID,
		IsMelee:           true,
	})
}

func (s *SkillSystem) shockStunSafetyZoneBlocked(sess *net.Session, casterMap int16, casterX, casterY int32, targetMap int16, targetX, targetY int32) bool {
	if s == nil || s.deps == nil || s.deps.MapData == nil {
		return false
	}
	if s.deps.MapData.IsSafetyZone(casterMap, casterX, casterY) ||
		s.deps.MapData.IsSafetyZone(targetMap, targetX, targetY) {
		handler.SendSystemMessage(sess, "在安全區域無法使用此技能。")
		return true
	}
	return false
}

func (s *SkillSystem) checkShockStunPlayerSuccess(caster, target *world.PlayerInfo) bool {
	return world.RandInt(100) < shockStunPlayerProbability(caster, target)
}

func shockStunPlayerProbability(caster, target *world.PlayerInfo) int {
	if caster == nil || target == nil {
		return 0
	}
	prob := 10
	attackLevel := caster.Level + caster.StunLevel
	if attackLevel > target.Level {
		prob = 40
	} else if attackLevel == target.Level {
		prob = 30
	}
	prob += shockStunBaseIntMagicHit(caster)
	prob += int(caster.OriginalMagicHit)
	prob += shockStunIntMagicHit(caster.Intel)
	prob -= int(target.RegistStun)
	if prob < 0 {
		return 0
	}
	if prob > 100 {
		return 100
	}
	return prob
}

func (s *SkillSystem) checkShockStunNpcSuccess(caster *world.PlayerInfo, npc *world.NpcInfo) bool {
	return world.RandInt(100) < shockStunNpcProbability(caster, npc)
}

func shockStunNpcProbability(caster *world.PlayerInfo, npc *world.NpcInfo) int {
	if caster == nil || npc == nil {
		return 0
	}
	prob := 10
	attackLevel := caster.Level + caster.StunLevel
	if attackLevel > npc.Level {
		prob = 40
	} else if attackLevel == npc.Level {
		prob = 30
	}
	prob += shockStunBaseIntMagicHit(caster)
	prob += int(caster.OriginalMagicHit)
	prob += shockStunIntMagicHit(caster.Intel)
	if prob < 0 {
		return 0
	}
	if prob > 100 {
		return 100
	}
	return prob
}

func shockStunIntMagicHit(intel int16) int {
	if intel < 23 || intel > 127 {
		return 0
	}
	return (int(intel) - 20) / 3
}

func shockStunBaseIntMagicHit(caster *world.PlayerInfo) int {
	baseInt := calcPlayerBaseIntLikeJava(caster)
	if baseInt >= 25 && baseInt <= 44 {
		return (baseInt - 15) / 10
	}
	if baseInt >= 45 {
		return 5
	}
	return 0
}

func checkNpcCasterShockStunSuccess(caster *world.NpcInfo, target *world.PlayerInfo, leverage int) bool {
	return world.RandInt(100) < shockStunNpcCasterProbability(caster, target, leverage)
}

func shockStunNpcCasterProbability(caster *world.NpcInfo, target *world.PlayerInfo, leverage int) int {
	if caster == nil || target == nil {
		return 0
	}
	if leverage <= 0 {
		leverage = 10
	}
	prob := 30
	if caster.Level > target.Level {
		prob = 70
	} else if caster.Level == target.Level {
		prob = 50
	}
	prob = prob * leverage / 10
	prob -= int(target.RegistStun)
	if prob < 0 {
		return 0
	}
	if prob > 90 {
		return 90
	}
	return prob
}

func (s *SkillSystem) hasTwoHandSwordEquipped(player *world.PlayerInfo) bool {
	if player == nil {
		return false
	}
	wpn := player.Equip.Weapon()
	if wpn == nil || s.deps == nil || s.deps.Items == nil {
		return false
	}
	info := s.deps.Items.Get(wpn.ItemID)
	return info != nil && info.Type == "tohandsword"
}

// ========================================================================
//  NPC Debuff
// ========================================================================

// executeNpcDebuffSkill 對 NPC 施加 debuff 技能。
func (s *SkillSystem) executeNpcDebuffSkill(sess *net.Session, player *world.PlayerInfo, skill *data.SkillInfo, npc *world.NpcInfo) {
	ws := s.deps.World

	if npc.MapID != player.MapID || npc.ShowID != player.ShowID {
		return
	}

	dist := chebyshevDist(player.X, player.Y, npc.X, npc.Y)
	if skill.SkillID == 87 {
		if shockStunRejectedNpcTargetBeforeConsume(npc) || shockStunHiddenNpcTargetBlocked(npc) {
			return
		}
		if dist > shockStunRange(skill) {
			return
		}
		if s.shockStunLineOfSightBlocked(player, npc.X, npc.Y) {
			return
		}
	} else {
		maxRange := int32(skill.Ranged)
		if maxRange <= 0 {
			maxRange = 10
		}
		if dist > maxRange+2 {
			return
		}
	}

	player.Heading = CalcHeading(player.X, player.Y, npc.X, npc.Y)

	// 對 NPC 施放 debuff 技能 → 累加仇恨（讓 NPC 追擊施法者）
	AddPlayerHateLikeJava(ws, npc, player, 1)

	nearby := ws.GetNearbyPlayersInShow(npc.X, npc.Y, npc.MapID, 0, npc.ShowID)

	if skill.SkillID != 87 {
		handler.BroadcastToPlayers(nearby, handler.BuildActionGfx(player.CharID, byte(skill.ActionID)))
	}

	switch skill.SkillID {
	case 23: // 能量感測 — Java: 依 NPC weakAttr bitmask 廣播地/火/水/風弱點特效
		if npc.Impl != "L1Monster" {
			return
		}
		broadcastWeakElementalEffects(npc, nearby)

	case handler.SkillShapeChange: // Owner: skill_polymorph.go
		s.executeShapeChangeNpc(sess, player, skill, npc, nearby)

	case 133: // 弱化屬性 — Owner: skill_elemental.go
		if player.ElfAttr == 0 {
			handler.SendServerMessage(sess, 79)
			return
		}
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 32
		}
		applyElementalFallDownToNpc(player, npc, dur)
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("弱化屬性  施法者=%s  NPC=%s  屬性=%d  持續=%d秒", player.Name, npc.Name, player.ElfAttr, dur))

	case 27: // 壞物術 — Owner: skill_weapon.go；NPC 近戰傷害減半，直到相消術清除
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		npc.WeaponBroken = true
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("壞物術  施法者=%s  NPC=%s", player.Name, npc.Name))

	case 87: // 衝擊之暈 — 需要雙手劍
		s.queueShockStunOnAction(sess, npc.ID)
		// Java `L1SkillUse.isTargetFailure()` 對 L1TowerInstance / L1DoorInstance 回傳 true，
		// TYPE_PROBABILITY 流程在 iter 階段就 remove，沒有清睡眠、解 ERASE_MAGIC、
		// 概率判定、套用 87 或送 4434；onAction 仍會在迴圈外觸發。
		if shockStunNpcProbabilityTargetFailure(npc) {
			return
		}
		clearShockStunNpcSleepEffects(npc)
		clearShockStunNpcEraseMagic(npc)
		if s.shockStunSafetyZoneBlocked(sess, player.MapID, player.X, player.Y, npc.MapID, npc.X, npc.Y) {
			return
		}
		if npc.HasDebuff(50) || npc.HasDebuff(157) {
			return
		}
		if !s.hasTwoHandSwordEquipped(player) {
			handler.SendSystemMessage(sess, "請使用雙手劍")
			return
		}
		if npc.HasDebuff(87) {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, 4434))
			return
		}
		if !s.checkShockStunNpcSuccess(player, npc) {
			return
		}
		dur := shockStunDurationSeconds()
		if player.IsGM() {
			handler.SendNormalChat(sess, 0, fmt.Sprintf("此次衝暈秒數為%d秒..只有GM看的到", dur))
		}
		if npc.Impl == "L1Monster" || npc.Impl == "L1Summon" || npc.Impl == "L1Pet" {
			npc.Paralyzed = true
		}
		npc.AddDebuff(87, dur*5)
		stunSkill := *skill
		stunSkill.BuffDuration = dur
		s.spawnGroundEffect(player, &stunSkill, shockStunEffectNpcID, world.GroundEffectShockStun, npc.X, npc.Y)
		handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, 4434))
		s.deps.Log.Info(fmt.Sprintf("衝擊之暈  施法者=%s  NPC=%s  持續=%d秒", player.Name, npc.Name, dur))

	case 157: // 大地屏障 — 凍結 + 灰色色調
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		dur := 1 + world.RandInt(12)
		npc.Paralyzed = true
		npc.AddDebuff(157, dur*5)
		handler.BroadcastToPlayers(nearby, handler.BuildPoison(npc.ID, 2))
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("大地屏障  施法者=%s  NPC=%s  持續=%d秒", player.Name, npc.Name, dur))

	case 103: // 暗黑盲咒
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 3
		}
		npc.Sleeped = true
		npc.AddDebuff(66, dur*5)
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("暗黑盲咒  施法者=%s  NPC=%s  持續=%d秒", player.Name, npc.Name, dur))

	case 66: // 沉睡之霧
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 10
		}
		npc.Sleeped = true
		npc.AddDebuff(66, dur*5)
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("沉睡之霧  施法者=%s  NPC=%s  持續=%d秒", player.Name, npc.Name, dur))

	case 212: // 幻想 — Java skillmode/PHANTASM.java:27-28 對 NPC `setSkillEffect(66, integer*1000) + setSleeped(true)`
		// 使用 FOG_OF_SLEEPING(66) 作為實際 debuff key 對齊 Java 行為（npc.hasSkillEffect(66) 可被
		// 其他系統正確命中）。NPC MR 機率走 generic `checkNpcMRResist`，與 Java
		// `L1MagicPc.calcProbabilityMagic case PHANTASM` ConfigIllusionstSkill 5/10/15
		// 公式有差異，屬 broader gap。
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 5
		}
		npc.Sleeped = true
		npc.AddDebuff(66, dur*5)
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("幻想  施法者=%s  NPC=%s  持續=%d秒", player.Name, npc.Name, dur))

	case 217: // 恐慌 — Java skillmode/PANIC.java:32-39 對 NPC `addStr/Con/Dex/Wis/Int(-1) + setSkillEffect(217, integer*1000)`
		// Go NPC 模型只有 STR/DEX 無 Con/Wis/Intel，且既有 NPC debuff 系統採 marker-only
		// 不修改 NPC stat（參考 skill 56 DISEASE 同模式），只註冊 debuff 標記讓
		// npc.HasDebuff(217) 可被其他系統查詢。實際 stat -1 屬 broader gap，需先建立
		// NPC stat mutation/revert 機制才能完整對齊。
		// NPC MR 機率走 generic `checkNpcMRResist`，與 Java `L1MagicNpc.calcProbabilityMagic
		// case PANIC` 的 `Random.nextInt(11)+20 + (atkLvl-defLvl)*2 * leverage/10` 公式
		// 不同屬 broader gap。
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 64
		}
		npc.AddDebuff(217, dur*5)
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("恐慌  施法者=%s  NPC=%s  持續=%d秒", player.Name, npc.Name, dur))

	case 33: // 木乃伊詛咒（NPC 版）— 階段一：灰色延遲
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		if npc.Paralyzed || npc.HasDebuff(33) || npc.HasDebuff(4001) {
			return
		}
		npc.AddDebuff(33, curseDelayTicks)
		handler.BroadcastToPlayers(nearby, handler.BuildPoison(npc.ID, 2))
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("木乃伊詛咒(階段一)  施法者=%s  NPC=%s  延遲=8秒", player.Name, npc.Name))

	case 11: // 毒咒 — 對 NPC 施加傷害毒（Java: L1DamagePoison.doInfection, 3000ms, 5dmg）
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		if !applyDamagePoisonToNpc(npc, sess.ID, 5, s.deps) {
			return
		}
		AddPlayerHateLikeJava(ws, npc, player, 1)
		npc.AddDebuff(11, 150) // 30 秒 = 150 ticks
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("毒咒  施法者=%s  NPC=%s  持續=30秒  每次=5傷害", player.Name, npc.Name))

	case 29, 76, 152: // 緩速系列
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 64
		}
		npc.AddDebuff(skill.SkillID, dur*5)
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("緩速術  施法者=%s  NPC=%s  技能=%d  持續=%d秒", player.Name, npc.Name, skill.SkillID, dur))

	case 50: // 冰矛 — NPC 凍結（Java: setFrozen + S_Poison 灰色）
		if npc.Paralyzed || npc.HasDebuff(50) || npc.HasDebuff(80) || npc.HasDebuff(22) || npc.HasDebuff(30) {
			break // 已被凍結
		}
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 8
		}
		npc.Paralyzed = true
		npc.AddDebuff(50, (dur+1)*5)
		handler.BroadcastToPlayers(nearby, handler.BuildPoison(npc.ID, 2))
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("冰矛凍結  施法者=%s  NPC=%s  持續=%d秒", player.Name, npc.Name, dur+1))

	case 80: // 冰雪颶風 — 對 NPC 施加凍結（Java: setFrozen + S_Poison 灰色）
		if npc.Paralyzed || npc.HasDebuff(50) || npc.HasDebuff(80) {
			break // 已被凍結/冰矛
		}
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			break // 抗性判定失敗不阻止傷害，只是不凍結
		}
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 16
		}
		npc.Paralyzed = true
		npc.AddDebuff(80, (dur+1)*5) // Java: buffDuration + 1
		handler.BroadcastToPlayers(nearby, handler.BuildPoison(npc.ID, 2))
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("冰雪颶風凍結  施法者=%s  NPC=%s  持續=%d秒", player.Name, npc.Name, dur+1))

	case 47: // 弱化術 — NPC debuff（Java: DMG-5, HIT-1）
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 64
		}
		npc.AddDebuff(47, dur*5)
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("弱化術  施法者=%s  NPC=%s  持續=%d秒", player.Name, npc.Name, dur))

	case 56: // 疾病術 — NPC debuff（Java: DMG-6, AC+12）
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 64
		}
		npc.AddDebuff(56, dur*5)
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("疾病術  施法者=%s  NPC=%s  持續=%d秒", player.Name, npc.Name, dur))

	case 112: // 破壞盔甲（NPC debuff）— Java: ARMOR_BREAK.java 對怪物/召喚/寵物
		if !s.calcArmorBreakProbNpc(player, npc) {
			s.sendCastFail(sess)
			return
		}
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 8
		}
		// 移除舊效果 → 重新套用
		npc.AddDebuff(112, dur*5) // 8 秒 = 40 ticks
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		handler.SendGlobalChat(sess, 9, "\\f2破壞盔甲 施放成功!")
		s.deps.Log.Info(fmt.Sprintf("破壞盔甲  施法者=%s  NPC=%s  持續=%d秒", player.Name, npc.Name, dur))

	case 173, 174: // 污濁之水 / 精準射擊 — Java 對 NPC 目標 setSkillEffect(skillID, duration)
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 192
		}
		npc.AddDebuff(skill.SkillID, dur*5)
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("妖精 NPC debuff  施法者=%s  NPC=%s  技能=%d  持續=%d秒", player.Name, npc.Name, skill.SkillID, dur))

	case 167: // 風之枷鎖 — Java WIND_SHACKLE.start(NPC) 對 cha=L1NpcInstance 走 setSkillEffect(167, integer*1000)
		if npc.HasDebuff(167) {
			if skill.CastGfx > 0 {
				handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
			}
			return // Java：hasSkillEffect(167) 已存在則略過
		}
		if !s.checkNpcMRResist(player, npc, skill.SkillID) {
			s.sendCastFail(sess)
			return
		}
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 16
		}
		npc.AddDebuff(167, dur*5)
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("風之枷鎖  施法者=%s  NPC=%s  持續=%d秒", player.Name, npc.Name, dur))

	case 44: // 魔法相消術 — 解除 NPC 所有 debuff + 狀態（Java: CANCELLATION.java:158-167）
		// 清除所有 debuffs
		for debuffID := range npc.ActiveDebuffs {
			if s.deps.Scripting.IsNonCancellable(int(debuffID)) {
				continue
			}
			delete(npc.ActiveDebuffs, debuffID)
		}
		hadNpcPoly := npc.PolyOriginalGfxID != 0
		clearNpcCancellationState(npc)
		// 清除所有視覺效果（毒色/灰色）
		handler.BroadcastToPlayers(nearby, handler.BuildPoison(npc.ID, 0))
		if hadNpcPoly {
			for _, viewer := range nearby {
				handler.SendChangeShape(viewer.Session, npc.ID, npc.GfxID, 0)
			}
		}
		// 施法特效
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
		s.deps.Log.Info(fmt.Sprintf("魔法相消術(NPC)  施法者=%s  NPC=%s", player.Name, npc.Name))

	default:
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, skill.CastGfx))
		}
	}
}

func broadcastWeakElementalEffects(npc *world.NpcInfo, nearby []*world.PlayerInfo) {
	if npc.WeakAttr&weakElementalEarth == weakElementalEarth {
		handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, 2169))
	}
	if npc.WeakAttr&weakElementalFire == weakElementalFire {
		handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, 2167))
	}
	if npc.WeakAttr&weakElementalWater == weakElementalWater {
		handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, 2166))
	}
	if npc.WeakAttr&weakElementalWind == weakElementalWind {
		handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(npc.ID, 2168))
	}
}

// checkNpcMRResist uses Eddy L1Magic.calcProbabilityMagic for player -> NPC effects.
func (s *SkillSystem) checkNpcMRResist(caster *world.PlayerInfo, npc *world.NpcInfo, skillID int32) bool {
	if caster.IsGM() {
		return true
	}
	if s.deps.Skills == nil {
		return false
	}
	sk := s.deps.Skills.Get(skillID)
	if sk == nil || (npc.HasDebuff(157) && skillID != 27 && skillID != 44) {
		return false
	}
	prob := javaPlayerDebuffProbability(caster, int(npc.Level), int(npc.MR), sk, world.RandInt)
	return world.RandInt(100) < min(90, prob)
}

// playerDebuffSkills 需要對玩家目標進行 MR 抗性判定的 debuff 技能。
// 這些技能對其他玩家施放時，必須通過魔法抗性檢查才能命中。
var playerDebuffSkills = map[int32]bool{
	11:  true, // 毒咒
	20:  true, // 闇盲咒術
	27:  true, // 壞物術
	29:  true, // 緩速術
	33:  true, // 木乃伊詛咒
	40:  true, // 黑闇之影
	47:  true, // 弱化術
	56:  true, // 疾病術
	66:  true, // 沉睡之霧
	71:  true, // 藥水霜化術
	76:  true, // 集體緩速術
	87:  true, // 衝擊之暈
	103: true, // 暗黑盲咒
	133: true, // 弱化屬性
	152: true, // 究極緩速術
	153: true, // 魔法消除
	157: true, // 大地屏障
	161: true, // 封印禁地
	167: true, // 風之枷鎖
	173: true, // 污濁之水
	174: true, // 精準射擊
	183: true, // 護衛毀滅
	188: true, // 恐懼無助
	193: true, // 驚悚死神
	212: true, // 幻想
	217: true, // 恐慌
}

// checkPlayerMRResist uses skill-specific dice and the matching status resistance.
func (s *SkillSystem) checkPlayerMRResist(caster, target *world.PlayerInfo, skillID int32) bool {
	if caster.IsGM() {
		return true
	}
	if s.deps.Skills == nil {
		return false
	}
	sk := s.deps.Skills.Get(skillID)
	if sk == nil || (target.HasBuff(157) && skillID != 27 && skillID != 44) {
		return false
	}
	prob := javaPlayerDebuffProbability(caster, int(target.Level), int(target.MR), sk, world.RandInt)
	prob = javaApplyStatusResistance(prob, skillID, target)
	return world.RandInt(100) < min(90, prob)
}

// calcArmorBreakProb 破壞盔甲對玩家目標的機率判定。
// Java: L1MagicPc.calcProbabilityMagic(ARMOR_BREAK) — 非標準 MR 判定，使用等級比較系統。
// 攻擊者等級 > 防禦者 → 60%；相等 → 40%；攻擊者 < 防禦者 → 20%
// 加上 7.6 magichit `(INT-20)/3` 與 BaseInt 加成（25-44: +(BaseInt-15)/10, 45+: +5）。
func (s *SkillSystem) calcArmorBreakProb(caster, target *world.PlayerInfo) bool {
	atkLv := int(caster.Level)
	defLv := int(target.Level)

	var prob int
	if atkLv > defLv {
		prob = 60
	} else if atkLv == defLv {
		prob = 40
	} else {
		prob = 20
	}

	// Java `L1MagicPc.java:738-744`：先加 magichit（7.6 智力魔法命中），再加 BaseInt 加成。
	// BaseInt 必須排除裝備與 buff 加成，與 `_pc.getBaseInt()` 對齊。
	prob += shockStunIntMagicHit(caster.Intel)
	prob += shockStunBaseIntMagicHit(caster)

	if prob < 1 {
		prob = 1
	}
	return world.RandInt(100) < prob
}

// calcArmorBreakProbNpc 破壞盔甲對 NPC 目標的機率判定。
// 與玩家版本相同的機率系統，但使用 NPC 等級。
func (s *SkillSystem) calcArmorBreakProbNpc(caster *world.PlayerInfo, npc *world.NpcInfo) bool {
	atkLv := int(caster.Level)
	defLv := int(npc.Level)

	var prob int
	if atkLv > defLv {
		prob = 60
	} else if atkLv == defLv {
		prob = 40
	} else {
		prob = 20
	}

	prob += shockStunIntMagicHit(caster.Intel)
	prob += shockStunBaseIntMagicHit(caster)

	if prob < 1 {
		prob = 1
	}
	return world.RandInt(100) < prob
}

func armorBreakProbabilityByLevel(attackLevel, defenseLevel, baseInt int) int {
	var prob int
	if attackLevel > defenseLevel {
		prob = 60
	} else if attackLevel == defenseLevel {
		prob = 40
	} else {
		prob = 20
	}
	if baseInt >= 25 && baseInt <= 44 {
		prob += (baseInt - 15) / 10
	} else if baseInt >= 45 {
		prob += 5
	}
	return prob
}
