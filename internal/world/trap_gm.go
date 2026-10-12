package world

import "github.com/l1jgo/server/internal/data"

// GetVisibleTrapsInScreen 包含 Java 的 trap base；這些標記永遠不參與踩踏判定。
func (mgr *TrapManager) GetVisibleTrapsInScreen(x, y int32, mapID int16) []*TrapInstance {
	traps := mgr.GetTrapsInScreen(x, y, mapID)
	for _, base := range mgr.allBases {
		if base.MapID == mapID && trapInScreenLikeJava(x, y, base.X, base.Y) {
			traps = append(traps, base)
		}
	}
	return traps
}

// ResetAllTraps 重建座標索引並清掉舊重生佇列，避免同一陷阱被重複註冊。
func (mgr *TrapManager) ResetAllTraps() {
	if mgr == nil {
		return
	}
	mgr.byTile = make(map[trapTileKey][]*TrapInstance)
	mgr.pendingRespawn = nil
	for _, trap := range mgr.allTraps {
		mgr.randomizePosition(trap)
		trap.Alive = true
		mgr.registerTile(trap)
	}
}

// Reload 就地替換，保留 TrapRespawnSystem 持有的指標。
func (mgr *TrapManager) Reload(traps *data.TrapData) {
	if mgr == nil || traps == nil {
		return
	}
	*mgr = *NewTrapManager(traps, mgr.mapChecker)
}
