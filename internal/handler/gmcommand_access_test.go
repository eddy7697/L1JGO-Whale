package handler

import (
	"strings"
	"testing"

	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

func TestGMCommandEntryRejectsNonStaff(t *testing.T) {
	for _, level := range []int16{0, 49, 50, 99, 101, 150, 199, 201, 32767} {
		capture := &captureGMCommandManager{}
		sess := newHandlerTestSession(t, 1)
		p := &world.PlayerInfo{AccessLevel: level, Session: sess}
		if HandleGMCommand(sess, p, ".lawful +1", &Deps{GMCmd: capture}) || capture.lawfulCalled {
			t.Fatalf("access %d executed a staff command", level)
		}
	}
}

func TestMonitorCannotBypassAccessThroughAliasesOrRepeat(t *testing.T) {
	for _, command := range []string{".item 40010", ".物品 40010", ".law +1", ".r"} {
		capture := &captureGMCommandManager{}
		sess := newHandlerTestSession(t, 1)
		p := &world.PlayerInfo{AccessLevel: 100, Session: sess, LastGMCommand: "lawful +1"}
		HandleGMCommand(sess, p, command, &Deps{GMCmd: capture})
		if capture.called || capture.lawfulCalled {
			t.Fatalf("monitor bypassed access with %s", command)
		}
	}
}

func TestGMRepeatReplacesArgumentsAndKeepsAuthorization(t *testing.T) {
	capture := &captureGMCommandManager{}
	sess := newHandlerTestSession(t, 1)
	p := &world.PlayerInfo{AccessLevel: 200, Session: sess}
	deps := &Deps{GMCmd: capture, Log: zap.NewNop()}
	HandleGMCommand(sess, p, ".LAW +123", deps)
	if !capture.lawfulCalled || p.LastGMCommand != "lawful +123" {
		t.Fatal("alias did not retain canonical last command")
	}
	HandleGMCommand(sess, p, ".r -99", deps)
	if capture.lawfulDelta != -99 {
		t.Fatal("repeat did not replace arguments")
	}
	p.AccessLevel = 100
	capture.lawfulCalled = false
	HandleGMCommand(sess, p, ".r", deps)
	if capture.lawfulCalled {
		t.Fatal("repeat retained revoked authority")
	}
}

func TestGMModeOffCanBeRestoredThroughCommand(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	p := &world.PlayerInfo{AccessLevel: 200, Session: sess}
	HandleGMCommand(sess, p, ".gm off", &Deps{})
	if p.IsGM() {
		t.Fatal("gm off did not disable game privileges")
	}
	HandleGMCommand(sess, p, ".遊戲管理員 on", &Deps{})
	if !p.IsGM() {
		t.Fatal("gm off prevented restoring privileges")
	}
}

func TestGMInvisibleCommandsAreIdempotentAndPreserveSpell(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	p := &world.PlayerInfo{AccessLevel: 200, Session: sess, SessionID: 1, CharID: 1, Invisible: true}
	ws := world.NewState()
	ws.AddPlayer(p)
	deps := &Deps{World: ws}
	HandleGMCommand(sess, p, ".隱身", deps)
	HandleGMCommand(sess, p, ".invisible", deps)
	if !p.GMInvisible {
		t.Fatal("repeating invisible toggled visibility")
	}
	HandleGMCommand(sess, p, ".visible", deps)
	if p.GMInvisible || !p.Invisible {
		t.Fatal("visible must only clear staff invisibility")
	}
}

type settingGMManager struct{ captureGMCommandManager }

func (m *settingGMManager) SetStat(_ *net.Session, p *world.PlayerInfo, stat string, value int16) {
	if stat == "str" {
		p.Str = value
	}
}

func TestGMSettingSavesBaseStatsAndPreservesOtherFields(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	p := &world.PlayerInfo{Session: sess, Inv: world.NewInventory(), Str: 20, Food: 180, Karma: 1000, PKCount: 3, ElixirStats: 2, MaxHP: 300}
	p.EquipBonuses.AddStr = 2
	p.AddBuff(&world.ActiveBuff{SkillID: 42, TicksLeft: 100, DeltaStr: 5, DeltaMaxHP: 50})
	gmSetting(sess, p, []string{"me", "str", "25"}, &Deps{GMCmd: &settingGMManager{}})
	row := gmCharacterRow(p)
	if p.Str != 32 || row.Str != 25 || row.MaxHP != 250 || row.Food != 180 || row.Karma != 1000 || row.PKCount != 3 || row.ElixirStats != 2 {
		t.Fatalf("wrong persisted base snapshot: %+v", row)
	}
}

func TestGMAddSkillIncludesBasicAndClassMagic(t *testing.T) {
	for _, c := range []struct {
		class   int16
		yes, no int32
	}{{0, 16, 17}, {1, 8, 9}, {2, 48, 49}, {3, 80, 81}, {4, 16, 17}, {5, 195, 196}, {6, 220, 221}} {
		if !gmClassCanLearnSkill(c.class, c.yes) || gmClassCanLearnSkill(c.class, c.no) {
			t.Fatalf("wrong Java skill boundary for class %d", c.class)
		}
	}
}

func TestMonitorHelpDoesNotAdvertiseAdministratorCommands(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	p := &world.PlayerInfo{AccessLevel: 100, Session: sess}
	gmCommandHelp(sess, p, []string{"item"})
	for _, payload := range drainMovementShowIDPackets(sess) {
		if strings.Contains(string(payload), ".item") {
			t.Fatal("restricted command leaked into monitor help")
		}
	}
}

func TestGMRecallAndTopcEnterTheDestinationInstance(t *testing.T) {
	ws := world.NewState()
	gm := addNpcActionShowIDPlayer(ws, &world.PlayerInfo{Session: newHandlerTestSession(t, 1), SessionID: 1, CharID: 1, Name: "gm", AccessLevel: 200, X: 100, Y: 100, MapID: 4, ShowID: 10, Inv: world.NewInventory()})
	target := addNpcActionShowIDPlayer(ws, &world.PlayerInfo{Session: newHandlerTestSession(t, 2), SessionID: 2, CharID: 2, Name: "target", X: 120, Y: 120, MapID: 4, ShowID: 20, Inv: world.NewInventory()})
	deps := &Deps{World: ws}
	HandleGMCommand(gm.Session, gm, ".topc target", deps)
	if gm.ShowID != target.ShowID || gm.X != target.X || gm.Y != target.Y {
		t.Fatal("topc failed to enter target instance")
	}
	gm.ShowID = 30
	HandleGMCommand(gm.Session, gm, ".recall target", deps)
	if target.ShowID != 30 {
		t.Fatal("recall left target in original instance")
	}
}

func TestGMKillAllDoesNotAffectOtherInstances(t *testing.T) {
	ws := world.NewState()
	sess := newHandlerTestSession(t, 1)
	gm := &world.PlayerInfo{Session: sess, SessionID: 1, CharID: 1, AccessLevel: 200, X: 100, Y: 100, MapID: 4, ShowID: 10}
	ws.AddPlayer(gm)
	a := &world.NpcInfo{ID: 10, X: 101, Y: 100, MapID: 4, ShowID: 10, HP: 100}
	b := &world.NpcInfo{ID: 20, X: 101, Y: 100, MapID: 4, ShowID: 20, HP: 100}
	ws.AddNpc(a)
	ws.AddNpc(b)
	HandleGMCommand(sess, gm, ".killall", &Deps{World: ws})
	if !a.Dead || b.Dead || b.HP != 100 {
		t.Fatal("killall crossed the instance boundary")
	}
}

func TestGMExpRejectsOverflowBeforeCombatMutation(t *testing.T) {
	sess := newHandlerTestSession(t, 1)
	p := &world.PlayerInfo{Session: sess, AccessLevel: 200, Exp: 100}
	// A nil combat service ensures overflow is rejected before dispatching it.
	HandleGMCommand(sess, p, ".exp 2147483647", &Deps{})
	if p.Exp != 100 {
		t.Fatal("overflow changed experience")
	}
}

func TestGMRezUsesResurrectionAndDoesNotRevealAcrossInstances(t *testing.T) {
	ws := world.NewState()
	gm := addNpcActionShowIDPlayer(ws, &world.PlayerInfo{Session: newHandlerTestSession(t, 1), SessionID: 1, CharID: 1, Name: "gm", AccessLevel: 200, X: 100, Y: 100, MapID: 4, ShowID: 10, Inv: world.NewInventory()})
	target := addNpcActionShowIDPlayer(ws, &world.PlayerInfo{Session: newHandlerTestSession(t, 2), SessionID: 2, CharID: 2, Name: "dead", X: 101, Y: 100, MapID: 4, ShowID: 10, Inv: world.NewInventory(), Dead: true, MaxHP: 100, MaxMP: 50})
	other := addNpcActionShowIDPlayer(ws, &world.PlayerInfo{Session: newHandlerTestSession(t, 3), SessionID: 3, CharID: 3, X: 101, Y: 100, MapID: 4, ShowID: 20, Inv: world.NewInventory()})
	death := &fakeResurrectionDeathManager{}
	HandleGMCommand(gm.Session, gm, ".rez dead", &Deps{World: ws, Death: death})
	if target.Dead || target.HP != 100 || !target.Dirty {
		t.Fatal("rez did not restore player state")
	}
	if len(drainMovementShowIDPackets(other.Session)) != 0 {
		t.Fatal("rez leaked packets across instances")
	}
}

func TestGMSpeedResetClearsAllStagesWithoutCrossingInstances(t *testing.T) {
	ws := world.NewState()
	gm := addNpcActionShowIDPlayer(ws, &world.PlayerInfo{Session: newHandlerTestSession(t, 1), SessionID: 1, CharID: 1, AccessLevel: 200, X: 100, Y: 100, MapID: 4, ShowID: 10, Inv: world.NewInventory()})
	other := addNpcActionShowIDPlayer(ws, &world.PlayerInfo{Session: newHandlerTestSession(t, 2), SessionID: 2, CharID: 2, X: 100, Y: 100, MapID: 4, ShowID: 20, Inv: world.NewInventory()})
	deps := &Deps{World: ws}
	HandleGMCommand(gm.Session, gm, ".speed", deps)
	if gm.MoveSpeed != 1 || gm.BraveSpeed != 1 || !gm.HasBuff(SkillStatusThirdSpeed) {
		t.Fatal("Java speed command did not apply all three stages")
	}
	drainMovementShowIDPackets(gm.Session)
	HandleGMCommand(gm.Session, gm, ".speed 0", deps)
	if gm.MoveSpeed != 0 || gm.BraveSpeed != 0 || gm.HasteTicks != 0 || gm.BraveTicks != 0 || gm.HasBuff(SkillStatusThirdSpeed) {
		t.Fatal("speed reset retained an active stage")
	}
	clearedThird := false
	for _, payload := range drainMovementShowIDPackets(gm.Session) {
		if len(payload) == 8 && payload[0] == packet.S_OPCODE_DRUNKEN && payload[5] == 0 {
			clearedThird = true
		}
	}
	if !clearedThird {
		t.Fatal("speed reset did not send the Java liquor cancellation packet")
	}
	if len(drainMovementShowIDPackets(other.Session)) != 0 {
		t.Fatal("speed packets crossed the instance boundary")
	}
	HandleGMCommand(gm.Session, gm, ".speed 2", deps)
	drainMovementShowIDPackets(gm.Session)
	HandleGMCommand(gm.Session, gm, ".speed 1", deps)
	clearedBrave := false
	for _, payload := range drainMovementShowIDPackets(gm.Session) {
		if len(payload) >= 6 && payload[0] == packet.S_OPCODE_SKILLBRAVE && payload[5] == 0 {
			clearedBrave = true
		}
	}
	if !clearedBrave || gm.BraveSpeed != 0 {
		t.Fatal("switching to haste retained the client's brave stage")
	}
}
