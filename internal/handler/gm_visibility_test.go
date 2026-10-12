package handler

import (
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"testing"
)

func TestGMInvisibleMovementAndRebuildNeverRevealPlayer(t *testing.T) {
	ws := world.NewState()
	gm := addMovementShowIDPlayer(ws, &world.PlayerInfo{CharID: 1, SessionID: 1, Session: newMovementShowIDTestSession(t, 1), X: 100, Y: 100, MapID: 4, AccessLevel: 200, GMInvisible: true})
	viewer := addMovementShowIDPlayer(ws, &world.PlayerInfo{CharID: 2, SessionID: 2, Session: newMovementShowIDTestSession(t, 2), X: 102, Y: 100, MapID: 4, GMFindInvis: true})
	deps := &Deps{World: ws}
	HandleMove(gm.Session, packet.NewReader([]byte{0, 0, 0, 0, 0, 2}), deps)
	HandleChangeDirection(gm.Session, packet.NewReader([]byte{0, 4}), deps)
	SendPutObject(viewer.Session, gm)
	packets := drainMovementShowIDPackets(viewer.Session)
	if hasMovementShowIDMovePacket(packets, gm.CharID) || hasMovementShowIDChangeHeadingPacket(packets, gm.CharID) || hasMovementShowIDPutObjectPacket(packets, gm.CharID) {
		t.Fatal("GM 隱身不得由移動、轉向或角色重送向其他玩家洩漏")
	}
	if gm.X != 101 {
		t.Fatal("GM 隱身仍應更新真實位置")
	}
}
