package system

import (
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
)

// executeTeleportSpell 處理傳送技能（5: 瞬間移動, 69: 集體瞬間移動）。
func (s *SkillSystem) executeTeleportSpell(sess *net.Session, player *world.PlayerInfo, skill *data.SkillInfo, bookmarkID int32) {
	var destX, destY int32
	var destMapID int16
	var destHeading int16 = 5

	if skill.SkillID == 5 && player.HasBuff(4000) {
		handler.SendNormalChat(sess, 0, "\\fY已被束縛的效果無法瞬移")
		return
	}
	if skill.SkillID == 5 && player.HasBuff(192) {
		handler.SendNormalChat(sess, 0, "\\fY身上有奪命之雷的效果無法瞬移")
		handler.SendParalysis(sess, handler.TeleportUnlock)
		return
	}

	if bookmarkID != 0 {
		// --- 書籤傳送 ---
		if s.deps.MapData != nil {
			if mi := s.deps.MapData.GetInfo(player.MapID); mi != nil {
				if ok, msgID := checkBookmarkTeleportMapRule(skill.SkillID, mi, player); !ok {
					handler.SendServerMessage(sess, msgID)
					handler.SendParalysis(sess, handler.TeleportUnlock)
					return
				}
			}
		}

		var found *world.Bookmark
		for i := range player.Bookmarks {
			if player.Bookmarks[i].ID == bookmarkID {
				found = &player.Bookmarks[i]
				break
			}
		}
		if found == nil {
			handler.SendParalysis(sess, handler.TeleportUnlock)
			return
		}
		destX = found.X
		destY = found.Y
		destMapID = found.MapID
	} else {
		// --- 隨機傳送 ---
		if s.deps.MapData != nil {
			if mi := s.deps.MapData.GetInfo(player.MapID); mi != nil {
				if ok, msgID := checkRandomTeleportMapRule(skill.SkillID, mi, player); !ok {
					handler.SendServerMessage(sess, msgID)
					handler.SendParalysis(sess, handler.TeleportUnlock)
					return
				}
			}
		}

		destMapID = player.MapID
		destX = player.X
		destY = player.Y

		minRX := player.X - 200
		maxRX := player.X + 200
		minRY := player.Y - 200
		maxRY := player.Y + 200
		if s.deps.MapData != nil {
			if mi := s.deps.MapData.GetInfo(destMapID); mi != nil {
				if minRX < mi.StartX {
					minRX = mi.StartX
				}
				if maxRX > mi.EndX {
					maxRX = mi.EndX
				}
				if minRY < mi.StartY {
					minRY = mi.StartY
				}
				if maxRY > mi.EndY {
					maxRY = mi.EndY
				}
			}
		}

		diffX := maxRX - minRX
		diffY := maxRY - minRY
		if diffX > 0 && diffY > 0 {
			for attempt := 0; attempt < 40; attempt++ {
				rx := minRX + int32(world.RandInt(int(diffX)+1))
				ry := minRY + int32(world.RandInt(int(diffY)+1))
				if s.deps.MapData != nil && s.deps.MapData.IsInMap(destMapID, rx, ry) &&
					s.deps.MapData.IsPassablePoint(destMapID, rx, ry) {
					destX = rx
					destY = ry
					break
				}
			}
		}
	}

	// --- 驗證通過，消耗 MP ---
	if skill.MpConsume > 0 {
		player.SpendMP(int32(skill.MpConsume))
		sendMpUpdate(sess, player)
	}

	nearby := s.deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, 0, player.ShowID)
	handler.BroadcastToPlayers(nearby, handler.BuildSkillEffect(player.CharID, int32(skill.CastGfx)))

	// 傳送時取消交易
	handler.CancelTradeIfActive(player, s.deps)

	// --- 集體傳送(69)：施法者傳送前先收集同公會成員 ---
	var clanMembers []*world.PlayerInfo
	if skill.SkillID == 69 && player.ClanID != 0 {
		for _, member := range nearby {
			if member.CharID == player.CharID {
				continue
			}
			if member.ClanID != player.ClanID {
				continue
			}
			if member.PrivateShop {
				continue
			}
			if chebyshevDist(player.X, player.Y, member.X, member.Y) > 3 {
				continue
			}
			if !member.NoAskMassTeleport {
				member.TeleportX = destX
				member.TeleportY = destY
				member.TeleportMapID = destMapID
				member.TeleportHeading = destHeading
				member.PendingYesNoType = 748
				member.PendingYesNoData = player.CharID
				handler.SendYesNoDialog(member.Session, 748)
				continue
			}
			clanMembers = append(clanMembers, member)
		}
	}

	handler.TeleportPlayer(sess, player, destX, destY, destMapID, destHeading, s.deps)

	// --- 集體傳送(69)：傳送血盟成員到相同目的地 ---
	for _, member := range clanMembers {
		handler.CancelTradeIfActive(member, s.deps)
		handler.TeleportPlayer(member.Session, member, destX, destY, destMapID, destHeading, s.deps)
	}
}

func checkBookmarkTeleportMapRule(skillID int32, mi *data.MapInfo, player *world.PlayerInfo) (bool, uint16) {
	if mi == nil || player.IsGM() {
		return true, 0
	}
	if skillID == 69 {
		if !mi.Escapable {
			return false, 276
		}
		return true, 0
	}
	if !mi.Teleportable && !hasTowerTeleportPermit(player, mi.MapID) {
		return false, 647
	}
	return true, 0
}

func checkRandomTeleportMapRule(skillID int32, mi *data.MapInfo, player *world.PlayerInfo) (bool, uint16) {
	if mi == nil || player.IsGM() || mi.Teleportable || (skillID == 5 && hasTowerTeleportPermit(player, mi.MapID)) {
		return true, 0
	}
	if skillID == 69 {
		return false, 276
	}
	return false, 647
}

func hasTowerTeleportPermit(player *world.PlayerInfo, mapID int16) bool {
	if player == nil || player.Inv == nil || mapID < 3301 || mapID > 3310 {
		return false
	}
	if hasInventoryItem(player, 84071) {
		return true
	}
	itemID := int32(84040) + int32(mapID-3300)
	return hasInventoryItem(player, itemID)
}

func hasInventoryItem(player *world.PlayerInfo, itemID int32) bool {
	item := player.Inv.FindByItemID(itemID)
	return item != nil && item.Count > 0
}
