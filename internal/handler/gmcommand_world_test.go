package handler

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
)

func TestGMHPBarUpdatesOnlyVisibleTargetsAndClearsOnOff(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	p := &world.PlayerInfo{Session: sess, SessionID: 1, CharID: 1, X: 100, Y: 100, MapID: 4, ShowID: 8, Known: world.NewKnownEntities(), AccessLevel: 200}
	ws := world.NewState()
	ws.AddPlayer(p)
	ws.AddNpc(&world.NpcInfo{ID: 10, Impl: "L1Monster", X: 101, Y: 100, MapID: 4, ShowID: 8, HP: 50, MaxHP: 100})
	ws.AddNpc(&world.NpcInfo{ID: 11, Impl: "L1Monster", X: 101, Y: 100, MapID: 4, ShowID: 9, HP: 50, MaxHP: 100})
	ws.AddNpc(&world.NpcInfo{ID: 12, Impl: "L1Merchant", X: 101, Y: 100, MapID: 4, ShowID: 8, HP: 50, MaxHP: 100})
	deps := &Deps{World: ws}
	dispatchGMWorld(sess, p, "hpbar", []string{"on"}, deps)
	packets := drainHandlerTestPackets(sess)
	if !gmWorldHasHPMeter(packets, 10, 50) || gmWorldHasHPMeter(packets, 11, 50) || gmWorldHasHPMeter(packets, 12, 50) {
		t.Fatal("hpbar leaked show or noncombat NPC")
	}
	ws.GetNpc(10).HP = 25
	UpdateGMWorldView(p, deps)
	if !gmWorldHasHPMeter(drainHandlerTestPackets(sess), 10, 25) {
		t.Fatal("damage must update enabled hpbar")
	}
	UpdateGMWorldView(p, deps)
	if len(drainHandlerTestPackets(sess)) != 0 {
		t.Fatal("unchanged HP must not flood packets")
	}
	dispatchGMWorld(sess, p, "hpbar", []string{"off"}, deps)
	if !gmWorldHasHPMeter(drainHandlerTestPackets(sess), 10, 255) {
		t.Fatal("hpbar off must clear existing bars")
	}
}

func gmWorldHasHPMeter(packets [][]byte, id int32, ratio byte) bool {
	for _, p := range packets {
		if len(p) >= 6 && p[0] == packet.S_OPCODE_HP_METER && int32(binary.LittleEndian.Uint32(p[1:5])) == id && p[5] == ratio {
			return true
		}
	}
	return false
}

func TestGMShowTrapTracksDisableAndReload(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	p := &world.PlayerInfo{Session: sess, SessionID: 1, CharID: 1, X: 100, Y: 100, MapID: 4, Known: world.NewKnownEntities()}
	ws := world.NewState()
	ws.AddPlayer(p)
	td := &data.TrapData{Templates: map[int32]*data.TrapTemplate{1: {TrapID: 1}}, Spawns: []data.TrapSpawn{{TrapID: 1, X: 100, Y: 100, MapID: 4, Count: 1}}}
	mgr := world.NewTrapManager(td, nil)
	deps := &Deps{World: ws, TrapMgr: mgr}
	dispatchGMWorld(sess, p, "showtrap", []string{"on"}, deps)
	if len(p.Known.GMTraps) != 2 {
		t.Fatal("showtrap must include trap and trap base")
	}
	drainHandlerTestPackets(sess)
	mgr.DisableTrap(mgr.GetTrapsAt(100, 100, 4)[0])
	UpdateGMWorldView(p, deps)
	if len(p.Known.GMTraps) != 1 || len(drainHandlerTestPackets(sess)) != 1 {
		t.Fatal("triggered trap must disappear while base remains")
	}
	dispatchGMWorld(sess, p, "showtrap", []string{"off"}, deps)
	if len(p.Known.GMTraps) != 0 {
		t.Fatal("off must clear every marker")
	}
}

func TestGMFavoriteSubstitutesParametersAndRejectsRecursion(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	p := &world.PlayerInfo{Session: sess, SessionID: 1, CharID: 123, AccessLevel: 200}
	deps := &Deps{World: world.NewState()}
	gmFavorite(sess, p, []string{"set", "action", "%1"}, deps)
	drainHandlerTestPackets(sess)
	gmFavorite(sess, p, []string{"68"}, deps)
	if !hasHandlerActionGfxPacket(drainHandlerTestPackets(sess), 123, 68) {
		t.Fatal("favorite must call real command with substitution")
	}
	for _, forbidden := range []string{".F", "r", "書籤", "doesnotexist"} {
		gmFavorite(sess, p, []string{"set", forbidden}, deps)
		if p.GMFavorite != "action %1" {
			t.Fatalf("must reject recursive or unknown command %q", forbidden)
		}
	}
	gmFavorite(sess, p, nil, deps)
	if hasHandlerActionGfxPacket(drainHandlerTestPackets(sess), 123, 68) {
		t.Fatal("missing argument must not execute command")
	}
}

func TestGMWorldToggleKeepsCommandAuthority(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	p := &world.PlayerInfo{Session: sess, CharID: 1, AccessLevel: 200}
	deps := &Deps{World: world.NewState()}
	dispatchGMWorld(sess, p, "gm", []string{"off"}, deps)
	if p.IsGM() || !p.CanUseGMCommands() || p.AccessLevel != 200 {
		t.Fatal("gm off must disable runtime privilege without revoking command authority")
	}
	dispatchGMWorld(sess, p, "gm", []string{"bad"}, deps)
	if p.IsGM() {
		t.Fatal("invalid argument must preserve mode")
	}
	dispatchGMWorld(sess, p, "gm", []string{"on"}, deps)
	if !p.IsGM() {
		t.Fatal("gm on must restore runtime privilege")
	}
}

func TestGMPatrolTimestampMatchesJavaTaipeiYear1970(t *testing.T) {
	active := time.Date(2026, 10, 13, 2, 3, 4, 0, time.UTC)
	want := time.Date(1970, 10, 13, 2, 3, 4, 0, time.UTC).Unix()
	if got := gmPatrolTimestamp(active); int64(got) != want {
		t.Fatalf("patrol time=%d, want %d", got, want)
	}
	if gmPatrolTimestamp(time.Time{}) != 0 {
		t.Fatal("missing login time must use Java fallback 0")
	}
}

func TestGMPatrolUsesJavaPacketBoxAndPlayerList(t *testing.T) {
	ws := world.NewState()
	sess := newHandlerTestSession(t, 1)
	ws.AddPlayer(&world.PlayerInfo{Session: sess, SessionID: 1, CharID: 1, Name: "GM", ClanName: "Staff"})
	gmPatrol(sess, &Deps{World: ws})
	packets := drainHandlerTestPackets(sess)
	if len(packets) != 1 {
		t.Fatalf("patrol packets=%d", len(packets))
	}
	r := packet.NewReader(packets[0])
	if r.Opcode() != 250 || r.ReadC() != 45 || r.ReadC() != 1 {
		t.Fatalf("wrong Java patrol header: %v", packets[0])
	}
	if r.ReadD() != 0 || r.ReadS() != "GM" || r.ReadS() != "Staff" || r.Remaining() != 0 {
		t.Fatalf("wrong Java patrol fields: %v", packets[0])
	}
}

func TestGMCastGfxKeepsGMInvisibleAndShowPrivate(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		ws := world.NewState()
		players := make([]*world.PlayerInfo, 3)
		for i := range players {
			id := uint64(i + 1)
			players[i] = &world.PlayerInfo{SessionID: id, Session: newHandlerTestSession(t, id), CharID: int32(id), X: 100, Y: 100, MapID: 4, ShowID: 10}
			ws.AddPlayer(players[i])
		}
		players[0].GMInvisible = hidden
		players[2].ShowID = 20
		dispatchGMWorld(players[0].Session, players[0], "castgfx", []string{"7976"}, &Deps{World: ws})
		if packets := drainHandlerTestPackets(players[0].Session); len(packets) != 1 {
			t.Fatalf("self effect packets = %d", len(packets))
		}
		want := 1
		if hidden {
			want = 0
		}
		if packets := drainHandlerTestPackets(players[1].Session); len(packets) != want {
			t.Fatalf("hidden=%t same-show packets = %d, want %d", hidden, len(packets), want)
		}
		if packets := drainHandlerTestPackets(players[2].Session); len(packets) != 0 {
			t.Fatalf("effect leaked across ShowID: %v", packets)
		}
	}
}
