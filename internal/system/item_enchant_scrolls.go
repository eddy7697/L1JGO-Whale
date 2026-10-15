package system

import (
	"fmt"
	"math/rand"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

// ---------- 屬性強化／象牙塔／飾品強化卷軸 ----------
//
// Java ref: C_ItemUSe（依 itemId 讀 [D targetObjID] 後分派）、
// model/item/action/Enchant.java scrollOfEnchantWeaponAttr / scrollOfEnchantWeaponIvoryTower /
// scrollOfEnchantArmorIvoryTower / scrollOfEnchantAccessory / SuccessEnchant / FailureEnchant。

// EnchantSpecialScroll 處理 world.IsSpecialEnchantScroll 的卷軸。
// C_USE_ITEM 接續資料: [D targetObjectID]
func (s *ItemUseSystem) EnchantSpecialScroll(sess *net.Session, r *packet.Reader, player *world.PlayerInfo, scroll *world.InvItem, scrollInfo *data.ItemInfo) {
	targetObjID := r.ReadD()
	s.EnchantSpecialScrollWithRoll(sess, player, scroll, scrollInfo, targetObjID, rand.Intn(100)+1)
}

// EnchantSpecialScrollWithRoll 與 EnchantSpecialScroll 相同，但由呼叫端提供 1-100 的亂數（供測試）。
// 回傳 true 表示卷軸已被消耗。
func (s *ItemUseSystem) EnchantSpecialScrollWithRoll(sess *net.Session, player *world.PlayerInfo, scroll *world.InvItem, scrollInfo *data.ItemInfo, targetObjID int32, roll int) bool {
	if player == nil || player.Inv == nil || scroll == nil || scrollInfo == nil {
		return false
	}
	target := player.Inv.FindByObjectID(targetObjID)
	var targetInfo *data.ItemInfo
	if target != nil {
		targetInfo = s.deps.Items.Get(target.ItemID)
	}

	switch scroll.ItemID {
	case world.ScrollEnchantWeaponWind, world.ScrollEnchantWeaponEarth, world.ScrollEnchantWeaponWater, world.ScrollEnchantWeaponFire:
		return s.enchantWeaponAttr(sess, player, scroll, target, targetInfo, roll)
	case world.ScrollEnchantWeaponIvoryTower:
		return s.enchantIvoryTower(sess, player, scroll, target, targetInfo, data.CategoryWeapon)
	case world.ScrollEnchantArmorIvoryTower:
		return s.enchantIvoryTower(sess, player, scroll, target, targetInfo, data.CategoryArmor)
	case world.ScrollEnchantAccessory:
		return s.enchantAccessory(sess, player, scroll, target, targetInfo, roll)
	}
	return false
}

// enchantWeaponAttr 武器屬性強化卷軸（Java Enchant.scrollOfEnchantWeaponAttr）。
//   - 目標必須是未封印武器，否則 79；safe_enchant<0 → 1453。
//   - 同屬性且已達 +3 → 1453（不消耗）。
//   - roll(1-100) <= AttrEnchantChance → 成功：同屬性 +1，不同屬性改為該屬性 1 級；訊息 1410。
//   - 失敗訊息 1411。成功或失敗皆消耗卷軸。
func (s *ItemUseSystem) enchantWeaponAttr(sess *net.Session, player *world.PlayerInfo, scroll, target *world.InvItem, targetInfo *data.ItemInfo, roll int) bool {
	if target == nil || targetInfo == nil || targetInfo.Category != data.CategoryWeapon || target.Bless >= 128 {
		handler.SendServerMessage(sess, 79) // 沒有任何事情發生。
		return false
	}
	if targetInfo.SafeEnchant < 0 {
		handler.SendServerMessage(sess, 1453) // 此裝備無法使用強化。
		return false
	}
	newKind := world.AttrEnchantScrollKind(scroll.ItemID)
	isSameAttr := newKind != world.AttrKindNone && target.AttrEnchantKind == newKind
	if isSameAttr && target.AttrEnchantLevel >= world.AttrEnchantMaxLevel {
		handler.SendServerMessage(sess, 1453)
		return false
	}

	logName := handler.BuildViewName(target, targetInfo)
	chance := attrEnchantChancePercent(s.deps)
	if chance >= roll {
		handler.SendServerMessageArgs(sess, 1410, logName) // 對 %0 附加強大的魔法力量成功。
		newLevel := int8(1)
		if isSameAttr {
			newLevel = target.AttrEnchantLevel + 1
		}
		target.AttrEnchantKind = newKind
		target.AttrEnchantLevel = newLevel
		handler.SendItemStatusUpdate(sess, target, targetInfo)
		handler.SendItemNameUpdate(sess, target, targetInfo)
		s.deps.Log.Info(fmt.Sprintf("屬性強化成功  角色=%s  道具=%s  屬性=%d  等級=%d", player.Name, targetInfo.Name, newKind, newLevel))
	} else {
		handler.SendServerMessageArgs(sess, 1411, logName) // 對 %0 附加魔法失敗。
		s.deps.Log.Info(fmt.Sprintf("屬性強化失敗  角色=%s  道具=%s", player.Name, targetInfo.Name))
	}
	s.consumeUsedItem(sess, player, scroll)
	player.Dirty = true
	return true
}

// attrEnchantChancePercent 取得屬性強化成功率（1-100 整數，Java Config.ATTR_ENCHANT_CHANCE）。
func attrEnchantChancePercent(deps *handler.Deps) int {
	if deps == nil || deps.Config == nil {
		return 10
	}
	return int(deps.Config.Enchant.AttrChance*100 + 0.5)
}

// enchantIvoryTower 象牙塔卷軸（Java scrollOfEnchantWeaponIvoryTower / scrollOfEnchantArmorIvoryTower）。
//   - 目標類別須符合、未封印、且在象牙塔白名單，否則 79。
//   - 目前強化 < safe_enchant → 消耗卷軸並必定 +1（SuccessEnchant）；否則 79 且不消耗。
func (s *ItemUseSystem) enchantIvoryTower(sess *net.Session, player *world.PlayerInfo, scroll, target *world.InvItem, targetInfo *data.ItemInfo, category data.ItemCategory) bool {
	if target == nil || targetInfo == nil || targetInfo.Category != category || target.Bless >= 128 {
		handler.SendServerMessage(sess, 79)
		return false
	}
	allowed := false
	if category == data.CategoryWeapon {
		allowed = world.IsIvoryTowerWeapon(target.ItemID)
	} else {
		allowed = world.IsIvoryTowerArmor(target.ItemID)
	}
	if !allowed {
		handler.SendServerMessage(sess, 79)
		return false
	}
	if int(target.EnchantLvl) >= targetInfo.SafeEnchant {
		handler.SendServerMessage(sess, 79)
		return false
	}
	s.consumeUsedItem(sess, player, scroll)
	s.applySuccessEnchant(sess, player, target, targetInfo, 1)
	player.Dirty = true
	return true
}

// accessoryEnchantChance 飾品強化成功門檻（Java：(50 + lvl) / (lvl + 1) + 35，整數除法）。
// 判定為 roll(1-100) < 門檻（嚴格小於），故實際成功率 = 門檻 - 1 個百分點：+0 門檻 85 → 實際 84%，+9 門檻 40 → 實際 39%。
func accessoryEnchantChance(enchantLevel int) int {
	return (50+enchantLevel)/(enchantLevel+1) + 35
}

// isAccessoryEnchantTarget 回傳防具是否為 Java type 8-12（amulet/ring/belt/ring2/earring）。
func isAccessoryEnchantTarget(info *data.ItemInfo) bool {
	if info == nil || info.Category != data.CategoryArmor {
		return false
	}
	switch info.Type {
	case "amulet", "ring", "belt", "ring2", "earring":
		return true
	}
	return false
}

// enchantAccessory 飾品強化卷軸（Java Enchant.scrollOfEnchantAccessory）。
//   - 目標須為未封印、grade != -1 的飾品（amulet/ring/belt/earring），強化等級 0-9，否則 79（不消耗）。
//   - roll(1-100) < (50+lvl)/(lvl+1)+35 → 成功：依 grade 累加飾品加成（+5→+6 時額外獎勵），
//     SuccessEnchant +1（訊息 161）、S_ItemStatus、消耗卷軸。
//   - 失敗：FailureEnchant（訊息 164、飾品消失）、消耗卷軸。
//   - grade 3（特等）台版未實裝：Java 不加任何屬性但仍 +1；grade 其他值 → 79（卷軸不消耗）。
func (s *ItemUseSystem) enchantAccessory(sess *net.Session, player *world.PlayerInfo, scroll, target *world.InvItem, targetInfo *data.ItemInfo, roll int) bool {
	if target == nil || targetInfo == nil || target.Bless >= 128 || !isAccessoryEnchantTarget(targetInfo) || targetInfo.Grade == -1 {
		handler.SendServerMessage(sess, 79)
		return false
	}
	if target.EnchantLvl < 0 || target.EnchantLvl >= world.AccessoryEnchantMaxLevel {
		handler.SendServerMessage(sess, 79)
		return false
	}

	if roll >= accessoryEnchantChance(int(target.EnchantLvl)) { // Java: rnd < chance 才成功
		// 失敗：飾品消失
		s.applyFailureEnchant(sess, player, target, targetInfo)
		s.consumeUsedItem(sess, player, scroll)
		player.Dirty = true
		return true
	}

	award := target.EnchantLvl == 5 // +6 時額外獎勵
	acc := &target.Accessory
	switch targetInfo.Grade {
	case 0: // 上等：四屬性 +1；獎勵 HPR/MPR +1
		acc.EarthMr++
		acc.FireMr++
		acc.WaterMr++
		acc.WindMr++
		if award {
			acc.HPR++
			acc.MPR++
		}
	case 1: // 中等：HP +2；獎勵 MR +1
		acc.AddHP += 2
		if award {
			acc.MDef++
		}
	case 2: // 初等：MP +1；獎勵 SP +1
		acc.AddMP++
		if award {
			acc.AddSP++
		}
	case 3: // 特等：功能台版未實裝
	default:
		handler.SendServerMessage(sess, 79)
		return false
	}

	s.applySuccessEnchant(sess, player, target, targetInfo, 1)
	handler.SendItemStatusUpdate(sess, target, targetInfo)
	s.consumeUsedItem(sess, player, scroll)
	player.Dirty = true
	return true
}

// applySuccessEnchant 對應 Java Enchant.SuccessEnchant：訊息 161、強化 +amount、
// 更新物品名稱／狀態，裝備中則重算角色屬性（Java 直接 addAc/addMr；Go 透過 RecalcEquipStats）。
func (s *ItemUseSystem) applySuccessEnchant(sess *net.Session, player *world.PlayerInfo, target *world.InvItem, targetInfo *data.ItemInfo, amount int) {
	// 光色：武器 $245 藍、防具 $252 銀（Java sa[type2][i+1]）；結果 $247（+1）
	lightColor := "$245"
	if targetInfo.Category == data.CategoryArmor {
		lightColor = "$252"
	}
	resultDesc := "$247"
	if amount >= 2 {
		resultDesc = "$248"
	}
	logName := handler.BuildViewName(target, targetInfo)
	handler.SendServerMessageArgs(sess, 161, logName, lightColor, resultDesc)

	target.EnchantLvl += int8(amount)
	handler.SendItemStatusUpdate(sess, target, targetInfo)
	handler.SendItemNameUpdate(sess, target, targetInfo)
	sendEffectOnPlayer(sess, player.CharID, 2583) // 與一般衝裝卷軸成功路徑一致

	if target.Equipped && s.deps.Equip != nil {
		s.deps.Equip.RecalcEquipStats(sess, player)
	}
	s.deps.Log.Info(fmt.Sprintf("衝裝成功  角色=%s  道具=%s  衝裝等級=%d", player.Name, targetInfo.Name, target.EnchantLvl))
}

// applyFailureEnchant 對應 Java Enchant.FailureEnchant：訊息 164（防具 $252、武器 $245、負值 $246）並移除整個物品。
func (s *ItemUseSystem) applyFailureEnchant(sess *net.Session, player *world.PlayerInfo, target *world.InvItem, targetInfo *data.ItemInfo) {
	color := "$245"
	if targetInfo.Category == data.CategoryArmor {
		color = "$252"
	}
	if target.EnchantLvl < 0 {
		color = "$246"
	}
	handler.SendServerMessageArgs(sess, 164, handler.BuildViewName(target, targetInfo), color)

	if target.Equipped && s.deps.Equip != nil {
		if slot := s.deps.Equip.FindEquippedSlot(player, target); slot != world.SlotNone {
			s.deps.Equip.UnequipSlot(sess, player, slot)
		}
	}
	player.Inv.RemoveItem(target.ObjectID, target.Count)
	handler.SendRemoveInventoryItem(sess, target.ObjectID)
	handler.SendWeightUpdate(sess, player)
	s.deps.Log.Info(fmt.Sprintf("衝裝碎裂  角色=%s  道具=%s", player.Name, targetInfo.Name))
}
