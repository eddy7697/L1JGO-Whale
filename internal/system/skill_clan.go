package system

import (
	"time"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
)

const (
	targetToClan         = 4
	targetToParty        = 8
	braveAvatarMasteryID = int32(119)
	braveAvatarSkillID   = int32(8065)
	braveAvatarRange     = int32(16)
	braveAvatarInterval  = 5 * time.Second
)

func (s *SkillSystem) applyRoyalAuraSkill(player *world.PlayerInfo, skill *data.SkillInfo, nearby []*world.PlayerInfo) bool {
	if player == nil || skill == nil {
		return false
	}
	switch skill.SkillID {
	case 114, 115, 117:
	default:
		return false
	}

	targets := s.royalAuraTargets(player, skill.TargetTo, nearby)
	for _, target := range targets {
		s.applyBuffEffect(target, skill)
	}
	if skill.CastGfx > 0 {
		handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(player.CharID, skill.CastGfx))
	}
	if skill.SysMsgHappen > 0 {
		handler.SendServerMessage(player.Session, uint16(skill.SysMsgHappen))
	}
	return true
}

func (s *SkillSystem) royalAuraTargets(player *world.PlayerInfo, targetTo int, nearby []*world.PlayerInfo) []*world.PlayerInfo {
	targets := []*world.PlayerInfo{player}
	seen := map[int32]bool{player.CharID: true}
	for _, other := range nearby {
		if other == nil || other.Dead || seen[other.CharID] {
			continue
		}
		if targetTo&targetToParty != 0 && sameParty(s.deps.World, player, other) {
			targets = append(targets, other)
			seen[other.CharID] = true
			continue
		}
		if targetTo&targetToClan != 0 && player.ClanID != 0 && player.ClanID == other.ClanID {
			targets = append(targets, other)
			seen[other.CharID] = true
		}
	}
	return targets
}

func sameParty(ws *world.State, a, b *world.PlayerInfo) bool {
	if ws == nil || a == nil || b == nil {
		return false
	}
	party := ws.Parties.GetParty(a.CharID)
	if party == nil {
		return false
	}
	for _, memberID := range party.Members {
		if memberID == b.CharID {
			return true
		}
	}
	return false
}

func braveAuraDamage(attacker *world.PlayerInfo, damage int32) int32 {
	return braveAuraDamageWithRoll(attacker, damage, world.RandInt(100))
}

func braveAuraDamageWithRoll(attacker *world.PlayerInfo, damage int32, roll int) int32 {
	if attacker == nil || damage <= 0 || !attacker.HasBuff(117) || roll >= 33 {
		return damage
	}
	return damage * 3 / 2
}

func (s *SkillSystem) updateBraveAvatarAura() {
	if s == nil || s.deps == nil || s.deps.World == nil {
		return
	}
	s.deps.World.AllPlayers(func(player *world.PlayerInfo) {
		if player == nil {
			return
		}
		if s.shouldHaveBraveAvatar(player) {
			s.applyBraveAvatar(player)
			return
		}
		s.removeBraveAvatar(player)
	})
}

func (s *SkillSystem) shouldHaveBraveAvatar(player *world.PlayerInfo) bool {
	party := s.deps.World.Parties.GetParty(player.CharID)
	if party == nil || len(party.Members) < 2 {
		return false
	}
	leader := s.deps.World.GetByCharID(party.LeaderID)
	if leader == nil || leader.ClassType != 0 || !s.playerKnowsSpell(leader, braveAvatarMasteryID) {
		return false
	}
	if leader.MapID != player.MapID {
		return false
	}
	return chebyshevDist(leader.X, leader.Y, player.X, player.Y) <= braveAvatarRange
}

func (s *SkillSystem) applyBraveAvatar(player *world.PlayerInfo) {
	if player.HasBuff(braveAvatarSkillID) {
		return
	}
	buff := &world.ActiveBuff{
		SkillID:            braveAvatarSkillID,
		TicksLeft:          0,
		DeltaStr:           1,
		DeltaDex:           1,
		DeltaIntel:         1,
		DeltaMR:            10,
		DeltaRegistStun:    2,
		DeltaRegistSustain: 2,
	}
	player.Str += buff.DeltaStr
	player.Dex += buff.DeltaDex
	player.Intel += buff.DeltaIntel
	player.MR += buff.DeltaMR
	player.RegistStun += buff.DeltaRegistStun
	player.RegistSustain += buff.DeltaRegistSustain
	player.AddBuff(buff)
	handler.SendPlayerStatus(player.Session, player)
	// Java `BraveAvatarTimer.run()` 第 54 行 `pc.sendPackets(new S_SPMR(pc))`：MR 改變需另送 S_SPMR，
	// `SendPlayerStatus`(S_STATUS) 不含 MR/SP。`applyBraveAvatar` 走獨立路徑非 `applyBuffEffect`，需手動補。
	handler.SendMagicStatus(player.Session, byte(player.SP), int32(player.MR))
	handler.SendNoneTimeIcon(player.Session, true, 479)
	nearby := s.deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, 0, player.ShowID)
	handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(player.CharID, 9009))
}

func (s *SkillSystem) removeBraveAvatar(player *world.PlayerInfo) {
	if !player.HasBuff(braveAvatarSkillID) {
		return
	}
	s.removeBuffAndRevert(player, braveAvatarSkillID)
	handler.SendNoneTimeIcon(player.Session, false, 479)
}

func (s *SkillSystem) applyTrueTargetEffect(caster, target *world.PlayerInfo, skill *data.SkillInfo, text string) {
	if caster == nil || target == nil || skill == nil {
		return
	}
	if !target.HasBuff(113) {
		dur := skill.BuffDuration
		if dur <= 0 {
			dur = 5
		}
		target.AddBuff(&world.ActiveBuff{
			SkillID:   113,
			TicksLeft: dur * 5,
		})
	}
	s.sendTrueTargetToClan(caster, target, text)
}

func (s *SkillSystem) sendTrueTargetToClan(caster, target *world.PlayerInfo, text string) {
	if caster.ClanID == 0 || s.deps == nil || s.deps.World == nil {
		handler.SendTrueTarget(caster.Session, target.CharID, caster.CharID, text)
		return
	}
	sent := false
	s.deps.World.AllPlayers(func(player *world.PlayerInfo) {
		if player != nil && player.ClanID == caster.ClanID {
			handler.SendTrueTarget(player.Session, target.CharID, caster.CharID, text)
			sent = true
		}
	})
	if !sent {
		handler.SendTrueTarget(caster.Session, target.CharID, caster.CharID, text)
	}
}

func (s *SkillSystem) clanSkillViewers(player *world.PlayerInfo) []*world.PlayerInfo {
	if player == nil {
		return nil
	}
	return s.deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, 0, player.ShowID)
}

func (s *SkillSystem) executeClanTargetSkill(sess *net.Session, player *world.PlayerInfo, skill *data.SkillInfo, targetID int32, targetName string, consume bool) {
	target := s.resolveClanSkillTarget(targetID, targetName)
	if target == nil {
		if targetName != "" {
			handler.SendServerMessageArgs(sess, 73, targetName)
			return
		}
		s.sendCastFail(sess)
		return
	}
	if target.CharID == player.CharID || player.ClanID == 0 || player.ClanID != target.ClanID {
		handler.SendServerMessage(sess, 414)
		return
	}

	switch skill.SkillID {
	case 116:
		if consume {
			s.consumeSkillResources(sess, player, skill)
		}
		nearby := s.clanSkillViewers(player)
		handler.BroadcastToPlayers(nearby, handler.BuildActionGfx(player.CharID, byte(skill.ActionID)))
		target.PendingYesNoType = 729
		target.PendingYesNoData = player.CharID
		handler.SendYesNoDialog(target.Session, 729)

	case 118:
		// Java `L1SkillUse.java:481-482` TYPE_NORMAL 流程：`runSkill() → useConsume()`。
		// `runSkill()` 進入 skillmode.start，若條件失敗（送 647/1192 + S_Paralysis）仍正常返回，
		// 因此 `useConsume()` 一定執行——MP 在 RUN_CLAN 失敗時也會消耗。Go 將 consume 提到 canRunClanTeleport
		// 檢查之前，與 Java 一致。
		if consume {
			s.consumeSkillResources(sess, player, skill)
		}
		if !s.canRunClanTeleport(player, target) {
			handler.SendServerMessage(sess, s.runClanRejectMessage(player, target))
			handler.SendParalysis(sess, handler.TeleportUnlock)
			return
		}
		nearby := s.clanSkillViewers(player)
		handler.BroadcastToPlayers(nearby, handler.BuildActionGfx(player.CharID, byte(skill.ActionID)))
		handler.TeleportPlayer(sess, player, target.X, target.Y, target.MapID, 5, s.deps)
	}
}

func (s *SkillSystem) resolveClanSkillTarget(targetID int32, targetName string) *world.PlayerInfo {
	if s == nil || s.deps == nil || s.deps.World == nil {
		return nil
	}
	if targetID != 0 {
		if target := s.deps.World.GetByCharID(targetID); target != nil {
			return target
		}
	}
	if targetName != "" {
		return s.deps.World.GetByName(targetName)
	}
	return nil
}

func (s *SkillSystem) canRunClanTeleport(player, target *world.PlayerInfo) bool {
	if !s.isEscapableForRunClan(player) {
		return false
	}
	if !isRunClanAllowedTargetMap(target.MapID) {
		return false
	}
	return !s.isInAnyCastleWarArea(target.X, target.Y, target.MapID)
}

func (s *SkillSystem) runClanRejectMessage(player, target *world.PlayerInfo) uint16 {
	if !s.isEscapableForRunClan(player) {
		return 647
	}
	if !isRunClanAllowedTargetMap(target.MapID) || s.isInAnyCastleWarArea(target.X, target.Y, target.MapID) {
		return 1192
	}
	return skillMsgCastFail
}

func (s *SkillSystem) isEscapableForRunClan(player *world.PlayerInfo) bool {
	if player.AccessLevel >= 200 {
		return true
	}
	if s.deps == nil || s.deps.MapData == nil {
		return true
	}
	if mi := s.deps.MapData.GetInfo(player.MapID); mi != nil {
		return mi.Escapable
	}
	return true
}

func isRunClanAllowedTargetMap(mapID int16) bool {
	return mapID == 0 || mapID == 4 || mapID == 304
}

func (s *SkillSystem) isInAnyCastleWarArea(x, y int32, mapID int16) bool {
	return s.deps != nil && s.deps.Castles != nil && s.deps.Castles.GetCastleIDByArea(x, y, mapID) != 0
}
