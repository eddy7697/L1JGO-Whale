package data

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGMCommandConfigContainsJavaRoomsAndItemSet(t *testing.T) {
	cfg, err := LoadGMCommandConfig("../../data/xml/GmCommands/GMCommands.xml")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rooms) != 84 {
		t.Fatalf("Java rooms=%d", len(cfg.Rooms))
	}
	room := cfg.Rooms["gmroom"]
	if room.MapID != 99 || room.X != 32737 || room.Y != 32796 {
		t.Fatalf("GMRoom: %+v", room)
	}
	set := cfg.ItemSets["DKSet"]
	if len(set) != 5 || set[0].ID != 58 || set[0].Enchant != 10 {
		t.Fatalf("DKSet: %+v", set)
	}
}

func TestGMSpawnStorePersistsAppendAndPreservesJavaNPCPrecedence(t *testing.T) {
	base := []SpawnEntry{{ID: 7, Source: "mob", NpcID: 1}, {ID: 7, Source: "npc", NpcID: 2}, {NpcID: 3}}
	path := filepath.Join(t.TempDir(), "gm_spawn_list.yaml")
	store, err := LoadGMSpawnStore(base, path)
	if err != nil {
		t.Fatal(err)
	}
	e, ok, legacy := store.Lookup(7)
	if !ok || legacy || e.NpcID != 2 {
		t.Fatalf("npc table must win: %+v %t %t", e, ok, legacy)
	}
	e, ok, legacy = store.Lookup(3)
	if !ok || !legacy || e.NpcID != 3 {
		t.Fatalf("legacy index fallback: %+v %t %t", e, ok, legacy)
	}
	first, err := store.Insert(SpawnEntry{Source: "mob", NpcID: 4, Count: 1, X: 100, Y: 101, MapID: 4})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Insert(SpawnEntry{Source: "npc", NpcID: 5, Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != 8 || second.ID != 9 {
		t.Fatalf("IDs %d, %d", first.ID, second.ID)
	}
	reloaded, err := LoadGMSpawnStore(base, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Entries()) != 5 {
		t.Fatal("persisted spawns lost across restart")
	}
	e, ok, legacy = reloaded.Lookup(first.ID)
	if !ok || legacy || e.X != 100 || e.Y != 101 {
		t.Fatalf("roundtrip: %+v", e)
	}
}

func TestGMSpawnStoreFailureDoesNotChangeMemory(t *testing.T) {
	store, err := LoadGMSpawnStore(nil, filepath.Join(t.TempDir(), "missing", "file.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Insert(SpawnEntry{Source: "mob", NpcID: 1, Count: 1}); err == nil {
		t.Fatal("must report failed persistence")
	}
	if len(store.Entries()) != 0 {
		t.Fatal("failed write must not create runtime spawn entry")
	}
	path := filepath.Join(t.TempDir(), "broken.yaml")
	if err := os.WriteFile(path, []byte("spawns: ["), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadGMSpawnStore(nil, path); err == nil {
		t.Fatal("invalid existing data must not be ignored")
	}
}

func TestGMTrapReloadValidatesBeforeReplacingLiveState(t *testing.T) {
	data, err := LoadTrapData("../../data/yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := data.ValidateGMReload(); err != nil {
		t.Fatalf("shipped trap data must reload: %v", err)
	}
	if len(data.Spawns) == 0 {
		t.Fatal("expected shipped trap spawns")
	}
	data.Spawns[0].RndX = -1
	if err := data.ValidateGMReload(); err == nil {
		t.Fatal("invalid random range must be rejected before live replacement")
	}
}
