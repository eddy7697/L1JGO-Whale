package world

import (
	"testing"
	"time"

	"github.com/l1jgo/server/internal/data"
)

func TestGMTrapResetRemovesRespawnQueueAndReloadKeepsManager(t *testing.T) {
	traps := &data.TrapData{Templates: map[int32]*data.TrapTemplate{1: {TrapID: 1}}, Spawns: []data.TrapSpawn{{TrapID: 1, X: 100, Y: 101, MapID: 4, Count: 1, Span: 1000}}}
	mgr := NewTrapManager(traps, nil)
	trap := mgr.GetTrapsAt(100, 101, 4)[0]
	id := trap.ID
	mgr.DisableTrap(trap)
	mgr.ResetAllTraps()
	mgr.ProcessRespawns(time.Now().Add(time.Hour))
	if got := mgr.GetTrapsAt(100, 101, 4); len(got) != 1 || got[0].ID != id {
		t.Fatalf("reset should reenable once, got %+v", got)
	}
	visible := mgr.GetVisibleTrapsInScreen(100, 101, 4)
	if len(visible) != 2 || !visible[1].Base || visible[1].ID == id {
		t.Fatal("GM must see separate trap base without triggering it")
	}
	mgr.Reload(traps)
	if got := mgr.GetTrapsAt(100, 101, 4); len(got) != 1 || got[0].ID == id {
		t.Fatal("reload should replace trap identities on same manager")
	}
}
