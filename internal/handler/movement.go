package handler

import (
	"math/rand"
	"time"

	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

// Direction deltas indexed by heading (0-7).
var headingDX = [8]int32{0, 1, 1, 1, 0, -1, -1, -1}
var headingDY = [8]int32{-1, -1, 0, 1, 1, 1, 0, -1}

// dungeonTeleportInvincibilityTicks 副本傳送後絕對屏障持續時間（5 ticks/秒 × 2 秒）。
// Java: DungeonTable.dg() / DungeonRTable.dg() — pc.setSkillEffect(ABSOLUTE_BARRIER, 2000)。
const dungeonTeleportInvincibilityTicks int = 10

// applyDungeonTeleportEffect 套用 Java DungeonTable/DungeonRTable 傳送前的副作用：
//   - 2 秒絕對屏障（skill 78，免疫所有傷害；防止抵達後即遭怪物 aggro 秒殺）
//   - 重置 HP 累計器（對齊 Java stopHpRegeneration；MP regen 為全域 tick 無需重置）
//
// 兩種 regen 函式都會在 AbsoluteBarrier=true 時 skip，因此 2 秒內 HP/MP 都不會回。
// 注意：若玩家已有 skill 78 buff（cast 取得，預設 12 秒），AddBuff 會替換為 2 秒——
// 對齊 Java setSkillEffect 同 ID 覆寫的行為。
func applyDungeonTeleportEffect(player *world.PlayerInfo) {
	if player == nil {
		return
	}
	player.AbsoluteBarrier = true
	player.RegenHPAcc = 0
	player.AddBuff(&world.ActiveBuff{
		SkillID:            78, // ABSOLUTE_BARRIER
		TicksLeft:          dungeonTeleportInvincibilityTicks,
		SetAbsoluteBarrier: true,
	})
}

// HandleMove processes C_MOVE (opcode 29).
// Java C_MoveChar 依 wire 值域判別明文 heading（0~7）與台版 XOR 0x49。
// 每包只表示走一格；點擊遠處後的連續步伐由客戶端逐包送出。
// 座標仍使用伺服器狀態，不把客戶端 X/Y 當成移動目的地。
func HandleMove(sess *net.Session, r *packet.Reader, deps *Deps) {
	// ReadC 在截短封包會回傳 0；先檢查 HH C，避免誤認成向北移動。
	if r.Remaining() < 5 {
		return
	}
	_ = r.ReadH() // client X（安全考量統一忽略，使用伺服器端座標）
	_ = r.ReadH() // client Y（同上）
	rawHeading := r.ReadC()

	// E:\L1JTW-Eddy C_MoveChar：登入器可送明文，不可僅依語系強制 XOR。
	// 兩種合法值域不重疊；除此之外的 heading 一律拒絕。
	heading := int16(rawHeading)
	if heading > 7 {
		heading = int16(rawHeading ^ 0x49)
	}
	if heading > 7 {
		return
	}

	ws := deps.World
	player := ws.GetBySession(sess.ID)
	if player == nil {
		return
	}

	// 麻痺/暈眩/凍結/睡眠時無法移動（客戶端已鎖定，這裡做伺服器端防護）
	if player.Paralyzed || player.Sleeped {
		return
	}

	// InputSystem 一次輪詢可批次分派多包，處理間隔不是客戶端送包間隔。
	// 不可在此用牆鐘間隔丟棄後續步伐，否則伺服器座標會永久落後。
	// Java C_MoveChar 對加速檢查 R_DETECTED 仍會移動；只在 R_DISPOSED 中止。
	// 網路層仍保留每連線 PPS 上限；此時間僅記錄最後處理的移動要求。
	player.LastMoveTime = time.Now().UnixNano()

	// 永遠使用伺服器端座標（安全性考量，所有語系統一）
	curX := player.X
	curY := player.Y

	// 從目前位置 + 朝向計算目的地
	destX := curX + headingDX[heading]
	destY := curY + headingDY[heading]

	// Java C_MoveChar 流程（第 130 行）：先清除舊座標 0x80，再做通行性判定。
	if deps.MapData != nil {
		deps.MapData.SetImpassable(player.MapID, curX, curY, false)
	}

	// ── 地圖切換點檢查（Java: C_MoveChar → DungeonTable.dg() / DungeonRTable.dg()）──
	// 玩家走入傳送座標時，直接觸發傳送，不移動到該格。

	// 1. 固定傳送門（DungeonTable）
	if deps.Portals != nil {
		if portal := deps.Portals.Get(destX, destY, player.MapID); portal != nil {
			// 船舶碼頭需額外驗證航線時間和船票
			isDock, allowed := CheckShipDock(destX, destY, player.MapID, player)
			if !isDock || allowed {
				// 一般傳送門或碼頭驗證通過 → 傳送（不移動到 destX/destY）
				cancelTradeIfActive(player, deps)
				applyDungeonTeleportEffect(player)
				teleportPlayer(sess, player, portal.DstX, portal.DstY, portal.DstMapID, portal.DstHeading, deps)
				return
			}
			// 碼頭驗證失敗 → 繼續正常移動（Java: dg() returns false）
		}
	}

	// 2. 隨機傳送門（DungeonRTable）— 多目標隨機選一個
	// Java: C_MoveChar → DungeonRTable.dg() 在 DungeonTable 之後檢查
	if deps.RandomPortals != nil {
		if rp := deps.RandomPortals.Get(destX, destY, player.MapID); rp != nil && len(rp.Destinations) > 0 {
			idx := rand.Intn(len(rp.Destinations))
			dst := rp.Destinations[idx]
			cancelTradeIfActive(player, deps)
			applyDungeonTeleportEffect(player)
			teleportPlayer(sess, player, dst.X, dst.Y, dst.MapID, rp.DstHeading, deps)
			return
		}
	}

	// 地形通行性檢查 + Java fallback（第 160-174 行）：
	// 1. isPassable 失敗 → 2. CheckUtil.checkPassable 檢查目的地有無實體
	// 地形不通 + 無實體 → 放行（信任客戶端，tile 資料可能與客戶端不完全吻合）
	// 地形不通 + 有實體佔位 → 拒絕
	if deps.MapData != nil && !deps.MapData.IsPassableIgnoreOccupant(player.MapID, curX, curY, int(heading)) {
		if ws.IsOccupied(destX, destY, player.MapID, player.CharID) {
			// 恢復舊座標 0x80（因為上面已經清除了，拒絕時要恢復）
			deps.MapData.SetImpassable(player.MapID, curX, curY, true)
			rejectMove(sess, player, ws, deps)
			return
		}
		// 地形不通但目的地無實體 → 信任客戶端，放行
	}

	// Update position to DESTINATION
	ws.UpdatePosition(sess.ID, destX, destY, player.MapID, heading)

	// Mark new position as impassable (for NPC pathfinding)
	if deps.MapData != nil {
		deps.MapData.SetImpassable(player.MapID, destX, destY, true)
	}

	// ── 陷阱觸發檢查（Java: WorldTrap.onPlayerMoved()）──
	if deps.TrapMgr != nil {
		if traps := deps.TrapMgr.GetTrapsAt(destX, destY, player.MapID); len(traps) > 0 {
			handleTrapTrigger(sess, player, traps, deps)
		}
	}

	// Auto-cancel trade if partner is too far (> 15 tiles or different map)
	if player.TradePartnerID != 0 {
		partner := deps.World.GetByCharID(player.TradePartnerID)
		if partner != nil {
			tdx := destX - partner.X
			tdy := destY - partner.Y
			if tdx < 0 {
				tdx = -tdx
			}
			if tdy < 0 {
				tdy = -tdy
			}
			dist := tdx
			if tdy > dist {
				dist = tdy
			}
			if dist > 15 || player.MapID != partner.MapID {
				cancelTradeIfActive(player, deps)
			}
		} else {
			cancelTradeIfActive(player, deps)
		}
	}

	// 廣播移動封包 + 格子封鎖（匹配 Java C_MoveChar: broadcastPacketAll）。
	// 只做 1 次 GetNearbyPlayers 查詢（原本 16 次）。
	// 玩家進出視野由 VisibilitySystem（Phase 3, 每 400ms）獨立處理。
	nearby := ws.GetNearbyPlayersInShow(destX, destY, player.MapID, sess.ID, player.ShowID)
	data := BuildMoveObject(player.CharID, curX, curY, heading)
	for _, viewer := range nearby {
		if viewer.CanSeePlayer(player) {
			viewer.Session.Send(data)
		}
	}
}

// rejectMove 碰撞拒絕：回彈玩家位置 + 重發所有附近實體。
// 對應 Java L1PcUnlock.Pc_Unlock() 流程：
//
//	S_OwnCharPack → removeAllKnownObjects → updateObject → S_CharVisualUpdate
//
// 只發 S_OwnCharPack 會讓客戶端清除附近物件渲染，必須立即重發所有可見實體。
func rejectMove(sess *net.Session, player *world.PlayerInfo, ws *world.State, deps *Deps) {
	// 1. 回彈：告知客戶端正確座標
	sendOwnCharPackPlayer(sess, player)

	// 2. 重置 Known 集合（客戶端收到 OwnCharPack 後會清除附近物件渲染）
	if player.Known != nil {
		player.Known.Reset()
	}

	px, py := player.X, player.Y

	// 3. 重發所有附近玩家 + 封鎖格子 + 填入 Known
	nearbyPlayers := ws.GetNearbyPlayersInShow(px, py, player.MapID, sess.ID, player.ShowID)
	for _, other := range nearbyPlayers {
		if !player.CanSeePlayer(other) {
			continue
		}
		SendPutObject(sess, other)
		if player.Known != nil {
			player.Known.Players[other.CharID] = world.KnownPos{X: other.X, Y: other.Y}
		}
	}

	// 4. 重發所有附近 NPC + 封鎖格子
	nearbyNpcs := ws.GetNearbyNpcsInShow(px, py, player.MapID, player.ShowID)
	for _, n := range nearbyNpcs {
		SendNpcPack(sess, n)
		if player.Known != nil {
			player.Known.Npcs[n.ID] = world.KnownPos{X: n.X, Y: n.Y}
		}
	}

	// 5. 重發所有附近召喚獸 + 封鎖格子
	nearbySummons := ws.GetNearbySummonsInShow(px, py, player.MapID, player.ShowID)
	for _, s := range nearbySummons {
		isOwner := s.OwnerCharID == player.CharID
		masterName := ""
		if m := ws.GetByCharID(s.OwnerCharID); m != nil {
			masterName = m.Name
		}
		SendSummonPack(sess, s, isOwner, masterName)
		if player.Known != nil {
			player.Known.Summons[s.ID] = world.KnownPos{X: s.X, Y: s.Y}
		}
	}

	// 6. 重發所有附近魔法娃娃 + 封鎖格子
	nearbyDolls := ws.GetNearbyDollsInShow(px, py, player.MapID, player.ShowID)
	for _, d := range nearbyDolls {
		masterName := ""
		if m := ws.GetByCharID(d.OwnerCharID); m != nil {
			masterName = m.Name
		}
		SendDollPack(sess, d, masterName)
		if player.Known != nil {
			player.Known.Dolls[d.ID] = world.KnownPos{X: d.X, Y: d.Y}
		}
	}

	// 7. 重發所有附近隨身祭司
	nearbyHierarchs := ws.GetNearbyHierarchsInShow(px, py, player.MapID, player.ShowID)
	for _, h := range nearbyHierarchs {
		masterName := ""
		if m := ws.GetByCharID(h.OwnerCharID); m != nil {
			masterName = m.Name
		}
		SendHierarchPack(sess, h, masterName)
		if player.Known != nil {
			player.Known.Hierarchs[h.ID] = world.KnownPos{X: h.X, Y: h.Y}
		}
	}

	// 8. 重發所有附近隨從 + 封鎖格子
	nearbyFollowers := ws.GetNearbyFollowersInShow(px, py, player.MapID, player.ShowID)
	for _, f := range nearbyFollowers {
		SendFollowerPack(sess, f)
		if player.Known != nil {
			player.Known.Followers[f.ID] = world.KnownPos{X: f.X, Y: f.Y}
		}
	}

	// 8. 重發所有附近寵物 + 封鎖格子
	nearbyPets := ws.GetNearbyPetsInShow(px, py, player.MapID, player.ShowID)
	for _, p := range nearbyPets {
		isOwner := p.OwnerCharID == player.CharID
		masterName := ""
		if m := ws.GetByCharID(p.OwnerCharID); m != nil {
			masterName = m.Name
		}
		SendPetPack(sess, p, isOwner, masterName)
		if player.Known != nil {
			player.Known.Pets[p.ID] = world.KnownPos{X: p.X, Y: p.Y}
		}
	}

	// 9. 重發所有附近地面物品（地面物品不佔格子，不需封鎖）
	nearbyGnd := ws.GetNearbyGroundItemsInShow(player.X, player.Y, player.MapID, player.ShowID)
	for _, g := range nearbyGnd {
		SendDropItem(sess, g)
		if player.Known != nil {
			player.Known.GroundItems[g.ID] = world.KnownPos{X: g.X, Y: g.Y}
		}
	}

	// 10. 重發所有附近門
	nearbyDoors := ws.GetNearbyDoors(player.X, player.Y, player.MapID)
	for _, d := range nearbyDoors {
		SendDoorPerceive(sess, d)
		if player.Known != nil {
			player.Known.Doors[d.ID] = world.KnownPos{X: d.X, Y: d.Y}
		}
	}

	// 11. 武器/變身視覺更新（Java 流程最後一步）
	sendCharVisualUpdate(sess, player)
}

// HandleChangeDirection processes C_CHANGE_DIRECTION (opcode 225).
// NOTE: Unlike C_MOVE, C_ChangeHeading does NOT XOR heading with 0x49 — raw value.
func HandleChangeDirection(sess *net.Session, r *packet.Reader, deps *Deps) {
	heading := int16(r.ReadC())
	if heading < 0 || heading > 7 {
		return
	}

	player := deps.World.GetBySession(sess.ID)
	if player == nil {
		return
	}
	deps.World.ChangePlayerHeading(player, heading)

	// 廣播方向變更給附近玩家
	nearby := deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, sess.ID, player.ShowID)
	chData := BuildChangeHeading(player.CharID, heading)
	for _, viewer := range nearby {
		if viewer.CanSeePlayer(player) {
			viewer.Session.Send(chData)
		}
	}
}
