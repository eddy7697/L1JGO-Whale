package system

import (
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

// sendThirdSpeedToAll 僅通知本人與同副本內看得見該角色的玩家。
func sendThirdSpeedToAll(player *world.PlayerInfo, liquorType byte, deps *handler.Deps) {
	if player == nil {
		return
	}
	payload := handler.BuildLiquor(player.CharID, liquorType)
	if player.Session != nil {
		player.Session.Send(payload)
	}
	if deps == nil || deps.World == nil {
		return
	}
	for _, viewer := range deps.World.GetNearbyPlayersInShow(player.X, player.Y, player.MapID, player.SessionID, player.ShowID) {
		if viewer.Session != nil && viewer.CanSeePlayer(player) {
			viewer.Session.Send(payload)
		}
	}
}
