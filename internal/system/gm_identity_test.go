package system

import (
	"github.com/l1jgo/server/internal/world"
	"testing"
)

func TestCancelMagicPreservesGMInvisible(t *testing.T) {
	ws := world.NewState()
	gm := addSkillTestPlayer(ws, &world.PlayerInfo{CharID: 1, SessionID: 1, Session: newSkillTestSession(t, 1), X: 100, Y: 100, MapID: 4, AccessLevel: 200, GMInvisible: true, Invisible: true})
	viewer := addSkillTestPlayer(ws, &world.PlayerInfo{CharID: 2, SessionID: 2, Session: newSkillTestSession(t, 2), X: 101, Y: 100, MapID: 4})
	gm.AddBuff(&world.ActiveBuff{SkillID: 60, TicksLeft: 100, SetInvisible: true})
	newSkillTestSystem(t, ws).GMClearAllStatuses(gm)
	if !gm.GMInvisible || gm.Invisible || gm.HasBuff(60) {
		t.Fatal("清除 buff 只應移除隱身術，保留獨立 GM 隱身")
	}
	if hasPutObjectPacket(drainSkillTestPackets(viewer.Session), gm.CharID) {
		t.Fatal("清除 buff 不應向旁人顯示 GM")
	}
}

func TestMonitorExcludedFromMonsterAggro(t *testing.T) {
	ws := world.NewState()
	monitor := addSkillTestPlayer(ws, &world.PlayerInfo{CharID: 1, SessionID: 1, Session: newSkillTestSession(t, 1), X: 100, Y: 100, MapID: 900, HP: 100, MaxHP: 100, AccessLevel: 100})
	npc := newNpcAITargetTestMonster(0)
	ws.AddNpc(npc)
	s := newNpcAILOSTestSystem(t, ws)
	if s.npcCanAcquireTargetLikeJava(npc, monitor) {
		t.Fatal("Java 監察員不應被怪物主動鎖定")
	}
	monitor.AccessLevel = 200
	monitor.SetGM(false)
	if !s.npcCanAcquireTargetLikeJava(npc, monitor) {
		t.Fatal("gm off 應允許主動怪物鎖定")
	}
}
