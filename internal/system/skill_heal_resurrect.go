package system

import (
	"fmt"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
)

// ========================================================================
//  復活技能
// ========================================================================

// isResurrectionSkill 檢查是否為復活型技能（定義在 Lua）。
func (s *SkillSystem) isResurrectionSkill(skill *data.SkillInfo) bool {
	fn := s.deps.Scripting
	if fn == nil {
		return false
	}
	return fn.GetResurrectEffect(int(skill.SkillID)) != nil
}

// teleportToMatherBlockedBeforeConsume 對齊 Java `TELEPORT_TO_MATHER.start()` 第 23-35 行
// 與 isEscapable 檢查，在 MP 消耗前返回。回傳 true 表示已送回饋封包並阻擋。
func (s *SkillSystem) teleportToMatherBlockedBeforeConsume(sess *net.Session, player *world.PlayerInfo) bool {
	if player.HasBuff(4000) { // 束縛
		handler.SendNormalChat(sess, 0, "\\fY已被束縛的效果無法瞬移")
		return true
	}
	if player.HasBuff(192) { // 奪命之雷
		handler.SendNormalChat(sess, 0, "\\fY身上有奪命之雷的效果無法瞬移")
		handler.SendParalysis(sess, handler.TeleportUnlock)
		return true
	}
	if s.deps.MapData != nil {
		if mi := s.deps.MapData.GetInfo(player.MapID); mi != nil && !mi.Escapable {
			handler.SendServerMessage(sess, 276)
			handler.SendParalysis(sess, handler.TeleportUnlock)
			return true
		}
	}
	return false
}

// executeResurrection 處理復活技能（18, 75, 131, 165）。
func (s *SkillSystem) executeResurrection(sess *net.Session, player *world.PlayerInfo, skill *data.SkillInfo, targetID int32) {
	nearby := s.deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, 0, player.ShowID)

	// 廣播施法動畫
	// Java `L1SkillUse.sendGrfx:1639` 對 TELEPORT/MASS_TELEPORT/TELEPORT_TO_MATHER 三項
	// 跳過 `S_DoActionGFX` 整個分支——傳送技能不送施法動作，避免動畫與「消失/出現」視覺衝突。
	if skill.SkillID != 131 {
		actData := handler.BuildActionGfx(player.CharID, byte(skill.ActionID))
		handler.BroadcastToPlayers(nearby, actData)
	}

	switch skill.SkillID {
	case 131: // 世界樹的呼喚 — 回母樹（33047, 32338, map 4, heading 5）
		// Java `skillmode/TELEPORT_TO_MATHER.java:52` 在 Teleportation 之前廣播
		// `S_SkillSound(self.id, 169)` 給附近玩家——讓施法者消失前播放傳送音效。
		if skill.CastGfx > 0 {
			handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(player.CharID, skill.CastGfx))
		}
		handler.TeleportPlayer(sess, player, 33047, 32338, 4, 5, s.deps)

	case 61, 75: // 返生術 / 終極返生術 — 需要目標同意
		if targetID == 0 {
			s.sendCastFail(sess)
			return
		}
		if target := s.deps.World.GetByCharID(targetID); target != nil {
			if !target.Dead {
				s.sendCastFail(sess)
				return
			}
			if target.MapID != player.MapID || target.ShowID != player.ShowID {
				return
			}
			// 儲存待復活資訊 → 發送同意對話框
			target.PendingResSkill = skill.SkillID
			target.PendingResCaster = player.CharID
			// Java: msgID 321（返生術）、322（終極返生術）— "%0 要為你施展復活術，是否同意？(Y/N)"
			msgID := uint16(321)
			if skill.SkillID == 75 {
				msgID = 322
			}
			handler.SendYesNoDialog(target.Session, msgID, player.Name)
			return
		}
		if pet := s.deps.World.GetPet(targetID); pet != nil {
			if s.resurrectPetWithHP(sess, player, skill, pet, pet.MaxHP/4) {
				return
			}
			s.sendCastFail(sess)
			return
		}
		if npc := s.deps.World.GetNpc(targetID); npc != nil {
			if npc.ShowID != player.ShowID {
				s.sendCastFail(sess)
				return
			}
			if s.resurrectNpcWithHP(npc, npc.MaxHP/4) {
				return
			}
			s.sendCastFail(sess)
			return
		}
		s.sendCastFail(sess)

	case 165: // 自然呼喚：玩家目標需送出復活同意視窗，不直接復活。
		if targetID == 0 {
			s.sendCastFail(sess)
			return
		}
		if target := s.deps.World.GetByCharID(targetID); target != nil {
			if !target.Dead {
				s.sendCastFail(sess)
				return
			}
			if target.MapID != player.MapID || target.ShowID != player.ShowID {
				return
			}
			if s.deps.World.IsPlayerAt(target.X, target.Y, target.MapID, target.SessionID) {
				handler.SendServerMessage(sess, 592)
				return
			}
			target.TempID = player.CharID
			target.PendingResSkill = skill.SkillID
			target.PendingResCaster = player.CharID
			handler.SendYesNoDialog(target.Session, 322)
			return
		}
		if pet := s.deps.World.GetPet(targetID); pet != nil {
			if s.callOfNatureResurrectPet(sess, player, skill, pet) {
				return
			}
			s.sendCastFail(sess)
			return
		}
		if npc := s.deps.World.GetNpc(targetID); npc != nil {
			if npc.ShowID != player.ShowID {
				s.sendCastFail(sess)
				return
			}
			if s.callOfNatureResurrectNpc(npc) {
				return
			}
			s.sendCastFail(sess)
			return
		}
		s.sendCastFail(sess)
	}

	// 施法特效
	if skill.CastGfx > 0 {
		effData := handler.BuildSkillEffect(player.CharID, skill.CastGfx)
		handler.BroadcastToPlayers(nearby, effData)
	}
}

// resurrectPlayer 復活死亡玩家，HP/MP 依復活技能定義。
func (s *SkillSystem) resurrectPlayer(target *world.PlayerInfo, caster *world.PlayerInfo, skill *data.SkillInfo) {
	target.Dead = false

	eff := s.deps.Scripting.GetResurrectEffect(int(skill.SkillID))
	if eff != nil {
		if eff.FixedHP == -1 {
			target.HP = int32(caster.Level)
		} else if eff.FixedHP > 0 {
			target.HP = int32(eff.FixedHP)
		} else {
			target.HP = int32(float64(target.MaxHP) * eff.HPRatio)
			target.MP = int32(float64(target.MaxMP) * eff.MPRatio)
		}
	} else {
		target.HP = int32(target.Level)
	}

	if target.HP < 1 {
		target.HP = 1
	}
	if target.HP > target.MaxHP {
		target.HP = target.MaxHP
	}
	if target.MP > target.MaxMP {
		target.MP = target.MaxMP
	}

	sendHpUpdate(target.Session, target)
	sendMpUpdate(target.Session, target)
	handler.SendPlayerStatus(target.Session, target)
	handler.SendPutObject(target.Session, target)

	nearbyTarget := s.deps.World.GetNearbyPlayersInShow(target.X, target.Y, target.MapID, 0, target.ShowID)
	for _, viewer := range nearbyTarget {
		if viewer.SessionID != target.SessionID {
			handler.SendPutObject(viewer.Session, target)
		}
	}

	s.deps.Log.Info(fmt.Sprintf("玩家復活  目標=%s  施法者=%s  技能ID=%d", target.Name, caster.Name, skill.SkillID))
}

// ========================================================================
//  NPC/寵物復活輔助
// ========================================================================

// callOfNatureResurrectNpc 以指定比例復活 NPC，165 使用滿 HP。
func (s *SkillSystem) callOfNatureResurrectNpc(npc *world.NpcInfo) bool {
	return s.resurrectNpcWithHP(npc, npc.MaxHP)
}

func (s *SkillSystem) resurrectNpcWithHP(npc *world.NpcInfo, hp int32) bool {
	if npc == nil || !npc.Dead || npc.Impl == "L1Tower" || s.isNpcCantResurrect(npc) {
		return false
	}
	if hp <= 0 {
		hp = 1
	}
	npc.Dead = false
	npc.HP = hp
	npc.DeleteTimer = 0
	npc.RespawnTimer = 0
	npc.AggroTarget = 0
	npc.HateList = nil
	npc.CompanionHate = nil
	npc.AttackTimer = 0
	npc.MoveTimer = 0
	npc.StuckTicks = 0
	npc.Paralyzed = false
	npc.Sleeped = false
	npc.WeaponBroken = false
	npc.ActiveDebuffs = nil
	npc.PoisonDmgAmt = 0
	npc.PoisonDmgTimer = 0
	npc.PoisonAttackerSID = 0
	removeShapeChangeFromNpc(npc)
	s.deps.World.OccupyEntity(npc.MapID, npc.X, npc.Y, npc.ID)
	return true
}

func (s *SkillSystem) isNpcCantResurrect(npc *world.NpcInfo) bool {
	if npc == nil {
		return true
	}
	if npc.CantResurrect {
		return true
	}
	if s.deps != nil && s.deps.Npcs != nil {
		if tmpl := s.deps.Npcs.Get(npc.NpcID); tmpl != nil && tmpl.CantResurrect {
			return true
		}
	}
	return false
}

func (s *SkillSystem) callOfNatureResurrectPet(sess *net.Session, player *world.PlayerInfo, skill *data.SkillInfo, pet *world.PetInfo) bool {
	return s.resurrectPetWithHP(sess, player, skill, pet, pet.MaxHP)
}

func (s *SkillSystem) resurrectPetWithHP(sess *net.Session, player *world.PlayerInfo, skill *data.SkillInfo, pet *world.PetInfo, hp int32) bool {
	if pet == nil || !pet.Dead {
		return false
	}
	if player.MapID != pet.MapID || player.ShowID != pet.ShowID || chebyshevDist(player.X, player.Y, pet.X, pet.Y) > 20 {
		return false
	}
	if s.deps.World.IsPlayerAt(pet.X, pet.Y, pet.MapID, 0) {
		handler.SendServerMessage(sess, 592)
		return true
	}
	if hp <= 0 {
		hp = 1
	}
	pet.Dead = false
	pet.HP = hp
	pet.Status = world.PetStatusRest
	pet.AggroTarget = 0
	pet.AggroPlayerID = 0
	pet.AggroPetID = 0
	pet.Dirty = true
	s.deps.World.PetRevive(pet)

	nearbyPet := s.deps.World.GetNearbyPlayersInShow(pet.X, pet.Y, pet.MapID, 0, pet.ShowID)
	for _, viewer := range nearbyPet {
		handler.SendRemoveObject(viewer.Session, pet.ID)
		if viewer.Known != nil {
			delete(viewer.Known.Pets, pet.ID)
		}
	}
	handler.SendPetHpMeter(sess, pet.ID, pet.HP, pet.MaxHP)
	if skill.CastGfx > 0 {
		handler.BroadcastToPlayers(nearbyPet, handler.BuildSkillEffect(pet.ID, skill.CastGfx))
	}
	return true
}

// ========================================================================
//  寵物/召喚物治療
// ========================================================================

// healCompanion 對自己的寵物或召喚物施放治療技能。
// Java: L1SkillUse.java — TYPE_HEAL + TARGET_TO_PET，只對自己的寵物/召喚物有效。
func (s *SkillSystem) healCompanion(sess *net.Session, player *world.PlayerInfo, skill *data.SkillInfo,
	companionID int32, hp *int32, maxHP int32, cx, cy int32, cMapID int16) {

	// 距離檢查
	if player.MapID != cMapID || chebyshevDist(player.X, player.Y, cx, cy) > 20 {
		return
	}

	nearby := s.deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, 0, player.ShowID)

	// 廣播施法動畫
	handler.BroadcastToPlayers(nearby, handler.BuildActionGfx(player.CharID, byte(skill.ActionID)))

	// 計算治療量
	if skill.DamageValue > 0 || skill.DamageDice > 0 {
		heal := int32(s.deps.Scripting.CalcHeal(skill.DamageValue, skill.DamageDice, skill.DamageDiceCount, int(player.Intel), int(player.Lawful), 10))
		if heal > 0 && *hp < maxHP {
			*hp += heal
			if *hp > maxHP {
				*hp = maxHP
			}
			// 發送 HP 條更新給主人
			handler.SendPetHpMeter(sess, companionID, *hp, maxHP)
		}
	}

	// 效果 GFX
	if skill.CastGfx > 0 {
		handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(companionID, skill.CastGfx))
	}
}

// resurrectPet 復活死亡寵物。
// Java: L1Character.resurrect() + L1PetInstance 特殊處理。
// 返生術(61)恢復 25% HP，終極返生術(75)恢復 100% HP。
func (s *SkillSystem) resurrectPet(sess *net.Session, player *world.PlayerInfo, skill *data.SkillInfo, pet *world.PetInfo) {
	if player.MapID != pet.MapID || player.ShowID != pet.ShowID || chebyshevDist(player.X, player.Y, pet.X, pet.Y) > 20 {
		return
	}

	nearby := s.deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, 0, player.ShowID)

	// 廣播施法動畫
	handler.BroadcastToPlayers(nearby, handler.BuildActionGfx(player.CharID, byte(skill.ActionID)))

	// 恢復 HP（返生術 25%，終極返生術 100%）
	hp := pet.MaxHP / 4
	if skill.SkillID == 75 {
		hp = pet.MaxHP
	}
	if hp <= 0 {
		hp = 1
	}

	// 復活寵物
	pet.Dead = false
	pet.HP = hp
	pet.Status = world.PetStatusRest
	pet.AggroTarget = 0
	pet.AggroPlayerID = 0
	pet.Dirty = true

	// 重新佔用格子
	s.deps.World.PetRevive(pet)

	// 讓附近玩家重新認識寵物（Java: removeKnownObject → updateObject）
	nearbyPet := s.deps.World.GetNearbyPlayersInShow(pet.X, pet.Y, pet.MapID, 0, pet.ShowID)
	for _, viewer := range nearbyPet {
		// 先移除再重新發送，確保客戶端正確更新
		handler.SendRemoveObject(viewer.Session, pet.ID)
		delete(viewer.Known.Pets, pet.ID)
	}

	// 發送 HP 條更新給主人
	handler.SendPetHpMeter(sess, pet.ID, pet.HP, pet.MaxHP)

	// 效果 GFX
	if skill.CastGfx > 0 {
		handler.BroadcastToPlayers(nearbyPet, handler.BuildSkillEffect(pet.ID, skill.CastGfx))
	}
}
