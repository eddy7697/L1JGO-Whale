package system

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/handler"
	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// Exercise real registry/input/combat phases: a client sends one packet per step
// and per swing, even when several requests arrive before the next 200ms tick.
func TestNativeInputRouteAndRepeatedAttack(t *testing.T) {
	for _, tc := range []struct {
		name string
		xor  byte
	}{{"plain", 0}, {"taiwan", 0x49}} {
		t.Run(tc.name, func(t *testing.T) {
			ws := world.NewState()
			actor := &world.PlayerInfo{
				CharID: 1001, SessionID: 1, Session: newSkillTestSession(t, 1),
				X: 100, Y: 100, MapID: 4, ShowID: 7, HP: 100,
			}
			viewer := &world.PlayerInfo{
				CharID: 1002, SessionID: 2, Session: newSkillTestSession(t, 2),
				X: 100, Y: 101, MapID: 4, ShowID: 7,
			}
			ws.AddPlayer(actor)
			ws.AddPlayer(viewer)
			// Merchants acknowledge swings without damage calculations or death.
			target := &world.NpcInfo{ID: 2001, Impl: "L1Merchant", X: 104, Y: 100, MapID: 4, ShowID: 7, HP: 100, MaxHP: 100}
			ws.AddNpc(target)
			deps := &handler.Deps{World: ws, Config: &config.Config{Server: config.ServerConfig{Language: 3}}, Log: zap.NewNop()}
			combat := NewCombatSystem(deps)
			deps.Combat = combat
			registry := packet.NewRegistry(zap.NewNop())
			handler.RegisterAll(registry, deps)
			store := l1net.NewSessionStore()
			actor.Session.SetState(packet.StateInWorld)
			store.Add(actor.Session)
			store.Add(viewer.Session)
			input := NewInputSystem(&l1net.Server{}, registry, store, 32, nil, nil, nil, nil, ws, nil, nil, zap.NewNop())

			for x := 100; x < 103; x++ {
				w := packet.NewWriterWithOpcode(packet.C_OPCODE_MOVE)
				w.WriteH(uint16(x))
				w.WriteH(100)
				w.WriteC(2 ^ tc.xor)
				actor.Session.InQueue <- w.RawBytes()
			}
			for i := 0; i < 3; i++ {
				w := packet.NewWriterWithOpcode(packet.C_OPCODE_ATTACK)
				w.WriteD(target.ID)
				w.WriteH(uint16(target.X))
				w.WriteH(uint16(target.Y))
				actor.Session.InQueue <- w.RawBytes()
			}
			input.Update(200 * time.Millisecond)
			combat.Update(200 * time.Millisecond)
			if actor.X != 103 || actor.Y != 100 {
				t.Fatalf("batched route stopped at (%d,%d), want (103,100)", actor.X, actor.Y)
			}
			if target.HP != 100 {
				t.Fatalf("merchant HP changed: %d", target.HP)
			}
			own := drainSkillTestPackets(actor.Session)
			if len(own) != 3 {
				t.Fatalf("want one own action reply per swing and no movement reset, got %d packets", len(own))
			}
			for _, p := range own {
				if p[0] != packet.S_OPCODE_ATTACK || binary.LittleEndian.Uint32(p[2:6]) != uint32(actor.CharID) || binary.LittleEndian.Uint32(p[6:10]) != uint32(target.ID) {
					t.Fatalf("unexpected own reply: %x", p)
				}
			}
			seen := drainSkillTestPackets(viewer.Session)
			if len(seen) != 6 {
				t.Fatalf("observer got %d packets, want 3 steps and 3 swings", len(seen))
			}
			for i, p := range seen[:3] {
				if p[0] != packet.S_OPCODE_MOVE_OBJECT || binary.LittleEndian.Uint16(p[5:7]) != uint16(100+i) || p[9] != 2 {
					t.Fatalf("step %d not broadcast in order: %x", i, p)
				}
			}
			for _, p := range seen[3:] {
				if p[0] != packet.S_OPCODE_ATTACK {
					t.Fatalf("unexpected observer reply: %x", p)
				}
			}
			// No server timer invents more attacks after the client stops sending.
			combat.Update(200 * time.Millisecond)
			if extra := drainSkillTestPackets(actor.Session); len(extra) != 0 {
				t.Fatalf("unexpected autonomous attack replies: %x", extra)
			}
		})
	}
}
