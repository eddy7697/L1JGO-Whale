package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

func gmWorldNpcTable(t *testing.T) *data.NpcTable {
	t.Helper()
	path := filepath.Join(t.TempDir(), "npcs.yaml")
	raw := []byte("npcs:\n  - npc_id: 45001\n    name: 測試獸\n    impl: L1Monster\n    gfx_id: 10\n    level: 10\n    hp: 100\n    mp: 25\n    ac: -5\n    atk_speed: 1000\n    passive_speed: 600\n")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	table, err := data.LoadNpcTable(path)
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func TestGMSummonUsesRealCompanionWithZeroPetCost(t *testing.T) {
	ws := world.NewState()
	sess := newSkillTestSession(t, 1)
	p := &world.PlayerInfo{Session: sess, SessionID: 1, CharID: 1, Name: "gm", X: 100, Y: 100, MapID: 4, ShowID: 9}
	ws.AddPlayer(p)
	deps := &handler.Deps{World: ws, Npcs: gmWorldNpcTable(t)}
	if err := NewSummonSystem(deps).GMSummon(p, 45001, 2); err != nil {
		t.Fatal(err)
	}
	summons := ws.GetSummonsByOwner(p.CharID)
	if len(summons) != 2 {
		t.Fatalf("summon count=%d", len(summons))
	}
	for _, s := range summons {
		if s.PetCost != 0 || s.TimerTicks != 18000 || s.HP != 100 || s.ShowID != 9 || s.Status != world.SummonAggressive {
			t.Fatalf("bad GM summon: %+v", s)
		}
	}
	if cost := NewPetSystem(deps).CalcUsedPetCost(p.CharID); cost != 0 {
		t.Fatalf("GM召喚不得佔用CHA，got %d", cost)
	}
}

func TestGMInsertFailureDoesNotSpawnAndRejectsUnknownType(t *testing.T) {
	store, err := data.LoadGMSpawnStore(nil, filepath.Join(t.TempDir(), "missing", "spawns.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	ws := world.NewState()
	deps := &handler.Deps{World: ws, Npcs: gmWorldNpcTable(t), GMSpawns: store}
	sys := NewGMCommandSystem(deps)
	p := &world.PlayerInfo{X: 100, Y: 100, MapID: 4}
	p.ShowID = 99
	if _, err := sys.InsertGMSpawn(p, "mob", 45001); err == nil {
		t.Fatal("instance must not create a permanent base-world spawn")
	}
	p.ShowID = 0
	if _, err := sys.InsertGMSpawn(p, "bogus", 45001); err == nil {
		t.Fatal("unknown type must fail")
	}
	if _, err := sys.InsertGMSpawn(p, "mob", 45001); err == nil {
		t.Fatal("failed persistence must fail command")
	}
	if ws.NpcCount() != 0 || len(store.Entries()) != 0 {
		t.Fatal("failed command must not mutate runtime")
	}
}

func TestGMDebugGfxUsesIndependentRuntimeValues(t *testing.T) {
	ws := world.NewState()
	table := gmWorldNpcTable(t)
	deps := &handler.Deps{World: ws, Npcs: table}
	if err := NewGMCommandSystem(deps).SpawnGMDebugGfx(&world.PlayerInfo{X: 100, Y: 100, MapID: 4, ShowID: 9}, 200, 2); err != nil {
		t.Fatal(err)
	}
	if ws.NpcCount() != 2 {
		t.Fatal("gfxid must create runtime NPCs")
	}
	seen := map[int32]bool{}
	for _, n := range ws.NpcList() {
		seen[n.GfxID] = true
		if n.ShowID != 9 || n.MoveSpeed != 0 || n.RespawnDelay != 0 {
			t.Fatalf("bad gfx NPC: %+v", n)
		}
	}
	if !seen[200] || !seen[201] || table.Get(45001).GfxID != 10 {
		t.Fatal("gfxid must not mutate shared template")
	}
}
