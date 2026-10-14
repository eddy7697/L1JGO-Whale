package system

import (
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

// ==================== 屬性衍生公式（Java: CalcStat / L1PcInstance.resetBaseAc / resetBaseMr） ====================

// CalcAcLikeJava 回傳等級與敏捷換算的基礎 AC（Java: CalcStat.calcAc）。
// baseAC 為起始值（Java 寫死 10，Go 取 [gameplay] base_ac），敏捷越高、等級越高 AC 越低。
func CalcAcLikeJava(baseAC int, level, dex int16) int16 {
	ac := baseAC
	lv := int(level)
	switch {
	case dex <= 9:
		ac -= lv / 8
	case dex <= 12:
		ac -= lv / 7
	case dex <= 15:
		ac -= lv / 6
	case dex <= 17:
		ac -= lv / 5
	default:
		ac -= lv / 4
	}
	return int16(ac)
}

// CalcStatMrLikeJava 回傳精神對應的抗魔加成（Java: CalcStat.calcStatMr）。
func CalcStatMrLikeJava(wis int16) int16 {
	switch {
	case wis <= 14:
		return 0
	case wis <= 16:
		return 3
	case wis == 17:
		return 6
	case wis == 18:
		return 10
	case wis == 19:
		return 15
	case wis == 20:
		return 21
	case wis == 21:
		return 28
	case wis == 22:
		return 37
	case wis == 23:
		return 47
	case wis <= 29:
		return 50
	case wis <= 34:
		return 52
	case wis <= 39:
		return 55
	case wis <= 43:
		return 59
	case wis <= 46:
		return 62
	case wis <= 49:
		return 64
	default:
		return 65
	}
}

// classBaseMrLikeJava 回傳職業固定抗魔（Java: L1PcInstance.resetBaseMr）。
func classBaseMrLikeJava(classType int16) int16 {
	switch classType {
	case 0: // 王族
		return 10
	case 1: // 騎士
		return 0
	case 2: // 妖精
		return 25
	case 3: // 法師
		return 15
	case 4: // 黑暗妖精
		return 10
	case 5: // 龍騎士
		return 18
	case 6: // 幻術師
		return 20
	}
	return 0
}

// CalcBaseMrLikeJava 回傳職業、精神與等級換算的基礎 MR（Java: resetBaseMr = 職業值 + calcStatMr(wis) + level/2）。
func CalcBaseMrLikeJava(classType, level, wis int16) int16 {
	return classBaseMrLikeJava(classType) + CalcStatMrLikeJava(wis) + level/2
}

// playerBaseDex 回傳扣除裝備與 buff 加成後的原始敏捷（Java: getBaseDex）。
func playerBaseDex(p *world.PlayerInfo) int16 {
	dex := p.Dex - int16(p.EquipBonuses.AddDex)
	for _, buff := range p.ActiveBuffs {
		dex -= buff.DeltaDex
	}
	return dex
}

// ResetBaseAC 依目前等級與原始敏捷重算基礎 AC，並以差值套用到 AC（Java: resetBaseAc）。
// 裝備、buff 等其他 AC 來源不受影響。
func ResetBaseAC(p *world.PlayerInfo, deps *handler.Deps) {
	baseAC := 10
	if deps != nil && deps.Config != nil {
		baseAC = deps.Config.Gameplay.BaseAC
	}
	newAC := CalcAcLikeJava(baseAC, p.Level, playerBaseDex(p))
	p.AC += newAC - p.BaseAC
	p.BaseAC = newAC
}

// ResetBaseMR 依職業、目前精神（含裝備與 buff）與等級重算基礎 MR，並以差值套用到 MR（Java: resetBaseMr）。
// 回傳 MR 是否改變，呼叫端據此決定是否送 S_SPMR。
func ResetBaseMR(p *world.PlayerInfo) bool {
	newMR := CalcBaseMrLikeJava(p.ClassType, p.Level, p.Wis)
	if newMR == p.BaseMR {
		return false
	}
	p.MR += newMR - p.BaseMR
	p.BaseMR = newMR
	return true
}

// ResetBaseStats 等級或屬性改變後重算基礎 AC 與 MR（Java: levelUp / levelDown / C_Attr）。
func ResetBaseStats(p *world.PlayerInfo, deps *handler.Deps) {
	ResetBaseAC(p, deps)
	ResetBaseMR(p)
}

// ==================== 高等級經驗懲罰（Java: ExpTable.getPenaltyRate） ====================

// javaExpPenaltyDivisors 為 Java charsettings.properties 的 Lv50Exp..Lv99Exp 預設值：
// 50 級以上取得的經驗除以對應除數。
var javaExpPenaltyDivisors = []int{
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 50-59
	1, 1, 1, 1, 1, 2, 2, 2, 2, 2, // 60-69
	4, 4, 4, 4, 4, 8, 8, 8, 8, 16, // 70-79
	32, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, // 80-89
	32768, 65536, 131072, 262144, 524288, 1048576, 2097152, 4194304, 8388608, 16777216, // 90-99
}

// ExpPenaltyRateLikeJava 回傳等級對應的經驗倍率（Java: ExpTable.getPenaltyRate）。
// divisors 為 50 級起的除數表；nil 或不足時以 Java 預設補齊。
func ExpPenaltyRateLikeJava(level int16, divisors []int) float64 {
	if level < 50 {
		return 1.0
	}
	idx := int(level) - 50
	table := javaExpPenaltyDivisors
	if idx < len(divisors) && divisors[idx] > 0 {
		return 1.0 / float64(divisors[idx])
	}
	if idx >= len(table) {
		idx = len(table) - 1
	}
	return 1.0 / float64(table[idx])
}
