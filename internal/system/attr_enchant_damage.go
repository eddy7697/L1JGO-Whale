package system

import "github.com/l1jgo/server/internal/world"

// 武器屬性強化附加傷害（Java L1Attack.calcAttrEnchantDmg）。
// Java 在 calcPcDamage 中把此值加到 weaponDamage（與 _weaponAddDmg、武器衝裝同階段），
// Go 對應做法是併入 CombatContext.AttackerDmgMod / RangedCombatContext.AttackerBowDmgMod。

// weaponAttrEnchantDamageVsNpc 計算玩家武器屬性強化對 NPC 的附加傷害。
// Java PC_NPC：目標 weak_attr 等於武器屬性 → resist = -50（傷害 ×1.5），否則不加減。
func weaponAttrEnchantDamageVsNpc(player *world.PlayerInfo, npc *world.NpcInfo) int {
	if player == nil || npc == nil {
		return 0
	}
	wpn := player.Equip.Weapon()
	if wpn == nil || wpn.AttrEnchantLevel <= 0 {
		return 0
	}
	return world.WeaponAttrEnchantDamage(wpn.AttrEnchantLevel, world.NpcAttrResist(wpn.AttrEnchantKind, npc.WeakAttr))
}

// weaponAttrEnchantDamageVsPc 計算玩家武器屬性強化對玩家的附加傷害。
// Java PC_PC：依武器屬性取目標對應的屬性抗性（getEarth/getFire/getWater/getWind）。
func weaponAttrEnchantDamageVsPc(attacker, target *world.PlayerInfo) int {
	if attacker == nil || target == nil {
		return 0
	}
	wpn := attacker.Equip.Weapon()
	if wpn == nil || wpn.AttrEnchantLevel <= 0 {
		return 0
	}
	resist := 0
	switch wpn.AttrEnchantKind {
	case world.AttrKindEarth:
		resist = int(target.EarthRes)
	case world.AttrKindFire:
		resist = int(target.FireRes)
	case world.AttrKindWater:
		resist = int(target.WaterRes)
	case world.AttrKindWind:
		resist = int(target.WindRes)
	}
	return world.WeaponAttrEnchantDamage(wpn.AttrEnchantLevel, resist)
}
