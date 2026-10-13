package handler

import (
	"testing"

	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// Java C_Attack: getWeight242() >= 197 → S_ServerMessage(110), attack is not processed.
func TestHandleAttackBlockedByOverweightLikeJava(t *testing.T) {
	tests := []struct {
		name   string
		handle func(*net.Session, *packet.Reader, *Deps)
	}{
		{name: "melee", handle: HandleAttack},
		{name: "ranged", handle: HandleFarAttack},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := world.NewState()
			sess := newHandlerTestSession(t, 1)
			player := &world.PlayerInfo{
				SessionID: sess.ID,
				Session:   sess,
				CharID:    1001,
				Name:      "heavy",
				Str:       1,
				Con:       1,
				Inv:       world.NewInventory(),
			}
			// MaxWeight(1,1) = 300; 2500 units → bar 242 (>= 197).
			player.Inv.AddItem(40001, 2500, "heavy item", 0, 1000, true, 0)
			ws.AddPlayer(player)

			combat := &captureCombatQueue{}
			deps := &Deps{World: ws, Combat: combat, Log: zap.NewNop()}

			tt.handle(sess, attackReader(2002), deps)

			if len(combat.requests) != 0 {
				t.Fatalf("Java C_Attack 在 getWeight240 >= 197 時拒絕攻擊，不應排入 CombatQueue，requests=%+v", combat.requests)
			}
			if !hasHandlerServerMessage(drainHandlerTestPackets(sess), 110) {
				t.Fatal("Java C_Attack 負重過高時應送 S_ServerMessage(110)")
			}
		})
	}
}

func TestMoveAtFullWeightIsCorrectedAndResumesAfterDroppingWeight(t *testing.T) {
	p, viewer, deps := newMovementProtocolPlayers(t, 3)
	p.Str, p.Con, p.ClassType = 1, 1, 1 // capacity 300
	p.Inv = world.NewInventory()
	it := p.Inv.AddItem(40010, 300, "weight", 0, 1000, true, 1)
	HandleMove(p.Session, moveReader(uint16(p.X), uint16(p.Y), 2), deps)
	if p.X != 100 || p.Y != 100 || p.LastMoveTime != 0 {
		t.Fatal("full weight must stop movement before changing world state")
	}
	if packets := drainMovementShowIDPackets(p.Session); len(packets) == 0 {
		t.Fatal("client-predicted movement must receive a correction")
	}
	if hasMovementShowIDMovePacket(drainMovementShowIDPackets(viewer.Session), p.CharID) {
		t.Fatal("rejected movement must not be broadcast")
	}
	p.Inv.RemoveItem(it.ObjectID, 1) // 299/300 → bar 241
	HandleMove(p.Session, moveReader(uint16(p.X), uint16(p.Y), 2), deps)
	if p.X != 101 || p.Y != 100 {
		t.Fatal("movement must resume immediately below full weight")
	}
}

// Below the 197 stage the attack must still reach the combat queue.
func TestHandleAttackAllowedBelowOverweightStage(t *testing.T) {
	ws := world.NewState()
	sess := newHandlerTestSession(t, 1)
	player := &world.PlayerInfo{
		SessionID: sess.ID,
		Session:   sess,
		CharID:    1001,
		Name:      "light",
		Str:       1,
		Con:       1,
		Inv:       world.NewInventory(),
	}
	// 200 of 300 → bar 161 (< 197).
	player.Inv.AddItem(40001, 200, "item", 0, 1000, true, 0)
	ws.AddPlayer(player)

	combat := &captureCombatQueue{}
	deps := &Deps{World: ws, Combat: combat, Log: zap.NewNop()}

	HandleAttack(sess, attackReader(2002), deps)

	if len(combat.requests) != 1 {
		t.Fatalf("below the 197 weight stage the attack should be queued, requests=%+v", combat.requests)
	}
}
