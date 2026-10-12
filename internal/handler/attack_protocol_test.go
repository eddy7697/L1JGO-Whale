package handler

import (
	"testing"

	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

func TestClientAttackPreservesTargetPositionAndRejectsTruncatedPackets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		opcode byte
		handle func(*net.Session, *packet.Reader, *Deps)
		melee  bool
	}{
		{"melee", packet.C_OPCODE_ATTACK, HandleAttack, true},
		{"ranged", packet.C_OPCODE_FAR_ATTACK, HandleFarAttack, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := world.NewState()
			sess := newHandlerTestSession(t, 1)
			ws.AddPlayer(&world.PlayerInfo{SessionID: sess.ID, Session: sess, CharID: 1001})
			queue := &captureCombatQueue{}
			deps := &Deps{World: ws, Combat: queue}
			w := packet.NewWriterWithOpcode(tc.opcode)
			w.WriteD(2001)
			w.WriteH(33001)
			w.WriteH(33002)
			payload := w.RawBytes()
			for size := 1; size < len(payload); size++ {
				tc.handle(sess, packet.NewReader(payload[:size]), deps)
			}
			if len(queue.requests) != 0 {
				t.Fatalf("截短攻擊不應排入佇列: %+v", queue.requests)
			}
			tc.handle(sess, packet.NewReader(payload), deps)
			if len(queue.requests) != 1 {
				t.Fatalf("完整攻擊應排入一次，got=%d", len(queue.requests))
			}
			got := queue.requests[0]
			if got.TargetID != 2001 || got.TargetX != 33001 || got.TargetY != 33002 || !got.FromClient || got.IsMelee != tc.melee {
				t.Fatalf("目標死亡／消失時仍需要原封包座標: %+v", got)
			}
		})
	}
}
