package handler

import (
	"math/rand"
	"time"

	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

// --- 料理系統 ---
// Java: C_ItemUSe.makeCooking(), L1Cooking.java, S_PacketBox.java
// 料理書物品 ID: 41255-41259（5 級）
// S_PacketBoxCooking: opcode 250 (S_OPCODE_EVENT), type 52=選單, type 53=圖示

// 料理相關常數
const (
	cookingBookMinID int32 = 41255
	cookingBookMaxID int32 = 41259
	cookingNowSkill  int32 = 2999 // 製作中狀態 skill ID
	campfireItemID   int32 = 41260
	campfireNpcID    int32 = 81170
	campfireGfxID    int32 = 5943
)

// CookingRecipe 料理配方。
type CookingRecipe struct {
	Materials []int32 // 每種材料各一個
	ResultID  int32   // 成功結果物品 ID
	SpecialID int32   // 特別版結果物品 ID（5% 機率）
}

// C_ItemUSe.makeCooking(): 24 道 Lv1–3 配方；料理書選單另支援 index 0–4。
// Java 不以書本等級再次限制 cookNo，採客戶端傳入的全域配方編號。
var cookingRecipes = []CookingRecipe{
	{[]int32{40057}, 41277, 41285},
	{[]int32{41275}, 41278, 41286},
	{[]int32{41263, 41265}, 41279, 41287},
	{[]int32{41274, 41267}, 41280, 41288},
	{[]int32{40062, 40069, 40064}, 41281, 41289},
	{[]int32{40056, 40060, 40061}, 41282, 41290},
	{[]int32{41276}, 41283, 41291},
	{[]int32{40499, 40060}, 41284, 41292},
	{[]int32{49040, 49048}, 49049, 49057},
	{[]int32{49041, 49048}, 49050, 49058},
	{[]int32{49042, 41265, 49048}, 49051, 49059},
	{[]int32{49043, 49048}, 49052, 49060},
	{[]int32{49044, 49048}, 49053, 49061},
	{[]int32{49045, 49048}, 49054, 49062},
	{[]int32{49046, 49048}, 49055, 49063},
	{[]int32{49047, 40499, 49048}, 49056, 49064},
	{[]int32{49048, 49243, 49260}, 49244, 49252},
	{[]int32{49048, 49243, 49261}, 49245, 49253},
	{[]int32{49048, 49243, 49262}, 49246, 49254},
	{[]int32{49048, 49243, 49263}, 49247, 49255},
	{[]int32{49048, 49243, 49264}, 49248, 49256},
	{[]int32{49048, 49243, 49265}, 49249, 49257},
	{[]int32{49048, 49243, 49266}, 49250, 49258},
	{[]int32{49048, 49243, 49267, 40499}, 49251, 49259},
}

// IsCookingBook 檢查物品是否為料理書。
func IsCookingBook(itemID int32) bool {
	return itemID >= cookingBookMinID && itemID <= cookingBookMaxID
}

// HandleCookingBook 處理使用料理書。
func HandleCookingBook(sess *net.Session, player *world.PlayerInfo, item *world.InvItem, deps *Deps) {
	// 計算料理等級（0-4）
	cookLevel := item.ItemID - cookingBookMinID

	// 顯示料理選單（Java: S_PacketBoxCooking type 52）
	sendCookingMenu(sess, player, int(cookLevel))
}

// handleCookingBookUse handles C_USE_ITEM's [C cookStatus][C cookNo].
func handleCookingBookUse(sess *net.Session, r *packet.Reader, player *world.PlayerInfo, item *world.InvItem, deps *Deps) {
	if r.Remaining() < 2 {
		return
	}
	status, cookNo := r.ReadC(), r.ReadC()
	if status == 0 {
		HandleCookingBook(sess, player, item, deps)
		return
	}
	HandleCookingSelect(sess, player, int(cookNo), deps)
}

// HandleCookingSelect 製作料理；食用的 buff 與圖示由原有食用路徑處理。
func HandleCookingSelect(sess *net.Session, player *world.PlayerInfo, cookNo int, deps *Deps) {
	handleCookingSelectAt(sess, player, cookNo, deps, rand.Intn(100)+1, time.Now())
}

func handleCookingSelectAt(sess *net.Session, player *world.PlayerInfo, cookNo int, deps *Deps, roll int, now time.Time) {
	if player == nil || player.Inv == nil || player.Dead || player.HP <= 0 || cookNo < 0 || cookNo >= len(cookingRecipes) || roll < 1 || roll > 100 {
		return
	}
	if deps == nil || deps.World == nil || deps.NpcSvc == nil || deps.ItemCreate == nil || deps.Items == nil {
		return
	}
	if !nearCookingFire(player, deps, false) {
		SendServerMessage(sess, 1160)
		return
	}
	if float64(player.Inv.TotalWeight()) >= world.PlayerMaxWeight(player) {
		SendServerMessage(sess, 1103)
		return
	}
	if buff := player.GetBuff(cookingNowSkill); buff != nil && (buff.ExpiresAt.IsZero() || now.Before(buff.ExpiresAt)) {
		return
	}
	player.AddBuff(&world.ActiveBuff{SkillID: cookingNowSkill, TicksLeft: 15, ExpiresAt: now.Add(3 * time.Second)})

	recipe := cookingRecipes[cookNo]
	materials := make([]*world.InvItem, 0, len(recipe.Materials))
	for _, materialID := range recipe.Materials {
		material := player.Inv.FindByItemID(materialID)
		if material == nil || material.Count < 1 {
			SendServerMessage(sess, 1102)
			return
		}
		materials = append(materials, material)
	}
	resultID, effectID := recipe.ResultID, int32(6392)
	if roll > 90 {
		resultID, effectID = recipe.SpecialID, 6390
	}
	if deps.Items.Get(resultID) == nil {
		return
	}
	for _, material := range materials {
		if !deps.NpcSvc.ConsumeItem(sess, player, material.ObjectID, 1) {
			return
		}
	}
	if roll > 95 {
		SendServerMessage(sess, 1101)
		effectID = 6394
	} else {
		giveCookingResult(sess, player, resultID, deps)
	}
	// Java broadcasts normal/failure animations to observers, special to self too.
	if effectID == 6390 {
		sess.Send(BuildSkillEffect(player.CharID, effectID))
	}
	BroadcastToPlayers(deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, player.SessionID, player.ShowID), BuildSkillEffect(player.CharID, effectID))
}

func giveCookingResult(sess *net.Session, player *world.PlayerInfo, resultID int32, deps *Deps) {
	info := deps.Items.Get(resultID)
	if _, ok := deps.ItemCreate.GiveItem(sess, player, resultID, 1); !ok {
		// Java createNewItem drops a result that cannot fit; ingredients stay consumed.
		ground := &world.GroundItem{
			ID: world.NextGroundItemID(), ItemID: resultID, Count: 1, Name: info.Name, GrdGfx: info.GrdGfx,
			X: player.X, Y: player.Y, MapID: player.MapID, ShowID: player.ShowID, TTL: 5 * 60 * 5,
			NoExpire: deps.Houses != nil && deps.Houses.FindHouseAt(player.X, player.Y, player.MapID) != nil,
		}
		deps.World.AddGroundItem(ground)
		for _, viewer := range deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, 0, player.ShowID) {
			SendDropItem(viewer.Session, ground)
		}
	}
	sendServerMessageS(sess, 403, info.Name)
}

func nearCookingFire(player *world.PlayerInfo, deps *Deps, checkNpcID bool) bool {
	for _, effect := range deps.World.GetNearbyGroundEffectsInShow(player.X, player.Y, player.MapID, player.ShowID) {
		if effect.TicksLeft <= 0 || ChebyshevDist(player.X, player.Y, effect.X, effect.Y) > 3 {
			continue
		}
		if (!checkNpcID && effect.GfxID == campfireGfxID) || (checkNpcID && effect.NpcID == campfireNpcID) {
			return true
		}
	}
	return false
}

// handleCookingFirewood mirrors C_ItemUSe item 41260: front tile, ten minutes.
func handleCookingFirewood(sess *net.Session, player *world.PlayerInfo, item *world.InvItem, deps *Deps) {
	if deps == nil || deps.World == nil || deps.NpcSvc == nil || player == nil || player.Inv == nil || item.Count < 1 {
		return
	}
	if nearCookingFire(player, deps, true) {
		SendServerMessage(sess, 1162)
		return
	}
	if !deps.NpcSvc.ConsumeItem(sess, player, item.ObjectID, 1) {
		return
	}
	heading := int(player.Heading) & 7
	dx, dy := headingDX[heading], headingDY[heading]
	effect := &world.GroundEffect{
		ID: world.NextGroundEffectID(), NpcID: campfireNpcID, GfxID: campfireGfxID,
		Type: world.GroundEffectNpcEffect, X: player.X + dx, Y: player.Y + dy,
		MapID: player.MapID, ShowID: player.ShowID, TicksLeft: 600 * 5,
	}
	deps.World.AddGroundEffect(effect)
	for _, viewer := range deps.World.GetNearbyPlayersInShow(effect.X, effect.Y, effect.MapID, 0, effect.ShowID) {
		SendGroundEffectPack(viewer.Session, effect)
		if viewer.Known != nil {
			viewer.Known.GroundEffects[effect.ID] = world.KnownPos{X: effect.X, Y: effect.Y}
		}
	}
}

// sendCookingMenu 發送料理選單封包。
// Java: S_PacketBoxCooking type 52 — 選單 UI
func sendCookingMenu(sess *net.Session, player *world.PlayerInfo, cookLevel int) {
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_EVENT)
	w.WriteC(52) // S_PacketBox.COOK_WINDOW
	for _, value := range []byte{0xdb, 0x31, 0xdf, 0x02, 0x01} {
		w.WriteC(value)
	}
	w.WriteC(byte(cookLevel))
	sess.Send(w.Bytes())
}

// SendCookingIcon 匯出 sendCookingIcon — 供 system 套件發送料理圖示。
func SendCookingIcon(sess *net.Session, player *world.PlayerInfo, cookType int, duration int16) {
	sendCookingIcon(sess, player, cookType, duration)
}

// sendCookingIcon 發送料理效果圖示封包。
// Java: S_PacketBoxCooking type 53 — 料理圖示
// 格式: writeC(STR) + writeC(INT) + writeC(WIS) + writeC(DEX) + writeC(CON) + writeC(CHA)
//       + writeH(food) + writeC(type) + writeC(icon_id) + writeH(time) + writeC(0)
func sendCookingIcon(sess *net.Session, player *world.PlayerInfo, cookType int, duration int16) {
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_EVENT)
	w.WriteH(53) // type 53: ICON_COOKING

	// 基礎六維屬性
	w.WriteC(byte(player.Str))
	w.WriteC(byte(player.Intel))
	w.WriteC(byte(player.Wis))
	w.WriteC(byte(player.Dex))
	w.WriteC(byte(player.Con))
	w.WriteC(byte(player.Cha))

	// 飽食度計算（Java: food = pc.get_food() * 10 - 250，最小 0）
	food := int(player.Food)*10 - 250
	if food < 0 {
		food = 0
	}
	w.WriteH(uint16(food))

	w.WriteC(byte(cookType)) // 料理 type
	w.WriteC(38)             // icon_id（Java 預設 38）
	w.WriteH(uint16(duration))
	w.WriteC(0) // 尾碼

	sess.Send(w.Bytes())
}
