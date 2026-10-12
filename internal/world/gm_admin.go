package world

// AllGroundItems 在遊戲迴圈逐一存取地面物，允許回呼移除當前物品。
func (s *State) AllGroundItems(fn func(*GroundItem)) {
	for _, item := range s.groundItems {
		fn(item)
	}
}
