package world

import "strconv"

// 屬性強化／象牙塔／飾品強化卷軸的物品 ID 與適用對象（Java: C_ItemUSe、Enchant.java）。
// 這些卷軸在 Java 依 itemId 分派，不走一般 dai/zel 衝裝流程。

// 武器屬性強化卷軸（Java Enchant.scrollOfEnchantWeaponAttr）
const (
	ScrollEnchantWeaponWind  int32 = 41429 // 風之武器強化卷軸 → kind 8
	ScrollEnchantWeaponEarth int32 = 41430 // 地之武器強化卷軸 → kind 1
	ScrollEnchantWeaponWater int32 = 41431 // 水之武器強化卷軸 → kind 4
	ScrollEnchantWeaponFire  int32 = 41432 // 火之武器強化卷軸 → kind 2

	ScrollEnchantArmorIvoryTower  int32 = 49311 // 象牙塔對盔甲施法的卷軸
	ScrollEnchantWeaponIvoryTower int32 = 49312 // 象牙塔對武器施法的卷軸
	ScrollEnchantAccessory        int32 = 49148 // 飾品強化卷軸

	// AttrEnchantMaxLevel 同屬性強化上限（Java: isSameAttr && oldAttrEnchantLevel >= 3 → 1453）
	AttrEnchantMaxLevel int8 = 3
	// AccessoryEnchantMaxLevel 飾品強化上限（Java: enchant_level >= 10 → 79）
	AccessoryEnchantMaxLevel int8 = 10
)

// 元素屬性種類（Java attrEnchantKind）
const (
	AttrKindNone  int8 = 0
	AttrKindEarth int8 = 1
	AttrKindFire  int8 = 2
	AttrKindWater int8 = 4
	AttrKindWind  int8 = 8
)

// AttrEnchantScrollKind 回傳屬性強化卷軸對應的元素種類；非屬性卷軸回傳 AttrKindNone。
func AttrEnchantScrollKind(scrollItemID int32) int8 {
	switch scrollItemID {
	case ScrollEnchantWeaponWind:
		return AttrKindWind
	case ScrollEnchantWeaponEarth:
		return AttrKindEarth
	case ScrollEnchantWeaponWater:
		return AttrKindWater
	case ScrollEnchantWeaponFire:
		return AttrKindFire
	}
	return AttrKindNone
}

// IsSpecialEnchantScroll 回傳此卷軸是否由 ItemUseManager.EnchantSpecialScroll 處理
// （Java C_ItemUSe 對這些 itemId 讀取 [D targetObjID] 後各自分派）。
func IsSpecialEnchantScroll(itemID int32) bool {
	switch itemID {
	case ScrollEnchantWeaponWind, ScrollEnchantWeaponEarth, ScrollEnchantWeaponWater, ScrollEnchantWeaponFire,
		ScrollEnchantArmorIvoryTower, ScrollEnchantWeaponIvoryTower, ScrollEnchantAccessory:
		return true
	}
	return false
}

// ivoryTowerWeaponIDs 象牙塔卷軸可強化的武器（Java scrollOfEnchantWeaponIvoryTower 白名單）。
var ivoryTowerWeaponIDs = map[int32]bool{
	7: true, 35: true, 48: true, 73: true, 105: true, 120: true,
	147: true, 156: true, 174: true, 175: true, 224: true,
}

// ivoryTowerArmorIDs 象牙塔卷軸可強化的防具（Java scrollOfEnchantArmorIvoryTower 白名單：象牙塔／泡水裝備）。
var ivoryTowerArmorIDs = map[int32]bool{
	20028: true, 20082: true, 20126: true, 20173: true, 20206: true, 20232: true,
	21138: true, 21051: true, 21052: true, 21053: true, 21054: true, 21055: true, 21056: true,
	21140: true, 21141: true,
}

// IsIvoryTowerWeapon 回傳武器是否在象牙塔卷軸白名單。
func IsIvoryTowerWeapon(itemID int32) bool { return ivoryTowerWeaponIDs[itemID] }

// IsIvoryTowerArmor 回傳防具是否在象牙塔卷軸白名單。
func IsIvoryTowerArmor(itemID int32) bool { return ivoryTowerArmorIDs[itemID] }

// AttrEnchantNamePrefix 回傳武器屬性強化的名稱前綴字串代碼（Java L1ItemInstance.getNumberedName）。
// 無屬性或等級超出 1-3 回傳空字串。
func AttrEnchantNamePrefix(kind, level int8) string {
	if level < 1 || level > 3 {
		return ""
	}
	base := 0
	switch kind {
	case AttrKindFire:
		base = 6115
	case AttrKindWater:
		base = 6118
	case AttrKindWind:
		base = 6121
	case AttrKindEarth:
		base = 6124
	default:
		return ""
	}
	return "$" + strconv.Itoa(base+int(level)-1)
}

// WeaponAttrEnchantDamage 計算武器屬性強化附加傷害（Java L1Attack.calcAttrEnchantDmg）。
// level 1/2/3 → 基礎 1/3/5；resist 為目標對該屬性的抗性（PC 目標取該屬性抗性，
// NPC 目標若 weak_attr 等於武器屬性則視為 -50）。
// 公式：resistFloor = int(0.32*|resist|)（保留正負號），係數 = 1 - resistFloor/32，傷害 = int(base*係數)。
func WeaponAttrEnchantDamage(level int8, resist int) int {
	base := 0
	switch level {
	case 1:
		base = 1
	case 2:
		base = 3
	case 3:
		base = 5
	default:
		return 0
	}
	abs := resist
	if abs < 0 {
		abs = -abs
	}
	resistFloor := int(0.32 * float64(abs))
	if resist < 0 {
		resistFloor = -resistFloor
	}
	coefficient := 1 - float64(resistFloor)/32.0
	return int(float64(base) * coefficient)
}

// NpcAttrResist 回傳 NPC 對武器屬性的等效抗性（Java PC_NPC：weak_attr == kind → -50，否則 0）。
func NpcAttrResist(weaponKind int8, npcWeakAttr int16) int {
	if weaponKind != AttrKindNone && int16(weaponKind) == npcWeakAttr {
		return -50
	}
	return 0
}
