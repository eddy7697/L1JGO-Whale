package system

import "github.com/l1jgo/server/internal/world"

// AddHate 累加仇恨值並維護 AggroTarget 快取。
// 若新仇恨累計超過當前目標，自動切換 AggroTarget。
// 遊戲迴圈單線程呼叫，無需鎖。
func AddHate(npc *world.NpcInfo, sessionID uint64, damage int32) {
	if damage <= 0 || sessionID == 0 {
		return
	}
	if npc.HateList == nil {
		npc.HateList = make(map[uint64]int32)
	}
	npc.HateList[sessionID] += damage

	// 首次受擊或仇恨超過當前目標 → 切換
	if npc.AggroTarget == 0 {
		npc.AggroTarget = sessionID
		linkMobGroupHateLikeJava(npc, sessionID)
		return
	}
	if sessionID != npc.AggroTarget {
		if npc.HateList[sessionID] > npc.HateList[npc.AggroTarget] {
			npc.AggroTarget = sessionID
		}
	}
	linkMobGroupHateLikeJava(npc, sessionID)
}

func AddPlayerHateLikeJava(ws *world.State, npc *world.NpcInfo, player *world.PlayerInfo, damage int32) {
	if npc == nil || player == nil {
		return
	}
	AddHate(npc, player.SessionID, damage)
	if damage <= 0 {
		return
	}
	linkNpcFamilyHateLikeJava(ws, npc, player)
}

func linkNpcFamilyHateLikeJava(ws *world.State, npc *world.NpcInfo, player *world.PlayerInfo) {
	if ws == nil || npc == nil || player == nil || player.SessionID == 0 {
		return
	}
	for _, member := range ws.GetNearbyNpcsInShow(player.X, player.Y, player.MapID, npc.ShowID) {
		if member == nil || member == npc || member.Dead {
			continue
		}
		if len(member.HateList) != 0 {
			continue
		}
		switch {
		case member.AgroFamily == 1:
			if npc.Family == "" || member.Family != npc.Family {
				continue
			}
		case member.AgroFamily > 1:
		default:
			continue
		}
		setNpcLinkTargetLikeJava(member, player.SessionID)
	}
}

func linkMobGroupHateLikeJava(npc *world.NpcInfo, sessionID uint64) {
	if npc == nil || sessionID == 0 || npc.GroupInfo == nil {
		return
	}
	for _, member := range npc.GroupInfo.Members {
		if member == nil || member == npc || member.Dead {
			continue
		}
		if member.ShowID != npc.ShowID {
			continue
		}
		if member.NpcID == 99007 {
			continue
		}
		if len(member.HateList) != 0 {
			continue
		}
		setNpcLinkTargetLikeJava(member, sessionID)
	}
}

func setNpcLinkTargetLikeJava(npc *world.NpcInfo, sessionID uint64) {
	if npc == nil || sessionID == 0 {
		return
	}
	npc.HateList = map[uint64]int32{sessionID: 0}
	npc.AggroTarget = sessionID
}

// GetMaxHateTarget 回傳仇恨值最高的 SessionID。
// 用於當前目標失效時重新選擇。回傳 0 表示仇恨列表為空。
func GetMaxHateTarget(npc *world.NpcInfo) uint64 {
	if len(npc.HateList) == 0 {
		return 0
	}
	var maxSID uint64
	var maxHate int32 = -1
	for sid, hate := range npc.HateList {
		if hate > maxHate {
			maxHate = hate
			maxSID = sid
		}
	}
	return maxSID
}

// RemoveHateTarget 從仇恨列表移除指定目標（斷線／離開範圍）。
func RemoveHateTarget(npc *world.NpcInfo, sessionID uint64) {
	if npc.HateList != nil {
		delete(npc.HateList, sessionID)
	}
}

// ClearHateList 清空仇恨列表（NPC 死亡或重生時呼叫）。
func ClearHateList(npc *world.NpcInfo) {
	npc.HateList = nil
	npc.CompanionHate = nil
}

// AddCompanionHate 記錄寵物／召喚獸造成的仇恨：仇恨歸主人（驅動 NPC 目標選擇），
// 同時個別記錄以便經驗分配時拆分主人與寵物份額（Java: CalcExp 以 L1PetInstance 為獨立取得者）。
func AddCompanionHate(npc *world.NpcInfo, companionID int32, masterSessionID uint64, damage int32) {
	if damage <= 0 || masterSessionID == 0 {
		return
	}
	AddHate(npc, masterSessionID, damage)
	if npc.CompanionHate == nil {
		npc.CompanionHate = make(map[int32]int32)
	}
	npc.CompanionHate[companionID] += damage
}

// GetTotalHate 回傳所有仇恨的累計總值（經驗分配用）。
func GetTotalHate(npc *world.NpcInfo) int32 {
	var total int32
	for _, h := range npc.HateList {
		total += h
	}
	return total
}
