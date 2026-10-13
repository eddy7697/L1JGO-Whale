package system

import (
	"context"
	"math/rand"
	"time"

	"github.com/l1jgo/server/internal/config"
	coresys "github.com/l1jgo/server/internal/core/system"
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// CastleSystem 處理城堡管理邏輯（稅率/寶庫/攻城戰排程）。
// 實作 handler.CastleManager 介面。
type CastleSystem struct {
	deps    *handler.Deps
	castles map[int32]*handler.CastleInfo // castle_id → 運行時狀態
	warEnd  map[int32]time.Time           // castle_id → 攻城戰結束時間

	// now 與 teleport 可由測試替換；nil 時使用 time.Now 與 handler.TeleportPlayer。
	now      func() time.Time
	teleport func(sess *net.Session, player *world.PlayerInfo, x, y int32, mapID, heading int16)
}

// NewCastleSystem 建立城堡管理系統，從 DB 載入城堡狀態。
func NewCastleSystem(deps *handler.Deps) *CastleSystem {
	s := &CastleSystem{
		deps:    deps,
		castles: make(map[int32]*handler.CastleInfo, 8),
		warEnd:  make(map[int32]time.Time, 8),
	}
	s.loadFromDB()
	return s
}

// clock 取得目前時間（測試可替換）。
func (s *CastleSystem) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// warDuration 單場攻城戰持續時間（Java Config.ALT_WAR_TIME，預設 2h）。
func (s *CastleSystem) warDuration() time.Duration {
	if s.deps != nil && s.deps.Config != nil && s.deps.Config.Gameplay.WarTime > 0 {
		return s.deps.Config.Gameplay.WarTime
	}
	return config.DefaultWarTime
}

// warInterval 攻城戰間隔（Java Config.ALT_WAR_INTERVAL，預設 4d）。
func (s *CastleSystem) warInterval() time.Duration {
	if s.deps != nil && s.deps.Config != nil && s.deps.Config.Gameplay.WarInterval > 0 {
		return s.deps.Config.Gameplay.WarInterval
	}
	return config.DefaultWarInterval
}

// teleportPlayer 傳送玩家（測試可替換）。
func (s *CastleSystem) teleportPlayer(player *world.PlayerInfo, x, y int32, mapID, heading int16) {
	if s.teleport != nil {
		s.teleport(player.Session, player, x, y, mapID, heading)
		return
	}
	handler.TeleportPlayer(player.Session, player, x, y, mapID, heading, s.deps)
}

// loadFromDB 從 DB 載入城堡動態狀態。
func (s *CastleSystem) loadFromDB() {
	if s.deps.CastleRepo == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	states, err := s.deps.CastleRepo.LoadAll(ctx)
	if err != nil {
		if s.deps.Log != nil {
			s.deps.Log.Error("載入城堡資料失敗", zap.Error(err))
		}
		return
	}

	for _, st := range states {
		ci := &handler.CastleInfo{
			CastleID:    st.CastleID,
			Name:        st.CastleName,
			TaxRate:     st.TaxRate,
			PublicMoney: st.PublicMoney,
			WarTime:     st.WarTime,
			IsWar:       false,
		}
		s.castles[st.CastleID] = ci
	}

	// 從公會資料反查城主
	s.refreshOwnerClans()

	// 計算攻城戰結束時間（開始時間 + WarTime；Java WarTimeController 建構子）
	for id, ci := range s.castles {
		s.warEnd[id] = ci.WarTime.Add(s.warDuration())
	}
}

// refreshOwnerClans 從 world.ClanManager 中反查各城堡的城主公會。
func (s *CastleSystem) refreshOwnerClans() {
	for _, ci := range s.castles {
		ci.OwnerClanID = 0
	}
	if s.deps.World == nil {
		return
	}
	s.deps.World.Clans.ForEach(func(clan *world.ClanInfo) {
		if clan.HasCastle > 0 {
			if ci, ok := s.castles[int32(clan.HasCastle)]; ok {
				ci.OwnerClanID = clan.ClanID
			}
		}
	})
}

// GetCastle 取得城堡運行時狀態。
func (s *CastleSystem) GetCastle(castleID int32) *handler.CastleInfo {
	return s.castles[castleID]
}

// GetCastleByOwnerClan 依城主公會 ID 取得城堡。
func (s *CastleSystem) GetCastleByOwnerClan(clanID int32) *handler.CastleInfo {
	for _, ci := range s.castles {
		if ci.OwnerClanID == clanID {
			return ci
		}
	}
	return nil
}

// SetTaxRate 設定城堡稅率（10-50%）。
func (s *CastleSystem) SetTaxRate(sess *net.Session, player *world.PlayerInfo, castleID int32, rate int32) {
	ci := s.castles[castleID]
	if ci == nil {
		return
	}
	if !s.isOwner(player, castleID) {
		return
	}
	if rate < 10 {
		rate = 10
	}
	if rate > 50 {
		rate = 50
	}

	ci.TaxRate = rate

	if s.deps.CastleRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.deps.CastleRepo.UpdateTaxRate(ctx, castleID, rate)
	}

	handler.SendServerMessageArgs(sess, 256, handler.Itoa(rate))
}

// Deposit 存入金幣到城堡寶庫。
func (s *CastleSystem) Deposit(sess *net.Session, player *world.PlayerInfo, castleID int32, amount int32) {
	ci := s.castles[castleID]
	if ci == nil || !s.isOwner(player, castleID) || amount <= 0 {
		return
	}

	adena := player.Inv.FindByItemID(world.AdenaItemID)
	if adena == nil || adena.Count < amount {
		handler.SendServerMessage(sess, 189)
		return
	}

	// 扣金幣
	adena.Count -= amount
	player.Dirty = true
	handler.SendItemCountUpdate(sess, adena)

	// 加寶庫
	ci.PublicMoney += int64(amount)
	if s.deps.CastleRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.deps.CastleRepo.UpdatePublicMoney(ctx, castleID, ci.PublicMoney)
	}

	handler.SendServerMessageArgs(sess, 143, handler.Itoa(amount))
}

// Withdraw 從城堡寶庫領出金幣。
func (s *CastleSystem) Withdraw(sess *net.Session, player *world.PlayerInfo, castleID int32, amount int32) {
	ci := s.castles[castleID]
	if ci == nil || !s.isOwner(player, castleID) || amount <= 0 {
		return
	}

	if int64(amount) > ci.PublicMoney {
		amount = int32(ci.PublicMoney)
	}
	if amount <= 0 {
		return
	}

	// 扣寶庫
	ci.PublicMoney -= int64(amount)
	if s.deps.CastleRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.deps.CastleRepo.UpdatePublicMoney(ctx, castleID, ci.PublicMoney)
	}

	// 加金幣
	if s.deps.ItemCreate != nil {
		if _, ok := s.deps.ItemCreate.GiveItem(sess, player, world.AdenaItemID, amount); ok {
			player.Dirty = true
			handler.SendServerMessageArgs(sess, 142, handler.Itoa(amount))
			return
		}
	}
	itemInfo := s.deps.Items.Get(world.AdenaItemID)
	destItem := player.Inv.AddItem(world.AdenaItemID, amount, "金幣", 0, 0, true, 0)
	if itemInfo != nil {
		destItem.InvGfx = itemInfo.InvGfx
		destItem.Weight = itemInfo.Weight
		destItem.Name = itemInfo.Name
	}
	player.Dirty = true
	handler.SendAddItem(sess, destItem, itemInfo)

	handler.SendServerMessageArgs(sess, 142, handler.Itoa(amount))
}

// GetTaxRate 取得城堡當前稅率。
func (s *CastleSystem) GetTaxRate(castleID int32) int32 {
	ci := s.castles[castleID]
	if ci == nil {
		return 0
	}
	return ci.TaxRate
}

// GetCastleIDByNpcLocation 依 NPC 座標查詢所屬城堡 ID。
func (s *CastleSystem) GetCastleIDByNpcLocation(x, y int32, mapID int16) int32 {
	if s.deps.Castles == nil {
		return 0
	}
	return s.deps.Castles.GetCastleIDByArea(x, y, mapID)
}

// IsWarNow 指定城堡是否在攻城戰中。
func (s *CastleSystem) IsWarNow(castleID int32) bool {
	ci := s.castles[castleID]
	return ci != nil && ci.IsWar
}

// IsAnyWarNow 是否有任何城堡在攻城戰中。
func (s *CastleSystem) IsAnyWarNow() bool {
	for _, ci := range s.castles {
		if ci.IsWar {
			return true
		}
	}
	return false
}

// CheckInWarArea 檢查座標是否在攻城戰區域中。
func (s *CastleSystem) CheckInWarArea(castleID int32, x, y int32, mapID int16) bool {
	if s.deps.Castles == nil {
		return false
	}
	return s.deps.Castles.CheckInWarAreaOrInner(castleID, x, y, mapID)
}

// AddPublicMoney 增加城堡寶庫金額（用於稅收自動存入）。
func (s *CastleSystem) AddPublicMoney(castleID int32, amount int64) {
	ci := s.castles[castleID]
	if ci == nil || amount <= 0 {
		return
	}
	ci.PublicMoney += amount
}

// TransferCastle 轉移城堡主權。
func (s *CastleSystem) TransferCastle(castleID int32, newClanID int32) {
	ci := s.castles[castleID]
	if ci == nil {
		return
	}

	// 清除舊城主
	if ci.OwnerClanID != 0 {
		oldClan := s.deps.World.Clans.GetClan(ci.OwnerClanID)
		if oldClan != nil {
			oldClan.HasCastle = 0
			if s.deps.ClanRepo != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = s.deps.ClanRepo.UpdateHasCastle(ctx, ci.OwnerClanID, 0)
			}
		}
	}

	// 設定新城主
	ci.OwnerClanID = newClanID
	if newClanID != 0 {
		newClan := s.deps.World.Clans.GetClan(newClanID)
		if newClan != nil {
			newClan.HasCastle = castleID
			if s.deps.ClanRepo != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = s.deps.ClanRepo.UpdateHasCastle(ctx, newClanID, castleID)
			}
		}
	}
}

// TickWar 攻城戰排程 tick（Java: WarTimeController.checkWarTime）。
func (s *CastleSystem) TickWar() {
	now := s.clock()

	for i := int32(1); i <= 8; i++ {
		ci := s.castles[i]
		if ci == nil {
			continue
		}
		warEnd := s.warEnd[i]

		if ci.WarTime.Before(now) && warEnd.After(now) {
			if !ci.IsWar {
				s.startWar(i, ci)
			}
		} else if warEnd.Before(now) {
			if ci.IsWar {
				s.endWar(i, ci)
			} else {
				// 攻城時間已過期且不在戰爭中（例如伺服器停機跨過攻城時段）：
				// Java 以「現在 + WarInterval」作為下次攻城時間，排程才不會卡住。
				s.pushExpiredWarTime(i, ci, now)
			}
		}
	}
}

// pushExpiredWarTime 將已過期的攻城時間往後推到 now + WarInterval。
// Java WarTimeController.checkWarTime 的「更新過期的攻城時間」分支；
// Java 同時把結束時間設為與開始相同（排程實際無法開戰），Go 改以 WarTime 推算結束時間。
// Java 的 WarUpdate 在此分支也會重設稅率與寶庫，Go 只推進時間（見稽核表 1-2 備註）。
func (s *CastleSystem) pushExpiredWarTime(castleID int32, ci *handler.CastleInfo, now time.Time) {
	ci.WarTime = now.Add(s.warInterval())
	s.warEnd[castleID] = ci.WarTime.Add(s.warDuration())

	if s.deps.Log != nil {
		s.deps.Log.Info("攻城時間已過期，往後推進",
			zap.Int32("castle_id", castleID),
			zap.String("name", ci.Name),
			zap.Time("next_war_time", ci.WarTime),
		)
	}

	if s.deps.CastleRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.deps.CastleRepo.UpdateWarTime(ctx, castleID, ci.WarTime)
	}
}

// startWar 攻城戰開始。
func (s *CastleSystem) startWar(castleID int32, ci *handler.CastleInfo) {
	ci.IsWar = true
	ci.TaxRate = 10
	ci.PublicMoney = 0

	if s.deps.CastleRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.deps.CastleRepo.ResetForWar(ctx, castleID)
	}

	if s.deps.Log != nil {
		s.deps.Log.Info("攻城戰開始", zap.Int32("castle_id", castleID), zap.String("name", ci.Name))
	}

	// 廣播攻城戰開始訊息（Java: MSG_WAR_BEGIN = 0）
	handler.BroadcastPacketBoxWar(s.deps.World, 0, castleID)

	// 生成戰爭旗
	s.SpawnWarFlags(castleID)

	// 傳送非城盟玩家
	s.teleportNonClanPlayers(castleID, ci)

	// 修復城門
	s.repairCastleGates(castleID)
}

// endWar 攻城戰結束。
func (s *CastleSystem) endWar(castleID int32, ci *handler.CastleInfo) {
	ci.IsWar = false

	if s.deps.Log != nil {
		s.deps.Log.Info("攻城戰結束", zap.Int32("castle_id", castleID), zap.String("name", ci.Name))
	}

	// 廣播攻城戰結束訊息（Java: MSG_WAR_END = 1）
	handler.BroadcastPacketBoxWar(s.deps.World, 1, castleID)

	// 更新下次攻城戰時間（+WarInterval，Java 預設 4 天；WarTimeController.WarUpdate）
	ci.WarTime = ci.WarTime.Add(s.warInterval())
	s.warEnd[castleID] = ci.WarTime.Add(s.warDuration())

	if s.deps.CastleRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.deps.CastleRepo.UpdateWarTime(ctx, castleID, ci.WarTime)
	}

	// 終止該城堡的所有戰爭（防禦方勝利）
	if s.deps.War != nil {
		s.deps.War.CeaseCastleWar(castleID)
	}

	// 發放攻城戰禮物（Java: ServerWarExecutor.checkWarTime → CastleWarGiftTable）
	s.distributeWarGifts(castleID, ci)

	// 清除戰爭旗
	s.ClearWarFlags(castleID)

	// 移除未被拾取的王冠（Java: WarTimeController — L1CrownInstance.deleteMe）
	s.ClearCrowns(castleID)

	// 守護塔刪除後重生（Java: WarTimeController — L1TowerInstance.deleteMe + L1WarSpawn.SpawnTower）
	s.SpawnTower(castleID)

	// 重生投石車（Java: ServerWarExecutor — 攻城結束後重新生成滿血投石車）
	s.ClearCatapults(castleID)
	s.SpawnCatapults(castleID)

	// 修復城門
	s.repairCastleGates(castleID)
}

// distributeWarGifts 發放攻城戰禮物給守城方在線公會成員。
// Java: CastleWarGiftTable.get().get_gift(castle_id) → 遍歷在線成員發放。
func (s *CastleSystem) distributeWarGifts(castleID int32, ci *handler.CastleInfo) {
	if s.deps.WarGifts == nil || ci.OwnerClanID == 0 {
		return
	}

	entry := s.deps.WarGifts.GetByCastle(castleID)
	if entry == nil || len(entry.Items) == 0 {
		return
	}
	if s.deps.ItemCreate == nil {
		return
	}

	// 遍歷守城公會在線成員
	s.deps.World.AllPlayers(func(p *world.PlayerInfo) {
		if p.ClanID != ci.OwnerClanID {
			return
		}

		for _, gift := range entry.Items {
			item, ok := s.deps.ItemCreate.GiveItem(p.Session, p, gift.ItemID, gift.Count)
			if !ok {
				continue
			}
			// 發送取得物品訊息。
			handler.SendServerMessageArgs(p.Session, 403, item.Name)
		}
	})

	if s.deps.Log != nil {
		s.deps.Log.Info("攻城戰禮物已發放",
			zap.Int32("castle_id", castleID),
			zap.Int32("clan_id", ci.OwnerClanID),
		)
	}
}

// teleportNonClanPlayers 將城區內非城盟玩家趕回城鎮回城點。
// Java: WarTimeController.checkWarTime／L1CrownInstance.onAction —
// 旗內且非 GM 且非城盟成員者，傳送到 L1CastleLocation.getGetBackLoc(castleId)（隨機取一點，heading 5）。
func (s *CastleSystem) teleportNonClanPlayers(castleID int32, ci *handler.CastleInfo) {
	if s.deps.Castles == nil || s.deps.World == nil {
		return
	}
	if s.deps.Castles.Get(castleID) == nil {
		return
	}

	var targets []*world.PlayerInfo
	s.deps.World.AllPlayers(func(player *world.PlayerInfo) {
		if !s.deps.Castles.CheckInWarArea(castleID, player.X, player.Y, player.MapID) {
			return
		}
		// GM 不被趕出（Java: !pc.isGm()）
		if player.IsGM() {
			return
		}
		// 城盟成員不傳送
		if ci.OwnerClanID != 0 && player.ClanID == ci.OwnerClanID {
			return
		}
		targets = append(targets, player)
	})

	for _, player := range targets {
		loc := s.deps.Castles.GetbackLoc(castleID, rand.Intn)
		s.teleportPlayer(player, loc.X, loc.Y, loc.Map, 5)
	}
}

// repairCastleGates 修復城堡所有城門。
func (s *CastleSystem) repairCastleGates(castleID int32) {
	if s.deps.World == nil || s.deps.Castles == nil {
		return
	}
	// 遍歷城堡所在地圖的門
	castleCfg := s.deps.Castles.Get(castleID)
	if castleCfg == nil {
		return
	}
	doors := s.deps.World.GetDoorsByMap(castleCfg.WarArea.Map)
	for _, door := range doors {
		if s.deps.Castles.CheckInWarArea(castleID, door.X, door.Y, door.MapID) {
			door.RepairGate()
		}
	}
}

// CastleWarTickSystem 攻城戰排程 tick 系統（Phase 3 PostUpdate）。
// 每個 tick 檢查 8 座城堡的攻城戰開始/結束時間。
type CastleWarTickSystem struct {
	castle handler.CastleManager
}

// NewCastleWarTickSystem 建立攻城戰排程 tick 系統。
func NewCastleWarTickSystem(castle handler.CastleManager) *CastleWarTickSystem {
	return &CastleWarTickSystem{castle: castle}
}

// Phase 回傳 Phase 3 (PostUpdate)。
func (s *CastleWarTickSystem) Phase() coresys.Phase {
	return coresys.PhasePostUpdate
}

// Update 每 tick 呼叫 TickWar 檢查攻城戰排程。
func (s *CastleWarTickSystem) Update(dt time.Duration) {
	if s.castle != nil {
		s.castle.TickWar()
	}
}

// --- 戰場實體管理 ---

// OnTowerDeath 守護塔被摧毀 → 在塔座標生成王冠（NPC 81125）。
// Java: L1TowerInstance.Death → L1WarSpawn.SpawnCrown
func (s *CastleSystem) OnTowerDeath(npc *world.NpcInfo) {
	// 亞丁子塔（81190-81193）不生成王冠
	if npc.NpcID >= 81190 && npc.NpcID <= 81193 {
		return
	}

	// 判斷塔所屬城堡
	castleID := s.getCastleIDByTower(npc)
	if castleID == 0 {
		return
	}

	if s.deps.Log != nil {
		s.deps.Log.Info("守護塔被摧毀，生成王冠",
			zap.Int32("castle_id", castleID),
			zap.Int32("tower_npc_id", npc.NpcID),
		)
	}

	// 生成王冠（NPC 81125）在塔座標
	crownTmpl := s.deps.Npcs.Get(81125)
	if crownTmpl == nil {
		return
	}
	crown := &world.NpcInfo{
		ID:         world.NextNpcID(),
		NpcID:      81125,
		Impl:       "L1Crown",
		GfxID:      crownTmpl.GfxID,
		Name:       crownTmpl.Name,
		NameID:     crownTmpl.NameID,
		X:          npc.X,
		Y:          npc.Y,
		MapID:      npc.MapID,
		HP:         1,
		MaxHP:      1,
		SpawnX:     npc.X,
		SpawnY:     npc.Y,
		SpawnMapID: npc.MapID,
	}
	s.deps.World.AddNpc(crown)

	// 通知附近玩家看到王冠
	nearby := s.deps.World.GetNearbyPlayers(npc.X, npc.Y, npc.MapID, 0)
	for _, p := range nearby {
		handler.SendNpcPack(p.Session, crown)
	}
}

// HandleCrownClick 玩家點擊王冠 → 城堡主權轉移。
// Java: L1CrownInstance.onAction
func (s *CastleSystem) HandleCrownClick(sess *net.Session, player *world.PlayerInfo, npc *world.NpcInfo) {
	// 前置檢查
	if player.Dead || player.ClanID == 0 {
		return
	}
	// 必須是君主（classType 0）
	if player.ClassType != 0 {
		return
	}
	// 必須是盟主
	clan := s.deps.World.Clans.GetClan(player.ClanID)
	if clan == nil || clan.LeaderID != player.CharID {
		return
	}
	// 血盟不得已有城堡
	if clan.HasCastle > 0 {
		handler.SendServerMessage(sess, 474) // 已擁有城堡
		return
	}
	// 距離檢查（±1 格）
	dx := player.X - npc.X
	dy := player.Y - npc.Y
	if dx < -1 || dx > 1 || dy < -1 || dy > 1 {
		return
	}

	// 推算城堡 ID
	castleID := s.getCastleIDByTower(npc)
	if castleID == 0 {
		// 王冠在塔座標生成，用同樣的方式推算
		if s.deps.Castles != nil {
			castleID = s.deps.Castles.GetCastleIDByArea(npc.X, npc.Y, npc.MapID)
		}
	}
	if castleID == 0 {
		return
	}

	// 檢查攻城戰是否進行中
	ci := s.castles[castleID]
	if ci == nil || !ci.IsWar {
		return
	}

	// 檢查玩家是否在戰爭中（必須已宣戰）
	if s.deps.War != nil && ci.OwnerClanID != 0 {
		defenceClan := s.deps.World.Clans.GetClan(ci.OwnerClanID)
		if defenceClan != nil {
			if !s.deps.War.IsWar(clan.ClanName, defenceClan.ClanName) {
				return // 未宣戰不可占領
			}
		}
	}

	if s.deps.Log != nil {
		s.deps.Log.Info("城堡被佔領",
			zap.Int32("castle_id", castleID),
			zap.String("new_clan", clan.ClanName),
			zap.String("leader", player.Name),
		)
	}

	// 宣布勝利（Java: war.winCastleWar）
	if s.deps.War != nil {
		s.deps.War.WinCastleWar(clan.ClanName, castleID)
	}

	// 轉移城堡主權
	s.TransferCastle(castleID, player.ClanID)

	// 刪除王冠
	s.removeWarNpc(npc)

	// 守護塔刪除後重生（Java: L1CrownInstance.onAction — tower.deleteMe + L1WarSpawn.SpawnTower）
	s.SpawnTower(castleID)

	// 通知新城盟成員
	handler.BroadcastPacketBoxWar(s.deps.World, 3, castleID) // MSG_WAR_INITIATIVE
	handler.BroadcastPacketBoxWar(s.deps.World, 4, castleID) // MSG_WAR_OCCUPY

	// 修復城門
	s.repairCastleGates(castleID)

	// 驅逐非城盟玩家
	s.teleportNonClanPlayers(castleID, ci)
}

// CanDamageTower 檢查是否可以攻擊守護塔。
// Java: L1TowerInstance.receiveDamage — 必須在攻城戰中、攻擊者必須已宣戰。
func (s *CastleSystem) CanDamageTower(player *world.PlayerInfo, npc *world.NpcInfo) bool {
	castleID := s.getCastleIDByTower(npc)
	if castleID == 0 {
		return false
	}

	// 必須在攻城戰期間
	ci := s.castles[castleID]
	if ci == nil || !ci.IsWar {
		return false
	}

	// 玩家必須有血盟
	if player.ClanID == 0 {
		return false
	}

	// 城盟成員不可攻擊自家塔
	if player.ClanID == ci.OwnerClanID {
		return false
	}

	// 玩家血盟必須在戰爭中（已宣戰）
	if s.deps.War != nil && ci.OwnerClanID != 0 {
		clan := s.deps.World.Clans.GetClan(player.ClanID)
		defenceClan := s.deps.World.Clans.GetClan(ci.OwnerClanID)
		if clan != nil && defenceClan != nil {
			if !s.deps.War.IsWar(clan.ClanName, defenceClan.ClanName) {
				return false
			}
		}
	}

	// 亞丁城主塔（81189）：子塔至少破壞 3 座才可攻擊（Java: L1TowerInstance.receiveDamage subTowerDeadCount < 3 → 不可攻擊）
	if npc.NpcID == npcIDAdenMainTower {
		if s.adenSubTowerDeadCount() < 3 {
			return false
		}
	}

	return true
}

// adenSubTowerDeadCount 計算亞丁子塔（81190-81193）已被破壞的數量。
func (s *CastleSystem) adenSubTowerDeadCount() int {
	count := 0
	for _, n := range s.deps.World.NpcList() {
		if n.NpcID >= npcIDAdenSubTower1 && n.NpcID <= npcIDAdenSubTower1+3 && n.Dead {
			count++
		}
	}
	return count
}

// --- 守護塔／王冠生命週期（Java L1WarSpawn + WarTimeController + L1CrownInstance）---

// 守護塔與王冠 NPC ID。
const (
	npcIDGuardianTower = 81111 // 一般城堡守護塔
	npcIDAdenMainTower = 81189 // 亞丁城主塔
	npcIDAdenSubTower1 = 81190 // 亞丁子塔 1-4 → 81190-81193
	npcIDCrown         = 81125 // 王冠
	adenCastleID       = 7
)

// SpawnTower 刪除城堡戰爭區內所有守護塔（含屍體）後，在塔座標重生守護塔。
// Java: WarTimeController.checkWarTime／L1CrownInstance.onAction 先 tower.deleteMe()，
// 再 L1WarSpawn.SpawnTower：一般城堡生成 81111，亞丁生成 81189 與四座子塔 81190-81193。
func (s *CastleSystem) SpawnTower(castleID int32) {
	if s.deps.Castles == nil || s.deps.Npcs == nil || s.deps.World == nil {
		return
	}
	castleCfg := s.deps.Castles.Get(castleID)
	if castleCfg == nil {
		return
	}

	s.removeTowers(castleID)

	towerNpcID := int32(npcIDGuardianTower)
	if castleID == adenCastleID {
		towerNpcID = npcIDAdenMainTower
	}
	s.spawnWarNpc(towerNpcID, castleCfg.Tower.X, castleCfg.Tower.Y, castleCfg.Tower.Map)

	if castleID == adenCastleID {
		for i, sub := range castleCfg.SubTowers {
			if i >= 4 {
				break
			}
			s.spawnWarNpc(npcIDAdenSubTower1+int32(i), sub.X, sub.Y, sub.Map)
		}
	}

	if s.deps.Log != nil {
		s.deps.Log.Info("守護塔已重生", zap.Int32("castle_id", castleID), zap.Int32("npc_id", towerNpcID))
	}
}

// removeTowers 刪除城堡戰爭區內所有守護塔 NPC（存活或屍體）。
func (s *CastleSystem) removeTowers(castleID int32) {
	var toRemove []*world.NpcInfo
	for _, npc := range s.deps.World.NpcList() {
		if npc.Impl != "L1Tower" {
			continue
		}
		if s.deps.Castles.CheckInWarArea(castleID, npc.X, npc.Y, npc.MapID) {
			toRemove = append(toRemove, npc)
		}
	}
	for _, npc := range toRemove {
		s.removeWarNpc(npc)
	}
}

// ClearCrowns 刪除城堡戰爭區內尚未被拾取的王冠（NPC 81125）。
// Java: WarTimeController.checkWarTime — L1CrownInstance.deleteMe。
func (s *CastleSystem) ClearCrowns(castleID int32) {
	if s.deps.Castles == nil || s.deps.World == nil {
		return
	}
	var toRemove []*world.NpcInfo
	for _, npc := range s.deps.World.NpcList() {
		if npc.NpcID != npcIDCrown {
			continue
		}
		if s.deps.Castles.CheckInWarArea(castleID, npc.X, npc.Y, npc.MapID) {
			toRemove = append(toRemove, npc)
		}
	}
	for _, npc := range toRemove {
		s.removeWarNpc(npc)
	}
}

// removeWarNpc 立即從世界移除戰爭物件 NPC 並通知附近玩家（Java: deleteMe + S_RemoveObject）。
// 屍體（Dead 且尚未清理 AOI）先做 AOI 清理，避免殘留在網格中。
func (s *CastleSystem) removeWarNpc(npc *world.NpcInfo) {
	if npc.Dead {
		if npc.DeleteTimer > 0 {
			s.deps.World.NpcCorpseCleanup(npc)
		}
	} else if s.deps.MapData != nil {
		s.deps.MapData.SetImpassable(npc.MapID, npc.X, npc.Y, false)
	}
	s.deps.World.RemoveNpc(npc.ID)

	nearby := s.deps.World.GetNearbyPlayersInShow(npc.X, npc.Y, npc.MapID, 0, npc.ShowID)
	handler.BroadcastToPlayers(nearby, handler.BuildRemoveObject(npc.ID))
}

// spawnWarNpc 依模板在指定座標生成戰爭物件（Java: L1WarSpawn.SpawnWarObject），
// 並向附近玩家送出 NPC 顯示封包。
func (s *CastleSystem) spawnWarNpc(npcID int32, x, y int32, mapID int16) *world.NpcInfo {
	tmpl := s.deps.Npcs.Get(npcID)
	if tmpl == nil {
		if s.deps.Log != nil {
			s.deps.Log.Warn("戰爭物件 NPC 模板不存在", zap.Int32("npc_id", npcID))
		}
		return nil
	}
	npc := newRuntimeNpcFromTemplate(tmpl, x, y, mapID, 0, 0, 0, s.deps.SprTable)
	s.deps.World.AddNpc(npc)
	if s.deps.MapData != nil {
		s.deps.MapData.SetImpassable(mapID, x, y, true)
	}
	nearby := s.deps.World.GetNearbyPlayers(x, y, mapID, 0)
	for _, p := range nearby {
		handler.SendNpcPack(p.Session, npc)
	}
	return npc
}

// SpawnWarFlags 攻城戰開始時沿城堡戰爭區邊界生成戰爭旗（NPC 81122）。
// Java: L1WarSpawn.SpawnFlag — 沿邊界每 8 格生成一個。
func (s *CastleSystem) SpawnWarFlags(castleID int32) {
	if s.deps.Castles == nil || s.deps.Npcs == nil {
		return
	}
	castleCfg := s.deps.Castles.Get(castleID)
	if castleCfg == nil {
		return
	}
	flagTmpl := s.deps.Npcs.Get(81122)
	if flagTmpl == nil {
		return
	}

	area := castleCfg.WarArea
	// 沿四邊每 8 格生成旗幟
	for x := area.X1; x <= area.X2; x += 8 {
		s.spawnFlag(flagTmpl, x, area.Y1, area.Map)
		s.spawnFlag(flagTmpl, x, area.Y2, area.Map)
	}
	for y := area.Y1 + 8; y < area.Y2; y += 8 {
		s.spawnFlag(flagTmpl, area.X1, y, area.Map)
		s.spawnFlag(flagTmpl, area.X2, y, area.Map)
	}
}

// spawnFlag 生成單個戰爭旗。
func (s *CastleSystem) spawnFlag(tmpl *data.NpcTemplate, x, y int32, mapID int16) {
	flag := &world.NpcInfo{
		ID:         world.NextNpcID(),
		NpcID:      81122,
		Impl:       tmpl.Impl,
		GfxID:      tmpl.GfxID,
		Name:       tmpl.Name,
		NameID:     tmpl.NameID,
		X:          x,
		Y:          y,
		MapID:      mapID,
		HP:         1,
		MaxHP:      1,
		SpawnX:     x,
		SpawnY:     y,
		SpawnMapID: mapID,
	}
	s.deps.World.AddNpc(flag)
}

// ClearWarFlags 攻城戰結束時清除戰爭旗（NPC 81122）。
func (s *CastleSystem) ClearWarFlags(castleID int32) {
	if s.deps.Castles == nil {
		return
	}
	// 收集需刪除的旗幟
	var toRemove []int32
	for _, npc := range s.deps.World.NpcList() {
		if npc.NpcID == 81122 {
			if s.deps.Castles.CheckInWarArea(castleID, npc.X, npc.Y, npc.MapID) {
				toRemove = append(toRemove, npc.ID)
			}
		}
	}
	for _, id := range toRemove {
		s.deps.World.RemoveNpc(id)
	}
}

// getCastleIDByTower 依守護塔 NPC 座標推算所屬城堡。
func (s *CastleSystem) getCastleIDByTower(npc *world.NpcInfo) int32 {
	if s.deps.Castles == nil {
		return 0
	}
	return s.deps.Castles.GetCastleIDByArea(npc.X, npc.Y, npc.MapID)
}

// isOwner 驗證玩家是否為指定城堡的城主公會盟主。
func (s *CastleSystem) isOwner(player *world.PlayerInfo, castleID int32) bool {
	ci := s.castles[castleID]
	if ci == nil || ci.OwnerClanID == 0 {
		return false
	}
	if player.ClanID != ci.OwnerClanID {
		return false
	}
	return player.ClanRank == 0 || player.ClanRank == 1
}

// --- 投石車系統（Java L1CatapultInstance + C_NPCAction 投石器段落）---

// catapultTarget 砲彈目標區域定義。
type catapultTarget struct {
	baseX    int32
	baseY    int32
	randW    int32
	randH    int32
	effectID int32
	silence  bool // true=沉默砲彈, false=普通砲彈
}

// catapultTargets 砲彈指令 → 目標區域映射（Java C_NPCAction.java:3636-3726）。
// 指令格式："0-N" = 普通砲彈, "1-N" = 沉默砲彈。
var catapultTargets = map[string]catapultTarget{
	// 奇巖攻擊方 (cgirana) — NPC 90331/90332
	"0-5":  {33629, 32730, 6, 4, 12205, false}, // 往外城門方向
	"0-6":  {33629, 32698, 8, 4, 12205, false}, // 往內城門方向
	"0-7":  {33629, 32675, 6, 6, 12205, false}, // 往守護塔方向
	"1-16": {33629, 32730, 6, 4, 12205, true},  // 外城門沉默
	"1-17": {33629, 32698, 8, 4, 12205, true},  // 內城門前沉默
	"1-18": {33626, 32704, 7, 4, 12205, true},  // 內城門左沉默
	"1-19": {33632, 32704, 7, 4, 12205, true},  // 內城門右沉默
	"1-20": {33629, 32675, 6, 6, 12205, true},  // 守護塔沉默
	// 奇巖防守方 (cgirand) — NPC 90333/90334
	"0-10": {33629, 32735, 6, 4, 12193, false}, // 往外城門方向
	// 肯特攻擊方 (ckenta) — NPC 90327/90328
	"0-1":  {33106, 32768, 5, 5, 12201, false}, // 往外城門方向
	"0-2":  {33164, 32776, 8, 9, 12201, false}, // 往守護塔方向
	"1-11": {33106, 32768, 5, 5, 12201, true},  // 外城門沉默
	"1-12": {33112, 32768, 5, 5, 12201, true},  // 外城門後沉默
	"1-13": {33164, 32785, 8, 9, 12201, true},  // 守護塔右沉默
	// 肯特防守方 (ckentd) — NPC 90329/90330
	"0-8": {33106, 32768, 5, 5, 12197, false}, // 往外城門方向
	// 妖堡攻擊方 (corca) — NPC 90335/90336
	"0-3":  {32792, 32321, 7, 4, 12205, false}, // 往外城門方向
	"0-4":  {32794, 32281, 8, 8, 12205, false}, // 往守護塔方向
	"1-14": {32792, 32321, 7, 4, 12205, true},  // 外城門沉默
	"1-15": {32794, 32281, 8, 8, 12205, true},  // 守護塔沉默
	// 妖堡防守方 (corcd) — NPC 90337
	"0-9": {32792, 32321, 7, 4, 12193, false}, // 往外城門方向
}

// catapultCastleNpcBase 城堡→投石車 NPC ID 起始對照。
// 每座城堡的投石車 NPC 依 catapults 配列順序依序遞增。
var catapultCastleNpcBase = map[int32]int32{
	1: 90327, // 肯特: 90327-90330 (4台)
	2: 90335, // 妖魔: 90335-90337 (3台)
	4: 90331, // 奇巖: 90331-90334 (4台)
}

// catapultHTMLMap 投石車 NPC ID → 對話 HTML ID。
var catapultHTMLMap = map[int32]string{
	90327: "ckenta", // 肯特攻擊
	90328: "ckenta",
	90329: "ckentd", // 肯特防守
	90330: "ckentd",
	90331: "cgirana", // 奇巖攻擊
	90332: "cgirana",
	90333: "cgirand", // 奇巖防守
	90334: "cgirand",
	90335: "corca", // 妖堡攻擊
	90336: "corca",
	90337: "corcd", // 妖堡防守
}

// IsCatapultAttacker 判斷投石車 NPC 是否為攻擊方。
func (s *CastleSystem) IsCatapultAttacker(npcID int32) bool {
	switch npcID {
	case 90327, 90328, 90331, 90332, 90335, 90336:
		return true
	}
	return false
}

// CanDamageCatapult 檢查是否可以攻擊投石車。
// Java: L1CatapultInstance.receiveDamage — 必須在攻城戰中、攻擊者已宣戰。
func (s *CastleSystem) CanDamageCatapult(player *world.PlayerInfo, npc *world.NpcInfo) bool {
	if s.deps.Castles == nil {
		return false
	}
	castleID := s.deps.Castles.GetCastleIDByArea(npc.X, npc.Y, npc.MapID)
	if castleID == 0 {
		return false
	}

	ci := s.castles[castleID]
	if ci == nil || !ci.IsWar {
		return false
	}

	if player.ClanID == 0 {
		return false
	}

	// 城盟不攻擊自家防守投石車
	if player.ClanID == ci.OwnerClanID && !s.IsCatapultAttacker(npc.NpcID) {
		return false
	}

	// 攻擊方不攻擊自方攻擊投石車
	if player.ClanID != ci.OwnerClanID && s.IsCatapultAttacker(npc.NpcID) {
		return false
	}

	// 必須已宣戰（與 CanDamageTower 相同邏輯）
	if s.deps.War != nil && ci.OwnerClanID != 0 {
		clan := s.deps.World.Clans.GetClan(player.ClanID)
		defenceClan := s.deps.World.Clans.GetClan(ci.OwnerClanID)
		if clan != nil && defenceClan != nil {
			if !s.deps.War.IsWar(clan.ClanName, defenceClan.ClanName) {
				return false
			}
		}
	}

	return true
}

// SpawnCatapults 生成指定城堡的投石車（Java: ServerWarExecutor.spawnCatapults）。
func (s *CastleSystem) SpawnCatapults(castleID int32) {
	if s.deps.Castles == nil || s.deps.Npcs == nil {
		return
	}
	castleCfg := s.deps.Castles.Get(castleID)
	if castleCfg == nil || len(castleCfg.Catapults) == 0 {
		return
	}

	baseNpcID, ok := catapultCastleNpcBase[castleID]
	if !ok {
		return
	}

	for i, cp := range castleCfg.Catapults {
		npcID := baseNpcID + int32(i)
		tmpl := s.deps.Npcs.Get(npcID)
		if tmpl == nil {
			continue
		}

		npc := &world.NpcInfo{
			ID:         world.NextNpcID(),
			NpcID:      npcID,
			Impl:       tmpl.Impl,
			GfxID:      tmpl.GfxID,
			Name:       tmpl.Name,
			NameID:     tmpl.NameID,
			X:          cp.X,
			Y:          cp.Y,
			MapID:      cp.Map,
			HP:         int32(tmpl.HP),
			MaxHP:      int32(tmpl.HP),
			SpawnX:     cp.X,
			SpawnY:     cp.Y,
			SpawnMapID: cp.Map,
			Level:      int16(tmpl.Level),
		}
		s.deps.World.AddNpc(npc)
	}

	if s.deps.Log != nil {
		s.deps.Log.Info("投石車已生成",
			zap.Int32("castle_id", castleID),
			zap.Int("count", len(castleCfg.Catapults)),
		)
	}
}

// ClearCatapults 清除指定城堡的投石車。
func (s *CastleSystem) ClearCatapults(castleID int32) {
	if s.deps.Castles == nil {
		return
	}
	var toRemove []int32
	for _, npc := range s.deps.World.NpcList() {
		if npc.Impl == "L1Catapult" {
			if s.deps.Castles.CheckInWarArea(castleID, npc.X, npc.Y, npc.MapID) {
				toRemove = append(toRemove, npc.ID)
			}
		}
	}
	for _, id := range toRemove {
		s.deps.World.RemoveNpc(id)
	}
}

// HandleCatapultAction 投石車砲彈發射（Java C_NPCAction.ShellDamage / ShellsSilence）。
func (s *CastleSystem) HandleCatapultAction(sess *net.Session, player *world.PlayerInfo, npc *world.NpcInfo, action string) {
	tgt, ok := catapultTargets[action]
	if !ok {
		return
	}

	curtime := time.Now().Unix()

	// 冷卻檢查（10 秒共用冷卻）
	if (npc.ShellDamageTime+10 > curtime) || (npc.ShellSilenceTime+10 > curtime) {
		handler.SendServerMessage(sess, 3680) // 需要再次上子彈
		return
	}

	// 炸彈消耗檢查
	bomb := player.Inv.FindByItemID(82500)
	if bomb == nil || bomb.Count < 1 {
		handler.SendSystemMessage(sess, "投石器需要炸彈才能發射。")
		return
	}

	// 消耗炸彈
	bomb.Count--
	if bomb.Count <= 0 {
		player.Inv.RemoveItem(bomb.ObjectID, 1)
		handler.SendRemoveInventoryItem(sess, bomb.ObjectID)
	} else {
		handler.SendItemCountUpdate(sess, bomb)
	}
	player.Dirty = true

	// 記錄冷卻時間
	if tgt.silence {
		npc.ShellSilenceTime = curtime
	} else {
		npc.ShellDamageTime = curtime
	}

	// 計算著彈點（隨機偏移）
	hitX := tgt.baseX + rand.Int31n(tgt.randW)
	hitY := tgt.baseY + rand.Int31n(tgt.randH)

	// 建構視覺封包（S_EffectLocation + S_DoActionGfx）
	effectPkt := buildEffectLocation(hitX, hitY, tgt.effectID)
	actionPkt := handler.BuildActionGfx(npc.ID, 1) // 發射動畫

	// 廣播給所有攻城區域內玩家 + 範圍傷害/沉默
	s.deps.World.AllPlayers(func(p *world.PlayerInfo) {
		if !s.isInAnyCastleWarArea(p) {
			return
		}

		// 視覺效果
		p.Session.Send(effectPkt)
		p.Session.Send(actionPkt)

		// 範圍判定（±2 格）
		if p.X >= hitX-2 && p.X <= hitX+2 && p.Y >= hitY-2 && p.Y <= hitY+2 {
			if tgt.silence {
				// 沉默砲彈：施加 15 秒沉默
				s.applyCatapultSilence(p)
			} else {
				// 普通砲彈：300 固定傷害
				s.applyCatapultDamage(p, npc)
			}
		}
	})
}

// applyCatapultDamage 投石車普通砲彈傷害（300 固定）。
func (s *CastleSystem) applyCatapultDamage(target *world.PlayerInfo, npc *world.NpcInfo) {
	if target.Dead {
		return
	}
	target.ApplyDamage(300)
	target.Dirty = true
	if target.HP <= 0 {
		target.HP = 0
		handler.SendHpUpdate(target.Session, target)
		// 傷害動畫
		dmgPkt := handler.BuildActionGfx(target.CharID, 2) // ACTION_Damage
		target.Session.Send(dmgPkt)
		nearby := s.deps.World.GetNearbyPlayers(target.X, target.Y, target.MapID, target.Session.ID)
		handler.BroadcastToPlayers(nearby, dmgPkt)
		if s.deps.Death != nil {
			s.deps.Death.KillPlayer(target)
		}
		return
	}
	handler.SendHpUpdate(target.Session, target)
	// 傷害動畫
	dmgPkt := handler.BuildActionGfx(target.CharID, 2)
	target.Session.Send(dmgPkt)
	nearby := s.deps.World.GetNearbyPlayers(target.X, target.Y, target.MapID, target.Session.ID)
	handler.BroadcastToPlayers(nearby, dmgPkt)
}

// applyCatapultSilence 投石車沉默砲彈效果（15 秒沉默）。
// Java: player.setSkillEffect(SILENCE, 15000) + S_SkillSound(2177)。
func (s *CastleSystem) applyCatapultSilence(target *world.PlayerInfo) {
	if target.Dead {
		return
	}
	target.Silenced = true
	target.CatapultSilenceEnd = time.Now().Unix() + 15

	// 沉默音效（GFX 2177）
	effectPkt := handler.BuildSkillEffect(target.CharID, 2177)
	target.Session.Send(effectPkt)
	nearby := s.deps.World.GetNearbyPlayers(target.X, target.Y, target.MapID, target.Session.ID)
	handler.BroadcastToPlayers(nearby, effectPkt)
}

// isInAnyCastleWarArea 檢查玩家是否在任何城堡攻城區域內。
func (s *CastleSystem) isInAnyCastleWarArea(p *world.PlayerInfo) bool {
	if s.deps.Castles == nil {
		return false
	}
	for _, ci := range s.castles {
		if ci.IsWar && s.deps.Castles.CheckInWarArea(ci.CastleID, p.X, p.Y, p.MapID) {
			return true
		}
	}
	return false
}

// buildEffectLocation 建構 S_EffectLocation 封包（Java: S_EffectLocation.java）。
// writeC(106) + writeH(x) + writeH(y) + writeH(gfxId)。
func buildEffectLocation(x, y, gfxID int32) []byte {
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_EFFECTLOCATION)
	w.WriteH(uint16(x))
	w.WriteH(uint16(y))
	w.WriteH(uint16(gfxID))
	return w.Bytes()
}
