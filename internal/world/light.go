package world

// ItemLightRange 對照 Eddy L1Item.getLightRange；數值為客戶端光圈大小。
func ItemLightRange(itemID int32) byte {
	switch itemID {
	case 40001:
		return 11
	case 40002, 40004:
		return 14
	case 40005:
		return 8
	}
	return 0
}

// ItemLightFuel 對照 Eddy L1Item.getLightFuel。魔法燈籠不需要燃料。
// Eddy L1PcInventory.loadItems 會於登入時重新填滿燈具燃料。
func ItemLightFuel(itemID int32) int32 {
	switch itemID {
	case 40001:
		return 6000
	case 40002, 40003:
		return 12000
	case 40005:
		return 600
	}
	return 0
}
