package system

// 隱身解除延遲（Java: L1PcInstance.beginInvisTimer / L1PcInvisDelay，DELAY_INVIS = 3000ms）。
// 無所遁形術揭露自己、脫下隱身斗篷後 3 秒內不可攻擊、撿物，也不能施放非隱身可用技能。
const InvisDelayTicks = 15 // 3 秒（200ms tick）

// castWithInvisLikeJava Java L1SkillUse.CAST_WITH_INVIS：隱身中／延遲中仍可施放的技能。
var castWithInvisLikeJava = map[int32]struct{}{}

func init() {
	for _, id := range []int32{
		1, 2, 3, 5, 8, 9, 12, 13, 14, 19, 21, 26, 31, 32, 35, 37, 42, 43, 44,
		48, 49, 52, 54, 55, 57, 60, 61, 63, 67, 68, 69, 72, 73, 75, 78, 79, 88, 89,
		90, 91, 97, 98, 99, 100, 101, 102, 104, 105, 106, 107, 109, 110, 111, 113, 114,
		115, 116, 117, 118, 129, 130, 131, 133, 134, 137, 138, 146, 147, 148, 149, 150, 151, 155, 156, 158, 159,
		163, 164, 165, 166, 168, 169, 170, 171, 175, 176, 181, 185,
		190, 195, 201, 204, 209, 211, 214,
		216, 219,
	} {
		castWithInvisLikeJava[id] = struct{}{}
	}
}

// castableWithInvisLikeJava 回傳技能是否屬於 Java CAST_WITH_INVIS 清單。
func castableWithInvisLikeJava(skillID int32) bool {
	_, ok := castWithInvisLikeJava[skillID]
	return ok
}
