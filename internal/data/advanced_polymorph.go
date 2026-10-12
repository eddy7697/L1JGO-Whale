package data

// Java AdvancedPolymorphCardMap v1：卡片 600101..600150 與變身外觀的固定對照。
// 600100 在本專案為涼鞋，不匯入 Java 的同號卡冊。
var advancedPolymorphIDs = [...]int32{
	14491, 12283, 12286, 12280, 12314, 12295, 12681, 13346, 12240, 15548,
	15537, 15550, 15539, 13152, 12237, 13635, 15831, 15830, 15528, 15833,
	15832, 15531, 13715, 13717, 13719, 13721, 13723, 13725, 13727, 13729,
	13731, 13733, 13735, 13737, 13739, 13741, 13743, 13745, 13631, 15814,
	15986, 16002, 16008, 16014, 16027, 16040, 16053, 16056, 16074, 16284,
}

func AdvancedPolymorphID(cardID int32) (int32, bool) {
	index := cardID - 600101
	if index < 0 || index >= int32(len(advancedPolymorphIDs)) {
		return 0, false
	}
	return advancedPolymorphIDs[index], true
}

func AdvancedPolymorphCard(polyID int32) (int32, bool) {
	for index, id := range advancedPolymorphIDs {
		if id == polyID {
			return int32(index) + 600101, true
		}
	}
	return 0, false
}
