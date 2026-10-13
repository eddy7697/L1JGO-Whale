package system

import (
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

// playerAttackViewers 對齊 Java pc.sendPackets + pc.broadcastPacket：
// 本人固定收到一次，旁觀者以攻擊者為中心，不能依目標位置漏掉畫面邊緣的動作。
func playerAttackViewers(ws *world.State, p *world.PlayerInfo) []*world.PlayerInfo {
	viewers := []*world.PlayerInfo{p}
	for _, viewer := range ws.GetNearbyPlayersInShow(p.X, p.Y, p.MapID, p.SessionID, p.ShowID) {
		if viewer.CanSeePlayer(p) {
			viewers = append(viewers, viewer)
		}
	}
	return viewers
}

func broadcastPlayerMeleeAttack(ws *world.State, p *world.PlayerInfo, targetID, damage int32) {
	data := handler.BuildAttackPacket(p.CharID, targetID, damage, p.Heading)
	for _, viewer := range playerAttackViewers(ws, p) {
		if viewer.Session != nil {
			viewer.Session.Send(data)
		}
	}
}

func broadcastPlayerRangedAttack(ws *world.State, p *world.PlayerInfo, targetID, damage, x, y int32) {
	for _, viewer := range playerAttackViewers(ws, p) {
		if viewer.Session != nil {
			handler.SendArrowAttackPacket(viewer.Session, p.CharID, targetID, damage, p.Heading, p.X, p.Y, x, y)
		}
	}
}
