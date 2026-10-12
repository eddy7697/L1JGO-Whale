package handler

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// parseInt32 解析字串為 int32，失敗回傳 0。
func parseInt32(s string) int32 {
	s = strings.TrimSpace(s)
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return int32(v)
}

// HandleNpcAction processes C_HACTION (opcode 125) — player clicks a button in NPC dialog.
// Also handles S_Message_YN (yes/no dialog) responses — client sends objectID=yesNoCount.
// The action string determines what to do: "buy", "sell", "teleportURL", etc.
func HandleNpcAction(sess *net.Session, r *packet.Reader, deps *Deps) {
	objID := r.ReadD()
	action := r.ReadS()

	deps.Log.Debug("C_NpcAction",
		zap.Int32("objID", objID),
		zap.String("action", action),
	)

	player := deps.World.GetBySession(sess.ID)
	if player == nil {
		return
	}

	// Clear pending state — any new NPC interaction overrides
	player.PendingCraftAction = ""
	player.FireSmithNpcObjID = 0
	player.PendingAuctionHouseID = 0
	player.PendingInnNpcObjID = 0
	// 任何 NPC 動作都會關閉動態對話的即時刷新（避免按鈕點完後 server 仍持續送舊狀態）
	hadLive := player.LiveDialog != nil
	player.LiveDialog = nil
	if hadLive {
		deps.Log.Info("dialog: cleared live dialog by action",
			zap.String("action", action), zap.Int32("char_id", player.CharID))
	}

	// --- Summon ring selection: numeric string response from "summonlist" dialog ---
	// Java: L1ActionPc.java checks cmd.matches("[0-9]+") && isSummonMonster().
	if player.SummonSelectionMode && isNumericString(action) {
		HandleSummonRingSelection(sess, player, action, deps)
		return
	}
	if player.PendingPolySkill && deps.Polymorph != nil {
		deps.Polymorph.UsePolySkill(sess, player, action)
		return
	}

	// --- Companion entity control (summon/pet before NPC lookup) ---
	if sum := deps.World.GetSummon(objID); sum != nil {
		if sum.OwnerCharID == player.CharID {
			handleSummonAction(sess, player, sum, strings.ToLower(action), deps)
		}
		return
	}
	if pet := deps.World.GetPet(objID); pet != nil {
		if pet.OwnerCharID == player.CharID && deps.PetLife != nil {
			deps.PetLife.HandlePetAction(sess, player, pet, strings.ToLower(action))
		}
		return
	}

	if objID == player.CharID {
		switch strings.ToLower(action) {
		case "teleport_open":
			player.NoAskMassTeleport = false
			sendNormalChat(sess, 0, "\\aD您已開啟了\\aI集體傳送術的詢問！")
			return
		case "teleport_close":
			player.NoAskMassTeleport = true
			sendNormalChat(sess, 0, "\\aG您已關閉了\\aE集體傳送術的詢問！")
			return
		}
	}

	npc := deps.World.GetNpc(objID)
	if npc == nil {
		// Not an NPC — check for S_Message_YN (yes/no dialog) response
		if player.PendingYesNoType != 0 {
			lAction := strings.ToLower(action)
			accepted := lAction != "no" && lAction != "n"
			handleYesNoResponse(sess, player, accepted, deps)
		}
		return
	}
	dx := int32(math.Abs(float64(player.X - npc.X)))
	dy := int32(math.Abs(float64(player.Y - npc.Y)))
	if dx > 5 || dy > 5 {
		return
	}

	// L1Crown — 王冠點擊 → 城堡主權轉移（Java: L1CrownInstance.onAction）
	if npc.Impl == "L1Crown" && deps.Castle != nil {
		deps.Castle.HandleCrownClick(sess, player, npc)
		return
	}

	// L1Catapult — 投石車操作（Java: C_NPCAction.java:3625-3727）
	if npc.Impl == "L1Catapult" && deps.Castle != nil {
		handleCatapultAction(sess, player, npc, action, deps)
		return
	}

	// Java C_NPCAction.java:183-187 — L1AuctionBoard 封包多一個 readS()，
	// 合併為 "cmd,extra"（如 "select,262145"）。不讀會導致後續封包位移錯誤。
	if npc.Impl == "L1AuctionBoard" {
		extra := r.ReadS()
		if extra != "" {
			action = action + "," + extra
		}
		if handleAuctionAction(sess, player, objID, action, deps) {
			return
		}
		return
	}

	lowerAction := strings.ToLower(action)

	// Auto-cancel trade when interacting with NPC
	cancelTradeIfActive(player, deps)

	// 動態 HTML 對話（YAML+HTM 引擎）action 分派
	// 若該 NPC 有 _routes.yaml 且 lowerAction 有對應 on_action 定義，直接交給 dialog 引擎
	if TryDispatchAction(sess, player, objID, lowerAction, deps) {
		return
	}

	// L1Housekeeper（管家 NPC）— 管家動作優先處理
	if npc.Impl == "L1Housekeeper" {
		if handleHousekeeperAction(sess, player, objID, npc.NpcID, lowerAction, deps) {
			return
		}
	}

	// 旅館 NPC — "room" / "hall" / "return" / "enter" 動作
	if handleInnAction(sess, player, objID, npc.NpcID, lowerAction, deps) {
		return
	}

	// Paginated teleporter NPC (e.g., NPC 91053): route all actions to paged handler
	if deps.TeleportPages != nil && deps.TeleportPages.IsPageTeleportNpc(npc.NpcID) {
		handlePagedTeleportAction(sess, player, npc, action, deps)
		return
	}

	switch lowerAction {
	case "buy":
		if npc.Impl == "L1Cn" {
			handleCnShopBuy(sess, npc.NpcID, objID, deps)
		} else {
			handleShopBuy(sess, npc.NpcID, objID, deps)
		}
	case "sell":
		if npc.Impl == "L1Cn" {
			handleCnShopSell(sess, npc.NpcID, objID, deps)
		} else {
			handleShopSell(sess, npc.NpcID, objID, deps)
		}
	case "poweritem":
		handlePowerItemList(sess, npc.NpcID, objID, deps)
	case "buyskill":
		openSpellShop(sess, deps)
	case "teleporturl", "teleporturla", "teleporturlb", "teleporturlc",
		"teleporturld", "teleporturle", "teleporturlf", "teleporturlg",
		"teleporturlh", "teleporturli", "teleporturlj", "teleporturlk":
		handleTeleportURLGeneric(sess, npc.NpcID, objID, action, deps)

	// Warehouse — 個人帳號倉庫
	case "retrieve":
		deps.Warehouse.OpenWarehouse(sess, player, objID, WhTypePersonal)
	case "deposit":
		deps.Warehouse.OpenWarehouseDeposit(sess, player, objID, WhTypePersonal)

	// Warehouse — 角色專屬倉庫（Java: retrieve-char → S_RetrieveChaList type=18）
	case "retrieve-char":
		deps.Warehouse.OpenWarehouse(sess, player, objID, WhTypeCharacter)

	// Warehouse — 精靈倉庫
	case "retrieve-elven":
		deps.Warehouse.OpenWarehouse(sess, player, objID, WhTypeElf)
	case "deposit-elven":
		deps.Warehouse.OpenWarehouseDeposit(sess, player, objID, WhTypeElf)

	// Warehouse — 血盟倉庫（含權限驗證 + 單人鎖定）
	case "retrieve-pledge":
		deps.Warehouse.OpenClanWarehouse(sess, player, objID)
	case "deposit-pledge":
		deps.Warehouse.OpenClanWarehouse(sess, player, objID) // 同 retrieve，客戶端內建 tab 處理
	case "history":
		// 血盟倉庫歷史記錄（Java: S_PledgeWarehouseHistory）
		if player.ClanID > 0 {
			deps.Warehouse.SendClanWarehouseHistory(sess, player.ClanID)
		}

	// EXP recovery / PK redemption (stub)
	case "exp":
		sendHypertext(sess, objID, "expr")
	case "pk":
		sendHypertext(sess, objID, "pkr")

	// ---------- NPC Services (data-driven from npc_services.yaml) ----------

	case "haste":
		handleNpcHaste(sess, player, npc, deps)
	case "0":
		handleNpcActionZero(sess, player, npc, objID, deps)
	case "fullheal":
		handleNpcFullHeal(sess, player, npc, deps)
	case "encw":
		handleNpcWeaponEnchant(sess, player, deps)
	case "enca":
		handleNpcArmorEnchant(sess, player, deps)

	// ---------- 火神精煉系統 ----------
	// 380 客戶端原生介面：S_EquipmentWindow type 48/49 開啟拖放 UI
	// 客戶端拖入裝備後送 C_PledgeContent (opcode 78) data=13/14 → handleRefineResolve / handleRefineTransform

	case "itemresolve":
		// 火神精煉（type 48 拖放 UI）— Java 380: S_EquipmentWindow(48, npcId)
		sendRefineUI(sess, npc.ID, 48)
	case "itemtransform":
		// 火神合成（type 49 拖放 UI）— Java 380: S_EquipmentWindow(49, npcId)
		sendRefineUI(sess, npc.ID, 49)
	case "request firecrystal":
		// 火神熔煉（商店賣出格式）— Java: Npc_FireSmith → S_ShopBuyListFireSmith
		// 與拖放 UI 並行存在的回退路徑（NPC 111414 火神煉化工匠用）
		sendFireSmithSellList(sess, player, npc, deps)
	case "request craft itemblend":
		// 火神工匠 ItemBlend 配方瀏覽（拖放 UI 之外的回退查詢介面）
		sendCraftItemBlend(sess, player, npc, deps, 0)

	// ---------- 火神工匠系統（Java: L1Blend / 道具製造系統DB化） ----------

	case "request craft":
		handleRequestCraft(sess, player, npc, deps)
	case "confirm craft":
		handleConfirmCraft(sess, player, npc, deps)
	case "cancel craft":
		// 循環瀏覽下一個配方（ItemBlend 模板用）
		if player.PendingCraftNpcID != 0 && deps.ItemMaking != nil {
			nextIdx := player.PendingCraftIndex + 1
			recipes := deps.ItemMaking.GetByNpcID(player.PendingCraftNpcID)
			if nextIdx < len(recipes) {
				sendCraftItemBlend(sess, player, npc, deps, nextIdx)
			} else {
				// 已到最後一個配方，回到第一個
				sendCraftItemBlend(sess, player, npc, deps, 0)
			}
		} else {
			player.PendingCraftKey = ""
			player.PendingCraftNpcID = 0
			player.PendingCraftIndex = 0
			player.CraftTradeTick = 0
		}

	// "ent" 動作 — 多個 NPC 共用，依 NPC ID 分派
	// Java: C_NPCAction.java 對 "ent" 按 npcId 做 if/else
	case "ent":
		switch npc.NpcID {
		case 80085: // 幽靈之家管理人杜烏 → 鬼屋副本
			enterHauntedHouse(sess, player, deps)
		default: // NPC 71264 回憶蠟燭嚮導等 → 角色重置
			StartCharReset(sess, player, deps)
		}

	// 副本框架 demo（MISS-P0-003 Stage E）
	case "enter_demo_dungeon":
		enterDemoDungeon(sess, player, deps)
	case "exit_demo_dungeon":
		exitDemoDungeon(sess, player, deps)

	// ---------- 城堡管理 NPC 動作 ----------

	case "inex":
		// 查詢城堡資金（Java: S_ServerMessage 309）
		handleCastleInex(sess, player, deps)
	case "tax":
		// 開啟稅率設定 UI（Java: S_TaxRate）
		handleCastleTax(sess, player, deps)
	case "withdrawal":
		// 開啟寶庫領出視窗（Java: S_Drawal）
		handleCastleWithdrawal(sess, player, deps)
	case "cdeposit":
		// 開啟寶庫存入視窗（Java: S_Deposit）
		handleCastleDeposit(sess, player, deps)
	case "castlegate":
		// 城門修復（非攻城戰時）
		handleCastleGateRepair(sess, player, deps)
	case "openigate":
		// 開啟內城門
		handleCastleGateOpen(sess, player, deps)
	case "closeigate":
		// 關閉內城門
		handleCastleGateClose(sess, player, deps)
	case "askwartime":
		// 查詢攻城戰時間（Java: C_NPCAction.java:742）
		handleAskWarTime(sess, npc, deps)

	// Close dialog — empty string (原版 L1J 標準) 或 @dynamic 對話的具名 close
	// （客戶端 @dynamic 路徑下「離開」鈕用 action="cancel" / "close"，避免 action="" 的
	// 「沒掛 action」狀態與「明確關閉」歧義，詳見 handler/dynamic_dialog.go 註解）
	case "", "cancel", "close":
		// Do nothing — dialog closes

	default:
		// ---------- NPC 專屬動作（依 NPC ID 分派） ----------

		// 寵物比賽管理人（NPC 80088）— Java: Npc_PetWar.action()
		// 動作格式: "ent,,<amuletObjID>"（客戶端 petmatcher HTML 生成）
		if npc.NpcID == 80088 && strings.HasPrefix(lowerAction, "ent") {
			handlePetMatchEntry(sess, player, action, deps)
			return
		}

		// 排名 NPC（80026-80029）動作處理
		if isRankingNpc(npc.NpcID) {
			if handleRankingNpcAction(sess, player, objID, npc, action, deps) {
				return
			}
		}

		if npc.NpcID == 81445 { // 欄位開放專家 史奈普
			handleSlotNpc(sess, player, objID, lowerAction, deps)
			return
		}

		// 火龍窟副本（MISS-P2-016）— 三個 NPC 全部由 YAML+HTM 對話引擎處理：
		//   46180 漢          data/dialogs/46180_han/         (按鈕 a → 給漢的袋子)
		//   46181 愛德納斯    data/dialogs/46181_eldnas/      (按鈕 a → 入副本 148)
		//   46164 死亡騎士    data/dialogs/46164_death_knight/(按鈕 enter → 給 850)
		// 上面 TryDispatchAction 已處理，不需要 switch fallback。

		// Check teleport destinations (handles "teleport xxx" and other
		// action names like "Strange21", "goto battle ring", "a"/"b"/etc.)
		if deps.Teleports.Get(npc.NpcID, action) != nil {
			handleTeleport(sess, player, npc.NpcID, action, deps)
			return
		}

		// Check if this is a polymorph NPC form (data-driven from npc_services.yaml)
		if polyID := deps.NpcServices.GetPolyForm(lowerAction); polyID > 0 {
			handleNpcPoly(sess, player, polyID, deps)
			return
		}

		// 任務 NPC 動作（YAML 驅動的任務對話系統）
		if handleQuestNpcAction(sess, player, objID, npc.NpcID, action, deps) {
			return
		}

		// 物品升級合成（火神煉化合成系統 DB 化）
		// Java: L1UpgradeItem — 依 NPC ID + 動作字串查找升級定義
		if deps.ItemUpgrades != nil {
			if upg := deps.ItemUpgrades.Get(npc.NpcID, action); upg != nil {
				handleItemUpgrade(sess, player, upg, deps)
				return
			}
		}

		// 火神系統配方（NPC 專屬，action = A-Z, a1-a17）
		// Java: craftkey = npcid + action → L1BlendTable.getTemplate(craftkey) → ShowCraftHtml
		if deps.ItemMaking != nil && deps.Craft != nil {
			if recipe := deps.ItemMaking.GetByNpcAction(npc.NpcID, action); recipe != nil {
				handleCraftSelect(sess, player, npc, recipe, deps)
				return
			}
			// 簡易配方（無 NPC 綁定）：直接執行
			if recipe := deps.ItemMaking.Get(action); recipe != nil {
				deps.Craft.HandleCraftEntry(sess, player, npc, recipe, action)
				return
			}
		}

		deps.Log.Debug("unhandled NPC action",
			zap.String("action", action),
			zap.Int32("npc_id", npc.NpcID),
		)
	}
}

// handleShopBuy — player presses "Buy" — show items NPC sells.
// Sends S_SELL_LIST (opcode 70) = S_ShopSellList in Java (items NPC sells to player).
func handleShopBuy(sess *net.Session, npcID, objID int32, deps *Deps) {
	shop := deps.Shops.Get(npcID)
	if shop == nil || len(shop.SellingItems) == 0 {
		sendNoSell(sess, objID)
		return
	}

	w := packet.NewWriterWithOpcode(packet.S_OPCODE_SELL_LIST) // opcode 70
	w.WriteD(objID)
	w.WriteH(uint16(len(shop.SellingItems)))

	for i, si := range shop.SellingItems {
		itemInfo := deps.Items.Get(si.ItemID)
		name := fmt.Sprintf("item#%d", si.ItemID)
		gfxID := int32(0)
		if itemInfo != nil {
			name = itemInfo.Name
			gfxID = itemInfo.InvGfx
		}

		// Append pack count to name if > 1
		if si.PackCount > 1 {
			name = fmt.Sprintf("%s (%d)", name, si.PackCount)
		}

		price := si.SellingPrice

		w.WriteD(int32(i))      // order index
		w.WriteH(uint16(gfxID)) // inventory graphic ID
		w.WriteD(price)         // price
		w.WriteS(name)          // item name

		// Status bytes: show item stats (damage, AC, class restrictions) like Java
		if itemInfo != nil {
			status := buildShopStatusBytes(itemInfo)
			w.WriteC(byte(len(status)))
			w.WriteBytes(status)
		} else {
			w.WriteC(0)
		}
	}

	w.WriteH(0x0007) // currency type: 7 = adena

	sess.Send(w.Bytes())
}

// handleShopSell — player presses "Sell" — show items NPC will buy from player.
// Sends S_SHOP_SELL_LIST (opcode 65) with assessed prices for player's items.
func handleShopSell(sess *net.Session, npcID, objID int32, deps *Deps) {
	shop := deps.Shops.Get(npcID)
	if shop == nil || len(shop.PurchasingItems) == 0 {
		sendNoSell(sess, objID)
		return
	}

	player := deps.World.GetBySession(sess.ID)
	if player == nil || player.Inv == nil {
		sendNoSell(sess, objID)
		return
	}

	// Build purchasing price lookup
	purchMap := make(map[int32]int32, len(shop.PurchasingItems))
	for _, pi := range shop.PurchasingItems {
		purchMap[pi.ItemID] = pi.PurchasingPrice
	}

	// Find sellable items in player's inventory
	type assessedItem struct {
		objectID int32
		price    int32
	}
	var items []assessedItem
	for _, invItem := range player.Inv.Items {
		price, ok := purchMap[invItem.ItemID]
		if !ok {
			continue
		}
		if invItem.EnchantLvl != 0 || invItem.Bless >= 128 {
			continue // skip enchanted/sealed
		}
		items = append(items, assessedItem{objectID: invItem.ObjectID, price: price})
	}

	if len(items) == 0 {
		sendNoSell(sess, objID)
		return
	}

	w := packet.NewWriterWithOpcode(packet.S_OPCODE_SHOP_SELL_LIST) // opcode 65
	w.WriteD(objID)
	w.WriteH(uint16(len(items)))
	for _, it := range items {
		w.WriteD(it.objectID)
		w.WriteD(it.price)
	}
	w.WriteH(0x0007) // currency: adena
	sess.Send(w.Bytes())
}

// handleTeleportURLGeneric shows the NPC's teleport page with data values (prices).
// Handles teleportURL, teleportURLA, teleportURLB, etc.
func handleTeleportURLGeneric(sess *net.Session, npcID, objID int32, action string, deps *Deps) {
	// Look up HTML data (contains htmlID + data values for price display)
	htmlData := deps.TeleportHtml.Get(npcID, action)
	if htmlData != nil {
		sendHypertextWithData(sess, objID, htmlData.HtmlID, htmlData.Data)
		return
	}

	// Fallback: try NpcAction table for teleport_url / teleport_urla
	npcAction := deps.NpcActions.Get(npcID)
	if npcAction == nil {
		return
	}
	lowerAction := strings.ToLower(action)
	switch lowerAction {
	case "teleporturl":
		if npcAction.TeleportURL != "" {
			sendHypertext(sess, objID, npcAction.TeleportURL)
		}
	case "teleporturla":
		if npcAction.TeleportURLA != "" {
			sendHypertext(sess, objID, npcAction.TeleportURLA)
		}
	}
}

// sendHypertext sends S_HYPERTEXT (opcode 39) to show an HTML dialog (no data values).
func sendHypertext(sess *net.Session, objID int32, htmlID string) {
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_HYPERTEXT)
	w.WriteD(objID)
	w.WriteS(htmlID)
	w.WriteH(0x00)
	w.WriteH(0)
	sess.Send(w.Bytes())
}

// SendHypertext 開啟 HTML 對話框。Exported for system package usage.
func SendHypertext(sess *net.Session, objID int32, htmlID string) {
	sendHypertext(sess, objID, htmlID)
}

// sendHypertextWithData sends S_HYPERTEXT with data values injected into the HTML template.
// Data values replace %0, %1, %2... placeholders in the client's built-in HTML.
func sendHypertextWithData(sess *net.Session, objID int32, htmlID string, data []string) {
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_HYPERTEXT)
	w.WriteD(objID)
	w.WriteS(htmlID)
	if len(data) > 0 {
		w.WriteH(0x01) // has data flag
		w.WriteH(uint16(len(data)))
		for _, val := range data {
			w.WriteS(val)
		}
	} else {
		w.WriteH(0x00)
		w.WriteH(0)
	}
	sess.Send(w.Bytes())
}

// SendHypertextWithData 匯出 sendHypertextWithData — 供 system 套件傳送帶資料的 HTML。
func SendHypertextWithData(sess *net.Session, objID int32, htmlID string, data []string) {
	sendHypertextWithData(sess, objID, htmlID, data)
}

// sendNoSell sends S_HYPERTEXT with "nosell" HTML to indicate NPC doesn't trade.
func sendNoSell(sess *net.Session, objID int32) {
	sendHypertext(sess, objID, "nosell")
}

// handleTeleport processes a "teleport xxx" action from the NPC dialog.
// Looks up the destination, checks adena cost, and teleports the player.
func handleTeleport(sess *net.Session, player *world.PlayerInfo, npcID int32, action string, deps *Deps) {
	dest := deps.Teleports.Get(npcID, action)
	if dest == nil {
		deps.Log.Debug("teleport destination not found",
			zap.String("action", action),
			zap.Int32("npc_id", npcID),
		)
		return
	}

	// 委派給 NpcServiceSystem 處理扣費 + 傳送
	if deps.NpcSvc != nil {
		deps.NpcSvc.NpcTeleportWithCost(sess, player, dest, 0)
	}

	deps.Log.Info(fmt.Sprintf("玩家傳送  角色=%s  動作=%s  x=%d  y=%d  地圖=%d  花費=%d", player.Name, action, dest.X, dest.Y, dest.MapID, dest.Price))
}

// teleportPlayer moves a player to a new location with full AOI updates.
// Used by NPC teleport, death restart, GM commands, etc.
//
// Packet sequence matches Java Teleportation.actionTeleportation() exactly:
//  1. Remove from old location (broadcast S_REMOVE_OBJECT to old nearby)
//  2. Update world position
//  3. S_MapID — client loads new map
//  4. Broadcast S_OtherCharPacks to new nearby (they see us arrive)
//  5. S_OwnCharPack — self character at new position (live player data)
//  6. updateObject equivalent — send nearby players, NPCs, ground items to self
//  7. S_CharVisualUpdate — weapon/poly visual fix (LAST per Java)
//
// TeleportPlayer 處理完整傳送流程。Exported for system package usage.
func TeleportPlayer(sess *net.Session, player *world.PlayerInfo, x, y int32, mapID, heading int16, deps *Deps) {
	teleportPlayer(sess, player, x, y, mapID, heading, deps)
}

func teleportPlayer(sess *net.Session, player *world.PlayerInfo, x, y int32, mapID, heading int16, deps *Deps) {
	teleportPlayerInShow(sess, player, x, y, mapID, heading, player.ShowID, deps)
}

// teleportPlayerInShow changes instance only after removing the character from
// the old audience. GM recall/topc must enter the target's actual instance.
func teleportPlayerInShow(sess *net.Session, player *world.PlayerInfo, x, y int32, mapID, heading int16, showID int32, deps *Deps) {
	// 傳送時釋放血盟倉庫鎖定（Java: Teleportation.java 行 122-123）
	if player.ClanID != 0 {
		if clan := deps.World.Clans.GetClan(player.ClanID); clan != nil {
			if clan.WarehouseUsingCharID == player.CharID {
				clan.WarehouseUsingCharID = 0
			}
		}
	}

	// Reset move speed timer (teleport resets speed validation)
	player.LastMoveTime = 0

	// Clear old tile (for NPC pathfinding)
	if deps.MapData != nil {
		deps.MapData.SetImpassable(player.MapID, player.X, player.Y, false)
	}

	// ── 收集玩家擁有的同伴（傳送前）──
	ownedPets := deps.World.GetPetsByOwner(player.CharID)
	ownedSummons := deps.World.GetSummonsByOwner(player.CharID)
	ownedDolls := deps.World.GetDollsByOwner(player.CharID)
	ownedFollower := deps.World.GetFollowerByOwner(player.CharID)
	ownedHierarch := deps.World.GetHierarchByOwner(player.CharID)

	// 從舊位置附近玩家視野中移除同伴（Java: Teleportation.java removeKnownObject）
	for _, pet := range ownedPets {
		if pet.Dead {
			continue
		}
		oldViewers := deps.World.GetNearbyPlayersInShow(pet.X, pet.Y, pet.MapID, 0, pet.ShowID)
		removeData := BuildRemoveObject(pet.ID)
		for _, v := range oldViewers {
			if v.CharID != player.CharID {
				v.Session.Send(removeData)
			}
		}
	}
	for _, sum := range ownedSummons {
		if sum.Dead {
			continue
		}
		oldViewers := deps.World.GetNearbyPlayersInShow(sum.X, sum.Y, sum.MapID, 0, sum.ShowID)
		removeData := BuildRemoveObject(sum.ID)
		for _, v := range oldViewers {
			if v.CharID != player.CharID {
				v.Session.Send(removeData)
			}
		}
	}
	for _, doll := range ownedDolls {
		oldViewers := deps.World.GetNearbyPlayersInShow(doll.X, doll.Y, doll.MapID, 0, doll.ShowID)
		removeData := BuildRemoveObject(doll.ID)
		for _, v := range oldViewers {
			if v.CharID != player.CharID {
				v.Session.Send(removeData)
			}
		}
	}
	if ownedFollower != nil && !ownedFollower.Dead {
		oldViewers := deps.World.GetNearbyPlayersInShow(ownedFollower.X, ownedFollower.Y, ownedFollower.MapID, 0, ownedFollower.ShowID)
		removeData := BuildRemoveObject(ownedFollower.ID)
		for _, v := range oldViewers {
			if v.CharID != player.CharID {
				v.Session.Send(removeData)
			}
		}
	}
	if ownedHierarch != nil {
		oldViewers := deps.World.GetNearbyPlayersInShow(ownedHierarch.X, ownedHierarch.Y, ownedHierarch.MapID, 0, ownedHierarch.ShowID)
		removeData := BuildRemoveObject(ownedHierarch.ID)
		for _, v := range oldViewers {
			if v.CharID != player.CharID {
				v.Session.Send(removeData)
			}
		}
	}

	// 1. 舊位置附近玩家：移除我 + 解鎖我的格子
	oldNearby := deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, sess.ID, player.ShowID)
	for _, other := range oldNearby {
		SendRemoveObject(other.Session, player.CharID)
	}

	// 2. 更新世界狀態位置（Java: moveVisibleObject + setLocation）
	player.ShowID = showID
	for _, pet := range ownedPets {
		pet.ShowID = showID
	}
	for _, sum := range ownedSummons {
		sum.ShowID = showID
	}
	for _, doll := range ownedDolls {
		doll.ShowID = showID
	}
	if ownedFollower != nil {
		ownedFollower.ShowID = showID
	}
	if ownedHierarch != nil {
		ownedHierarch.ShowID = showID
	}
	deps.World.UpdatePosition(sess.ID, x, y, mapID, heading)

	// 標記新格子不可通行（NPC 尋路用）
	if deps.MapData != nil {
		deps.MapData.SetImpassable(mapID, x, y, true)
	}

	// ── 傳送同伴到新位置（Java: Teleportation.java 寵物跟隨移動）──
	// 方向偏移：將同伴分散在玩家周圍（避免疊在同一格）
	offsets := [4][2]int32{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	oi := 0
	for _, pet := range ownedPets {
		if pet.Dead {
			continue
		}
		ox, oy := offsets[oi%4][0], offsets[oi%4][1]
		oi++
		deps.World.TeleportPet(pet.ID, x+ox, y+oy, mapID, heading)
	}
	for _, sum := range ownedSummons {
		if sum.Dead {
			continue
		}
		ox, oy := offsets[oi%4][0], offsets[oi%4][1]
		oi++
		deps.World.TeleportSummon(sum.ID, x+ox, y+oy, mapID, heading)
	}
	for _, doll := range ownedDolls {
		ox, oy := offsets[oi%4][0], offsets[oi%4][1]
		oi++
		deps.World.TeleportDoll(doll.ID, x+ox, y+oy, mapID, heading)
	}
	if ownedFollower != nil && !ownedFollower.Dead {
		ox, oy := offsets[oi%4][0], offsets[oi%4][1]
		deps.World.TeleportFollower(ownedFollower.ID, x+ox, y+oy, mapID, heading)
	}
	if ownedHierarch != nil {
		ox, oy := offsets[oi%4][0], offsets[oi%4][1]
		deps.World.TeleportHierarch(ownedHierarch.ID, x+ox, y+oy, mapID, heading)
	}

	// 3. S_MapID（即使同地圖也要發——客戶端傳送需要；依目標地圖 underwater 設定送水的旗標）
	sendMapIDForPlayer(sess, player, int16(mapID), deps)

	// 重置 Known 集合（傳送 = 完全切換場景）
	if player.Known == nil {
		player.Known = world.NewKnownEntities()
	} else {
		player.Known.Reset()
	}

	// 4. 目的地附近玩家：顯示我 + 封鎖我的格子 + 填入 Known
	newNearby := deps.World.GetNearbyPlayersInShow(x, y, mapID, sess.ID, player.ShowID)
	for _, other := range newNearby {
		if !other.CanSeePlayer(player) {
			continue
		}
		SendPutObject(other.Session, player)
	}

	// 5. S_OwnCharPack
	sendOwnCharPackPlayer(sess, player)

	// 6. 發送附近實體給自己 + 封鎖格子 + 填入 Known
	for _, other := range newNearby {
		if !player.CanSeePlayer(other) {
			continue
		}
		SendPutObject(sess, other)
		player.Known.Players[other.CharID] = world.KnownPos{X: other.X, Y: other.Y}
	}

	nearbyNpcs := deps.World.GetNearbyNpcsInShow(x, y, mapID, player.ShowID)
	for _, npc := range nearbyNpcs {
		SendNpcPack(sess, npc)
		player.Known.Npcs[npc.ID] = world.KnownPos{X: npc.X, Y: npc.Y}
	}

	nearbyGnd := deps.World.GetNearbyGroundItemsInShow(x, y, mapID, player.ShowID)
	for _, g := range nearbyGnd {
		SendDropItem(sess, g)
		player.Known.GroundItems[g.ID] = world.KnownPos{X: g.X, Y: g.Y}
	}

	nearbyDoors := deps.World.GetNearbyDoors(x, y, mapID)
	for _, d := range nearbyDoors {
		SendDoorPerceive(sess, d)
		player.Known.Doors[d.ID] = world.KnownPos{X: d.X, Y: d.Y}
	}

	// 發送同伴 + 附近其他人的同伴（同伴已傳送到新位置，GetNearby* 會包含它們）
	nearbySum := deps.World.GetNearbySummonsInShow(x, y, mapID, player.ShowID)
	for _, sum := range nearbySum {
		isOwner := sum.OwnerCharID == player.CharID
		masterName := ""
		if m := deps.World.GetByCharID(sum.OwnerCharID); m != nil {
			masterName = m.Name
		}
		SendSummonPack(sess, sum, isOwner, masterName)
		player.Known.Summons[sum.ID] = world.KnownPos{X: sum.X, Y: sum.Y}
		// 也發送給新位置附近的其他玩家（讓他們看到傳送過來的召喚獸）
		if isOwner {
			for _, other := range newNearby {
				SendSummonPack(other.Session, sum, false, player.Name)
			}
		}
	}
	nearbyDolls := deps.World.GetNearbyDollsInShow(x, y, mapID, player.ShowID)
	for _, doll := range nearbyDolls {
		masterName := ""
		if m := deps.World.GetByCharID(doll.OwnerCharID); m != nil {
			masterName = m.Name
		}
		SendDollPack(sess, doll, masterName)
		player.Known.Dolls[doll.ID] = world.KnownPos{X: doll.X, Y: doll.Y}
		if doll.OwnerCharID == player.CharID {
			for _, other := range newNearby {
				SendDollPack(other.Session, doll, player.Name)
			}
		}
	}
	nearbyHierarchs := deps.World.GetNearbyHierarchsInShow(x, y, mapID, player.ShowID)
	for _, h := range nearbyHierarchs {
		masterName := ""
		if m := deps.World.GetByCharID(h.OwnerCharID); m != nil {
			masterName = m.Name
		}
		SendHierarchPack(sess, h, masterName)
		player.Known.Hierarchs[h.ID] = world.KnownPos{X: h.X, Y: h.Y}
		if h.OwnerCharID == player.CharID {
			for _, other := range newNearby {
				SendHierarchPack(other.Session, h, player.Name)
			}
		}
	}
	nearbyFollowers := deps.World.GetNearbyFollowersInShow(x, y, mapID, player.ShowID)
	for _, f := range nearbyFollowers {
		SendFollowerPack(sess, f)
		player.Known.Followers[f.ID] = world.KnownPos{X: f.X, Y: f.Y}
		if f.OwnerCharID == player.CharID {
			for _, other := range newNearby {
				SendFollowerPack(other.Session, f)
			}
		}
	}
	nearbyPets := deps.World.GetNearbyPetsInShow(x, y, mapID, player.ShowID)
	for _, pet := range nearbyPets {
		isOwner := pet.OwnerCharID == player.CharID
		masterName := ""
		if m := deps.World.GetByCharID(pet.OwnerCharID); m != nil {
			masterName = m.Name
		}
		SendPetPack(sess, pet, isOwner, masterName)
		player.Known.Pets[pet.ID] = world.KnownPos{X: pet.X, Y: pet.Y}
		if isOwner {
			for _, other := range newNearby {
				SendPetPack(other.Session, pet, false, player.Name)
			}
		}
	}

	// 限時地圖偵測（Java: Teleportation.teleportation() 中的 isTimingMap 檢查）
	OnEnterTimedMap(sess, player, mapID, deps)

	// Release client teleport lock (Java: S_Paralysis always sent in finally block).
	sendTeleportUnlock(sess)
}

// handleYesNoResponse processes S_Message_YN dialog responses.
// Routes to trade or party accept/decline based on PendingYesNoType.
func handleYesNoResponse(sess *net.Session, player *world.PlayerInfo, accepted bool, deps *Deps) {
	msgType := player.PendingYesNoType
	data := player.PendingYesNoData
	player.PendingYesNoType = 0
	player.PendingYesNoData = 0

	switch msgType {
	case 252: // Trade confirmation
		handleTradeYesNo(sess, player, data, accepted, deps)
	case 729: // Call Clan（呼喚盟友）— 玩家接受盟主呼喚後傳送到盟主位置
		handleCallClanYesNo(sess, player, data, accepted, deps)
	case 748: // Mass Teleport（集體傳送術）— 接受後傳送到施法時暫存座標
		if accepted {
			TeleportPlayer(sess, player, player.TeleportX, player.TeleportY, player.TeleportMapID, player.TeleportHeading, deps)
		}
	}
}

func handleCallClanYesNo(sess *net.Session, player *world.PlayerInfo, callerID int32, accepted bool, deps *Deps) {
	if !accepted || player.Paralyzed || player.Sleeped || deps.World == nil {
		return
	}
	caller := deps.World.GetByCharID(callerID)
	if caller != nil && caller.ClanID == player.ClanID {
		// Java `C_Attr.callClan` 第 1226 行：傳送至 `leader.X + (rand%5 - rand%5), leader.Y + (rand%5 - rand%5)`
		// （`(int)(Math.random()*5)` ＝ 0..4），最終分佈 [-4..+4] 防止盟員疊在盟主同格上。
		dx := int32(world.RandInt(5) - world.RandInt(5))
		dy := int32(world.RandInt(5) - world.RandInt(5))
		TeleportPlayer(sess, player, caller.X+dx, caller.Y+dy, caller.MapID, 5, deps)
	}
}

func handleAllianceCallClanYesNo(sess *net.Session, player *world.PlayerInfo, callerID int32, accepted bool, deps *Deps) {
	if !accepted || player.Paralyzed || player.Sleeped || deps.World == nil {
		return
	}
	caller := deps.World.GetByCharID(callerID)
	if caller != nil && sameAlliance(player, caller, deps) {
		TeleportPlayer(sess, player, caller.X, caller.Y, caller.MapID, 5, deps)
	}
}

func sameAlliance(player, caller *world.PlayerInfo, deps *Deps) bool {
	if player.ClanID == 0 || caller.ClanID == 0 || deps.Alliances == nil {
		return false
	}
	alliance := deps.Alliances.GetAllianceByClan(player.ClanID)
	return alliance != nil && alliance.Contains(caller.ClanID)
}

// ========================================================================
//  NPC Service Handlers
// ========================================================================

// handleNpcHaste — Haste buffer NPC. Parameters from npc_services.yaml.
func handleNpcHaste(sess *net.Session, player *world.PlayerInfo, npc *world.NpcInfo, deps *Deps) {
	h := deps.NpcServices.Haste()
	if npc.NpcID != h.NpcID {
		return
	}
	applyHaste(sess, player, h.DurationSec, h.Gfx, deps)
	sendServerMessage(sess, h.MsgID)
}

// handleNpcActionZero — routes the "0" action based on NPC ID.
// Healer and cancellation NPC parameters from npc_services.yaml.
func handleNpcActionZero(sess *net.Session, player *world.PlayerInfo, npc *world.NpcInfo, objID int32, deps *Deps) {
	// Check if this NPC is a cancellation NPC
	cancel := deps.NpcServices.Cancel()
	if npc.NpcID == cancel.NpcID {
		if player.Level <= cancel.MaxLevel {
			cancelAllBuffs(player, deps)
			broadcastEffect(sess, player, cancel.Gfx, deps)
		}
		return
	}

	// Check if this NPC is a healer
	if healer := deps.NpcServices.GetHealer(npc.NpcID); healer != nil {
		deps.NpcSvc.NpcFullHeal(sess, player, npc.NpcID)
		return
	}

	// Unknown "0" action for this NPC — try showing dialog
	npcAction := deps.NpcActions.Get(npc.NpcID)
	if npcAction != nil && npcAction.NormalAction != "" {
		sendHypertext(sess, objID, npcAction.NormalAction)
	}
}

// handleNpcFullHeal — 委派給 NpcServiceSystem 處理 NPC 完整治療。
func handleNpcFullHeal(sess *net.Session, player *world.PlayerInfo, npc *world.NpcInfo, deps *Deps) {
	if deps.NpcSvc != nil {
		deps.NpcSvc.NpcFullHeal(sess, player, npc.NpcID)
	}
}

// handleNpcWeaponEnchant — 委派給 NpcServiceSystem 處理 NPC 武器附魔。
func handleNpcWeaponEnchant(sess *net.Session, player *world.PlayerInfo, deps *Deps) {
	if deps.NpcSvc != nil {
		deps.NpcSvc.NpcWeaponEnchant(sess, player)
	}
}

// handleNpcArmorEnchant — 委派給 NpcServiceSystem 處理 NPC 防具附魔。
func handleNpcArmorEnchant(sess *net.Session, player *world.PlayerInfo, deps *Deps) {
	if deps.NpcSvc != nil {
		deps.NpcSvc.NpcArmorEnchant(sess, player)
	}
}

// handleNpcPoly — 委派給 NpcServiceSystem 處理 NPC 變身。
func handleNpcPoly(sess *net.Session, player *world.PlayerInfo, polyID int32, deps *Deps) {
	if deps.NpcSvc != nil {
		deps.NpcSvc.NpcPoly(sess, player, polyID)
	}
}

// sendAdenaUpdate 發送金幣數量更新封包。
func sendAdenaUpdate(sess *net.Session, player *world.PlayerInfo) {
	adena := player.Inv.FindByItemID(world.AdenaItemID)
	if adena != nil {
		sendItemCountUpdate(sess, adena)
	}
	sendWeightUpdate(sess, player)
}

// SendAdenaUpdate 匯出 sendAdenaUpdate — 供 system 套件更新金幣顯示。
func SendAdenaUpdate(sess *net.Session, player *world.PlayerInfo) {
	sendAdenaUpdate(sess, player)
}

// ========================================================================
//  Crafting System (NPC Item Making)
// ========================================================================

// sendInputAmount sends S_OPCODE_INPUTAMOUNT (136) — S_HowManyMake crafting batch dialog.
// Java: S_HowManyMake(npcObjectId, maxAmount, actionName)
// The client concatenates the two writeS strings with a space separator when sending back C_Amount.
func sendInputAmount(sess *net.Session, npcObjID int32, maxSets int32, action string) {
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_INPUTAMOUNT)
	w.WriteD(npcObjID)
	w.WriteD(0)       // unknown
	w.WriteD(0)       // spinner initial value
	w.WriteD(0)       // spinner minimum
	w.WriteD(maxSets) // spinner maximum
	w.WriteH(0)       // unknown

	// Split action: "request adena2" → prefix="request", suffix="adena2"
	// Client concatenates: "request" + " " + "adena2" = "request adena2" (matches YAML key)
	suffix := action
	if strings.HasPrefix(action, "request ") {
		suffix = action[len("request "):]
	}
	w.WriteS("request")
	w.WriteS(suffix)

	sess.Send(w.Bytes())
}

// SendInputAmount 匯出 sendInputAmount — 供 system/craft.go 發送批量製作對話框。
func SendInputAmount(sess *net.Session, npcObjID int32, maxSets int32, action string) {
	sendInputAmount(sess, npcObjID, maxSets, action)
}

// HandleCraftAmount processes C_Amount (opcode 11) when a crafting batch response is pending.
// Called from HandleHypertextInputResult when player.PendingCraftAction is set.
// Java: C_Amount.java — [D npcObjID][D amount][C unknown][S actionStr]
func HandleCraftAmount(sess *net.Session, r *packet.Reader, player *world.PlayerInfo, deps *Deps) {
	action := player.PendingCraftAction
	player.PendingCraftAction = "" // clear pending state

	npcObjID := r.ReadD()
	amount := r.ReadD()
	_ = r.ReadC() // unknown delimiter
	actionStr := r.ReadS()

	if amount <= 0 {
		return
	}

	npc := deps.World.GetNpc(npcObjID)
	if npc == nil {
		return
	}

	// Distance check
	dx := int32(math.Abs(float64(player.X - npc.X)))
	dy := int32(math.Abs(float64(player.Y - npc.Y)))
	if dx > 5 || dy > 5 {
		return
	}

	// Look up recipe — prefer the action string from client, fallback to stored action
	recipe := deps.ItemMaking.Get(actionStr)
	if recipe == nil {
		recipe = deps.ItemMaking.Get(action)
	}
	if recipe == nil {
		return
	}

	if deps.Craft != nil {
		deps.Craft.ExecuteCraft(sess, player, npc, recipe, amount)
	}
}

// ========================================================================
//  火神工匠系統 — Java: L1BlendTable / L1Blend / Npc_CraftDesk
// ========================================================================

// sendCraftItemBlend 使用 ItemBlend 模板顯示指定配方的詳細資訊。
// 3.80C 客戶端的 type 48/49 拖放介面無法使用，且不支援 inline HTML（htmlID 用於讀取本地對話檔）。
// 改用客戶端已有的 ItemBlend 模板（3.80C 確認可用）呈現配方。
// 玩家點 "confirm craft" → 開啟交易視窗確認；"cancel craft" → 顯示下一個配方。
//
// Java ItemBlend 模板資料格式：
//
//	data[0] = 成品名稱
//	data[1] = 額外獎勵資訊（空字串 = 無）
//	data[2] = 等級限制（" 無限制 " 或 " XX級以上。 "）
//	data[3] = 職業限制（" 所有職業" 或具體職業名）
//	data[4] = 成功機率（" XX %" 或空字串 = 100%）
//	data[5] = 增加機率道具資訊（空字串）
//	data[6] = 替代材料資訊（空字串）
//	data[7+] = 材料條目（"材料名 (數量) 個"）
func sendCraftItemBlend(sess *net.Session, player *world.PlayerInfo, npc *world.NpcInfo, deps *Deps, index int) {
	if deps.ItemMaking == nil {
		sendGlobalChat(sess, 9, "\\f3製作系統尚未啟用。")
		return
	}
	recipes := deps.ItemMaking.GetByNpcID(npc.NpcID)
	if len(recipes) == 0 {
		sendGlobalChat(sess, 9, "\\f3此 NPC 沒有可用的配方。")
		return
	}

	// 循環索引
	if index < 0 || index >= len(recipes) {
		index = 0
	}
	recipe := recipes[index]

	// 儲存瀏覽狀態
	player.PendingCraftKey = recipe.Action
	player.PendingCraftNpcID = npc.NpcID
	player.PendingCraftIndex = index

	// 組裝 data[0]: 成品名稱
	productName := recipe.Note
	if len(recipe.Items) > 0 {
		out := recipe.Items[0]
		if info := deps.Items.Get(out.ItemID); info != nil {
			productName = info.Name
			if out.EnchantLvl > 0 {
				productName = fmt.Sprintf("+%d %s", out.EnchantLvl, productName)
			}
			if out.Amount > 1 {
				productName = fmt.Sprintf("%s (%d)", productName, out.Amount)
			}
		}
	}

	// data[1]: 額外獎勵
	bonusInfo := ""
	if recipe.BonusItemID > 0 {
		if info := deps.Items.Get(recipe.BonusItemID); info != nil {
			bonusInfo = fmt.Sprintf("製造成功時額外獲得: %s", info.Name)
			if recipe.BonusItemCount > 1 {
				bonusInfo += fmt.Sprintf(" (%d)", recipe.BonusItemCount)
			}
		}
	}

	// data[2]: 等級限制
	levelInfo := " 無限制 "
	if recipe.RequiredLevel > 0 {
		levelInfo = fmt.Sprintf(" %d級以上。 ", recipe.RequiredLevel)
	}

	// data[3]: 職業限制
	classInfo := " 所有職業"
	if recipe.RequiredClass > 0 {
		if name := classIDToName(recipe.RequiredClass); name != "" {
			classInfo = " " + name
		}
	}

	// data[4]: 成功機率（未設定或 0 視為 100%）
	rate := recipe.SuccessRate
	if rate <= 0 {
		rate = 100
	}
	rateInfo := fmt.Sprintf(" %d %%", rate)

	// data[5], data[6]: 空字串（增加機率道具、替代材料）
	// data[7+]: 材料條目
	matCount := len(recipe.Materials)
	msgs := make([]string, 7+matCount)
	msgs[0] = productName
	msgs[1] = bonusInfo
	msgs[2] = levelInfo
	msgs[3] = classInfo
	msgs[4] = rateInfo
	msgs[5] = ""
	msgs[6] = fmt.Sprintf("(%d/%d)", index+1, len(recipes))

	for i, mat := range recipe.Materials {
		matName := fmt.Sprintf("item#%d", mat.ItemID)
		if info := deps.Items.Get(mat.ItemID); info != nil {
			matName = info.Name
		}
		if mat.EnchantLvl > 0 {
			matName = fmt.Sprintf("+%d %s", mat.EnchantLvl, matName)
		}
		msgs[7+i] = fmt.Sprintf("%s (%d) 個", matName, mat.Amount)
	}

	sendHypertextWithData(sess, npc.ID, "ItemBlend", msgs)
}

// handleRequestCraft 處理 "request craft" — 顯示配方清單。
// 815 版：smithitem 有 "request craft" 按鈕 → 送 smithitem1（41 個配方名稱）。
// 3.80C：smithitem 直接顯示配方清單（desc-c.tbl），不一定有 "request craft" 按鈕。
// 保留此函式作為相容性備用。
func handleRequestCraft(sess *net.Session, player *world.PlayerInfo, npc *world.NpcInfo, deps *Deps) {
	if deps.ItemMaking == nil {
		return
	}
	recipes := deps.ItemMaking.GetByNpcID(npc.NpcID)
	if len(recipes) == 0 {
		return
	}

	const smithitem1Slots = 41 // Java: msg0~msg40，固定 41 格
	msgs := make([]string, smithitem1Slots)
	for i := 0; i < smithitem1Slots && i < len(recipes); i++ {
		msgs[i] = recipes[i].Note
	}

	sendHypertextWithData(sess, npc.ID, "smithitem1", msgs)
}

// handleCraftSelect 處理配方選擇 — 開啟交易視窗顯示成品與材料。
// 交易視窗佈局：
//   - 上方（panelType=0, 玩家側）：成品預覽
//   - 下方（panelType=1, 對方側）：需要的材料
//
// 3.80C 客戶端在同一 tick 收到 S_Trade + S_TradeAddItem 時，交易視窗尚未初始化完成，
// 導致物品不顯示。因此 S_Trade 立即發送，S_TradeAddItem 延遲 1 tick 由 CraftTradeSystem 發送。
// 玩家按確認（C_ACCEPT_XCHG）→ handleCraftTradeConfirm 執行製作。
func handleCraftSelect(sess *net.Session, player *world.PlayerInfo, npc *world.NpcInfo, recipe *data.CraftRecipe, deps *Deps) {
	// 儲存選中的配方，交易確認時使用
	player.PendingCraftKey = recipe.Action
	player.PendingCraftNpcID = npc.NpcID

	// 開啟交易視窗 — 對方名稱（下方標題）顯示「需要的材料」
	sendTradeOpen(sess, "需要的材料")

	// 延遲 1 tick 發送物品（等待客戶端初始化交易視窗）
	player.CraftTradeTick = 1
}

// SendCraftTradeItems 延遲發送製作交易視窗的物品（由 CraftTradeSystem 呼叫）。
// 根據 PendingCraftKey/NpcID 查找配方，發送成品與材料。
// 物品不存在於 YAML 時使用 fallback 值（名稱顯示 "item#ID"，GfxID 使用 InvGfx 或預設 24）。
func SendCraftTradeItems(sess *net.Session, player *world.PlayerInfo, deps *Deps) {
	if player.PendingCraftKey == "" || deps.ItemMaking == nil {
		return
	}

	recipe := deps.ItemMaking.GetByNpcAction(player.PendingCraftNpcID, player.PendingCraftKey)
	if recipe == nil {
		return
	}

	// 上方（panelType=0, 玩家側）：成品預覽
	// Java S_TradeAddItem 使用 item.getItem().getGfxId() = 地面圖示（GrdGfx）
	for _, out := range recipe.Items {
		gfx, viewName, bless := craftTradeItemInfo(out.ItemID, out.Amount, out.EnchantLvl, deps)
		sendTradeAddItem(sess, gfx, viewName, bless, 0)
	}

	// 下方（panelType=1, 對方側）：需要的材料
	for _, mat := range recipe.Materials {
		gfx, viewName, bless := craftTradeItemInfo(mat.ItemID, mat.Amount, mat.EnchantLvl, deps)
		sendTradeAddItem(sess, gfx, viewName, bless, 1)
	}
}

// craftTradeItemInfo 取得物品的交易視窗顯示資訊。
// 物品存在於 YAML → 使用真實 GrdGfx、名稱、bless。
// 物品不存在 → 使用 fallback GfxID 24（常見物品圖示）、"item#ID" 名稱、bless=0。
func craftTradeItemInfo(itemID, amount, enchantLvl int32, deps *Deps) (gfx uint16, viewName string, bless byte) {
	info := deps.Items.Get(itemID)
	if info != nil {
		gfx = uint16(info.InvGfx)
		viewName = info.Name
		bless = byte(info.Bless)
	} else {
		// 物品未定義時的 fallback：GfxID 24（寶石圖示），名稱用 item#ID
		gfx = 24
		viewName = fmt.Sprintf("item#%d", itemID)
		bless = 0
	}
	if enchantLvl > 0 {
		viewName = fmt.Sprintf("+%d %s", enchantLvl, viewName)
	}
	if amount > 1 {
		viewName = fmt.Sprintf("%s (%d)", viewName, amount)
	}
	return
}

// craftItemDisplayName 組裝成品顯示名稱（模擬 Java getLogName()）
func craftItemDisplayName(itemID, amount, enchantLvl int32, deps *Deps) string {
	name := fmt.Sprintf("item#%d", itemID)
	if info := deps.Items.Get(itemID); info != nil {
		name = info.Name
	}
	if enchantLvl > 0 {
		name = fmt.Sprintf("+%d %s", enchantLvl, name)
	}
	if amount > 1 {
		name += fmt.Sprintf(" (%d)", amount)
	}
	return name
}

// handleConfirmCraft 處理 "confirm craft" — 開啟交易視窗預覽成品與材料。
// 直接送 S_Trade 開啟視窗，延遲 1 tick 發 S_TradeAddItem。
func handleConfirmCraft(sess *net.Session, player *world.PlayerInfo, npc *world.NpcInfo, deps *Deps) {
	if player.PendingCraftKey == "" || deps.ItemMaking == nil || deps.Craft == nil {
		return
	}

	// 驗證 NPC 一致性
	if player.PendingCraftNpcID != npc.NpcID {
		player.PendingCraftKey = ""
		player.PendingCraftNpcID = 0
		player.PendingCraftIndex = 0
		player.CraftTradeTick = 0
		return
	}

	recipe := deps.ItemMaking.GetByNpcAction(player.PendingCraftNpcID, player.PendingCraftKey)
	if recipe == nil {
		player.PendingCraftKey = ""
		player.PendingCraftNpcID = 0
		player.PendingCraftIndex = 0
		player.CraftTradeTick = 0
		return
	}

	// 開啟交易視窗 + 延遲 1 tick 發送物品
	handleCraftSelect(sess, player, npc, recipe, deps)
}

// classIDToName 將職業 ID 轉換為顯示名稱。
func classIDToName(classID int32) string {
	switch classID {
	case 1:
		return "王族"
	case 2:
		return "騎士"
	case 3:
		return "法師"
	case 4:
		return "妖精"
	case 5:
		return "黑暗妖精"
	case 6:
		return "龍騎士"
	case 7:
		return "幻術師"
	case 8:
		return "戰士"
	default:
		return ""
	}
}

// sendRefineUI 發送火神精煉/合成原生拖放 UI 封包。
// Java 380 參考：S_EquipmentWindow(int type, int value) — 當 type==48||49 時：
//
//	writeC(S_OPCODE_CHARRESET)  // opcode 64（在 Go 命名為 S_OPCODE_CHARSYNACK）
//	writeC(type)                // 48=精煉, 49=合成
//	writeD(value)               // NPC object ID
//	writeC(149)                 // 0x95
//	writeC(25)                  // 0x19
//
// 客戶端拖入裝備後回傳 C_PledgeContent (opcode 78) data=13/14 → handleRefineResolve/Transform。
func sendRefineUI(sess *net.Session, npcObjID int32, refineType byte) {
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_CHARSYNACK)
	w.WriteC(refineType) // 48=精煉, 49=合成
	w.WriteD(npcObjID)   // NPC object ID
	w.WriteC(0x95)       // 380 尾碼 byte1（149）
	w.WriteC(0x19)       // 380 尾碼 byte2（25）
	sess.Send(w.Bytes())
}

// sendFireSmithSellList 發送火神精煉介面（分解物品換結晶）。
// Java: S_ShopBuyListFireSmith — 使用 S_OPCODE_SHOP_SELL_LIST（opcode 65）。
// 格式與商店賣出列表完全相同，「價格」欄位填入結晶數量。
// 客戶端回傳 C_Result type=1（賣出），伺服器攔截後給予結晶而非金幣。
func sendFireSmithSellList(sess *net.Session, player *world.PlayerInfo, npc *world.NpcInfo, deps *Deps) {
	if deps.FireCrystals == nil {
		sendGlobalChat(sess, 9, "\\f3火神精煉系統尚未啟用。")
		return
	}

	// 排除的物品 ID（Java: S_ShopBuyListFireSmith.assessItems）
	excludeItems := map[int32]bool{
		40308: true, // 金幣
		41246: true, // 魔法結晶體
		44070: true, // 天寶
		40314: true, // 項圈
		40316: true, // 高等寵物項圈
		83000: true, // 貝利
		83022: true, // 黃金貝利
		80033: true, // 推廣銀幣
	}

	type assessedItem struct {
		objectID     int32
		crystalCount int32
	}
	var items []assessedItem

	for _, invItem := range player.Inv.Items {
		// 跳過排除物品
		if excludeItems[invItem.ItemID] {
			continue
		}
		// 跳過已裝備物品
		if invItem.Equipped {
			continue
		}

		itemInfo := deps.Items.Get(invItem.ItemID)
		if itemInfo == nil {
			continue
		}
		// 只處理武器和防具（Java: type2 != 0）
		if itemInfo.Category == data.CategoryEtcItem {
			continue
		}

		// 計算基礎 item ID（去除祝福/詛咒偏移）
		// Java: bless==0 → itemId-100000; bless==2 → itemId-200000
		lookupID := invItem.ItemID
		if invItem.Bless == 0 { // 祝福狀態
			candidateID := invItem.ItemID - 100000
			if candidateInfo := deps.Items.Get(candidateID); candidateInfo != nil {
				if candidateInfo.Name == itemInfo.Name {
					lookupID = candidateID
				}
			}
		} else if invItem.Bless == 2 { // 詛咒狀態
			candidateID := invItem.ItemID - 200000
			if candidateInfo := deps.Items.Get(candidateID); candidateInfo != nil {
				if candidateInfo.Name == itemInfo.Name {
					lookupID = candidateID
				}
			}
		}

		entry := deps.FireCrystals.Get(lookupID)
		if entry == nil {
			continue
		}

		crystalCount := entry.GetCrystalCount(int(invItem.EnchantLvl), int(itemInfo.Category), itemInfo.SafeEnchant)
		if crystalCount > 0 {
			items = append(items, assessedItem{objectID: invItem.ObjectID, crystalCount: crystalCount})
		}
	}

	if len(items) == 0 {
		// 無可精煉物品（Java: S_NPCTalkReturn "smithitem3"）
		sendHypertext(sess, npc.ID, "smithitem3")
		return
	}

	// 標記玩家正在使用火神精煉（用於 C_Result 攔截）
	player.FireSmithNpcObjID = npc.ID

	w := packet.NewWriterWithOpcode(packet.S_OPCODE_SHOP_SELL_LIST)
	w.WriteD(npc.ID)
	w.WriteH(uint16(len(items)))
	for _, it := range items {
		w.WriteD(it.objectID)     // 物品 object ID
		w.WriteD(it.crystalCount) // 結晶數量（顯示為「價格」）
	}
	w.WriteH(0x0007) // 幣種: 7=金幣（客戶端顯示用）
	sess.Send(w.Bytes())
}

// SendCloseList 關閉 NPC 對話視窗。
// Java: S_CloseList → opcode 39 + writeD(objID) + writeS("")
func SendCloseList(sess *net.Session, objID int32) {
	sendHypertext(sess, objID, "")
}

// ========================================================================
//  Summon control — Java: L1ActionSummon.action()
// ========================================================================

// handleSummonAction processes summon control commands from the moncom dialog.
// Action strings: "aggressive", "defensive", "stay", "extend", "alert", "dismiss".
func handleSummonAction(sess *net.Session, player *world.PlayerInfo, sum *world.SummonInfo, action string, deps *Deps) {
	switch action {
	case "aggressive":
		sum.Status = world.SummonAggressive
	case "defensive":
		sum.Status = world.SummonDefensive
		sum.AggroTarget = 0
		sum.AggroPlayerID = 0
	case "stay":
		sum.Status = world.SummonRest
		sum.AggroTarget = 0
		sum.AggroPlayerID = 0
	case "extend":
		sum.Status = world.SummonExtend
		sum.AggroTarget = 0
		sum.AggroPlayerID = 0
	case "alert":
		sum.Status = world.SummonAlert
		sum.HomeX = sum.X
		sum.HomeY = sum.Y
		sum.AggroTarget = 0
		sum.AggroPlayerID = 0
	case "dismiss":
		DismissSummon(sum, player, deps)
		return
	}
	// Refresh menu with updated status
	sendSummonMenu(sess, sum)
}

// isNumericString returns true if s is a non-empty string of ASCII digits.
// Java: cmd.matches("[0-9]+") — used to detect summon selection responses.
func isNumericString(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ---------- 欄位開放專家 史奈普（NPC 81445）----------
// Java: C_NPCAction.java — npcId == 81445
// 動作 A = Lv76 戒指欄位（任務 79）
// 動作 B = Lv81 戒指欄位（任務 80）
// 動作 C = Lv85 護符欄位（任務 82，自訂功能）
func handleSlotNpc(sess *net.Session, player *world.PlayerInfo, npcObjID int32, action string, deps *Deps) {
	switch action {
	case "a": // Lv76 戒指欄（第3個戒指欄位）
		if player.IsQuestDone(79) {
			SendServerMessage(sess, 3254) // 已經開通
			return
		}
		player.PendingYesNoType = 3312
		player.PendingYesNoData = 76
		sendYesNoDialog(sess, 3312)

	case "b": // Lv81 戒指欄（第4個戒指欄位）
		if player.IsQuestDone(80) {
			SendServerMessage(sess, 3254) // 已經開通
			return
		}
		player.PendingYesNoType = 3313
		player.PendingYesNoData = 81
		sendYesNoDialog(sess, 3313)

	case "c": // Lv85 護符欄（自訂擴充欄位）
		if player.IsQuestDone(82) {
			SendServerMessage(sess, 3254) // 已經開通
			return
		}
		player.PendingYesNoType = 3590
		player.PendingYesNoData = 85
		sendYesNoDialog(sess, 3590)
	}
}

// ---------- 物品升級合成系統 ----------

// handleItemUpgrade 委派給 NpcServiceSystem 處理物品升級合成。
func handleItemUpgrade(sess *net.Session, player *world.PlayerInfo, upg *data.ItemUpgrade, deps *Deps) {
	if deps.NpcSvc != nil {
		deps.NpcSvc.NpcUpgrade(sess, player, upg)
	}
}

// handlePetMatchEntry 處理寵物比賽報名（NPC 80088）。
// Java: Npc_PetWar.action() — cmd = "ent,,<amuletObjID>"。
func handlePetMatchEntry(sess *net.Session, player *world.PlayerInfo, action string, deps *Deps) {
	if deps.PetMatch == nil {
		return
	}

	// 解析動作字串："ent,,<amuletObjID>"
	parts := strings.Split(action, ",")
	if len(parts) < 3 {
		return
	}
	amuletObjID := parseInt32(parts[2])
	if amuletObjID <= 0 {
		return
	}

	// 驗證物品存在於玩家背包
	collarItem := player.Inv.FindByObjectID(amuletObjID)
	if collarItem == nil {
		return
	}

	if !deps.PetMatch.EnterPetMatch(sess, player, amuletObjID) {
		// EnterPetMatch 內部已發送錯誤訊息
	}
}

// --- 投石車 NPC 動作 ---

// catapultHTMLMap 投石車 NPC ID → 對話 HTML ID（Java: ckenta/ckentd/cgirana/cgirand/corca/corcd）。
var catapultHTMLMap = map[int32]string{
	90327: "ckenta", 90328: "ckenta",
	90329: "ckentd", 90330: "ckentd",
	90331: "cgirana", 90332: "cgirana",
	90333: "cgirand", 90334: "cgirand",
	90335: "corca", 90336: "corca",
	90337: "corcd",
}

// handleCatapultAction 投石車 NPC 動作分派。
// Java: C_NPCAction.java:3625-3727。
func handleCatapultAction(sess *net.Session, player *world.PlayerInfo, npc *world.NpcInfo, action string, deps *Deps) {
	// 檢查攻城戰是否進行中
	castleID := deps.Castle.GetCastleIDByNpcLocation(npc.X, npc.Y, npc.MapID)
	if castleID == 0 || !deps.Castle.IsWarNow(castleID) {
		SendServerMessage(sess, 3683) // 攻城戰未進行
		return
	}

	// 必須是君主（ClanRank: 0=盟主, 7=聯盟盟主）
	if player.ClanRank != 0 && player.ClanRank != 7 {
		SendServerMessage(sess, 2498) // 只有血盟君主才可
		return
	}

	// 檢查操作權限：攻擊方投石車只有宣戰方可用，防守方只有守城方可用
	ci := deps.Castle.GetCastle(castleID)
	if ci == nil {
		return
	}
	isAttacker := deps.Castle.IsCatapultAttacker(npc.NpcID)
	if isAttacker {
		// 攻擊方投石車 → 玩家必須是宣戰方（非城盟）
		if ci.OwnerClanID != 0 && player.ClanID == ci.OwnerClanID {
			SendServerMessage(sess, 3681) // 守城方不可用攻擊投石車
			return
		}
	} else {
		// 防守方投石車 → 玩家必須是城盟
		if ci.OwnerClanID == 0 || player.ClanID != ci.OwnerClanID {
			SendServerMessage(sess, 3682) // 攻擊方不可用防守投石車
			return
		}
	}

	// 初次點擊（action 為空或 "0"）→ 開啟對話 HTML
	if action == "" || action == "0" {
		htmlID, ok := catapultHTMLMap[npc.NpcID]
		if !ok {
			return
		}
		sendHypertext(sess, npc.ID, htmlID)
		return
	}

	// 砲彈發射指令（"0-N" 或 "1-N"）
	deps.Castle.HandleCatapultAction(sess, player, npc, action)
}

// --- 攻城戰時間查詢 ---

// guardCastleMap 近衛兵 NPC ID → 城堡 ID + HTML ID 映射。
// Java: C_NPCAction.java:742-772。
var guardCastleMap = map[int32]struct {
	castleID int32
	htmlID   string
}{
	60514: {1, "ktguard7"},  // 肯特
	60560: {2, "orcguard7"}, // 妖魔
	60552: {3, "wdguard7"},  // 風木
	60524: {4, "grguard7"},  // 奇巖
	60525: {4, "grguard7"},  // 奇巖
	60529: {4, "grguard7"},  // 奇巖
	70857: {5, "heguard7"},  // 海音
	60530: {6, "dcguard7"},  // 侏儒
	60531: {6, "dcguard7"},  // 侏儒
	60533: {7, "adguard7"},  // 亞丁
	60534: {7, "adguard7"},  // 亞丁
	81156: {8, "dfguard3"},  // 狄亞得
}

// handleAskWarTime 處理 askwartime NPC 動作（查詢攻城戰時間）。
// Java: C_NPCAction.java:742 — 依 NPC ID 查城堡 → makeWarTimeStrings → 發送 HTML。
func handleAskWarTime(sess *net.Session, npc *world.NpcInfo, deps *Deps) {
	entry, ok := guardCastleMap[npc.NpcID]
	if !ok {
		return
	}

	if deps.Castle == nil {
		return
	}
	ci := deps.Castle.GetCastle(entry.castleID)
	if ci == nil {
		return
	}

	// Java makeWarTimeStrings: 格式化攻城時間為字串陣列
	warTime := ci.WarTime
	year := strconv.Itoa(warTime.Year())
	month := strconv.Itoa(int(warTime.Month()))
	day := strconv.Itoa(warTime.Day())
	hour := strconv.Itoa(warTime.Hour())
	minute := strconv.Itoa(warTime.Minute())

	var htmldata []string
	if entry.castleID == 2 {
		// 妖魔城：5 個元素（無空前綴）
		htmldata = []string{year, month, day, hour, minute}
	} else {
		// 其他城堡：6 個元素（首元素為空）
		htmldata = []string{"", year, month, day, hour, minute}
	}

	sendHypertextWithData(sess, npc.ID, entry.htmlID, htmldata)
}
