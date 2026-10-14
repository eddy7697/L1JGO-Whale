package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/world"
)

// Java DropTable.setDrop：機率 = chance × 全域倍率 × 地圖 drop_rate × 單品 drop_rate；
// 數量 = min/max × 單品 drop_amount。
func TestJavaGiveDropsAppliesMapAndItemRates(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mapRate  float64
		itemRate float64
		amount   float64
		want     int32
	}{
		{"all rates one", 1, 1, 1, 10},
		{"map rate zero blocks drop", 0, 1, 1, 0},
		{"item rate zero blocks drop", 1, 0, 1, 0},
		{"item amount scales count", 1, 1, 3, 30},
		{"half chance still drops with max chance", 0.5, 2, 1, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, killer, npc := newJavaLootTest(t)
			npc.NpcID = 45000
			npc.MapID = 900
			killer.MapID = 900
			dir := t.TempDir()
			dropPath := filepath.Join(dir, "drops.yaml")
			if err := os.WriteFile(dropPath, []byte("drops:\n  - mob_id: 45000\n    items:\n      - item_id: 40308\n        min: 10\n        max: 10\n        chance: 1000000\n"), 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			s.deps.Drops, err = data.LoadDropTable(dropPath)
			if err != nil {
				t.Fatal(err)
			}
			mapYAML := []byte("maps:\n  - map_id: 900\n    name: rate_test\n    start_x: 0\n    end_x: 10\n    start_y: 0\n    end_y: 10\n    drop_rate: " + ftoa(tc.mapRate) + "\n")
			mapPath := filepath.Join(dir, "map_list.yaml")
			if err := os.WriteFile(mapPath, mapYAML, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "900.txt"), []byte("15,15,15,15,15,15,15,15,15,15,15\n"), 0600); err != nil {
				t.Fatal(err)
			}
			s.deps.MapData, err = data.LoadMapData(mapPath, dir)
			if err != nil {
				t.Fatal(err)
			}
			s.deps.DropItems = data.NewDropItemTable([]data.DropItemRate{{ItemID: 40308, DropRate: tc.itemRate, DropAmount: tc.amount}})
			s.deps.Config = &config.Config{Rates: config.RatesConfig{DropRate: 1, GoldRate: 1}}
			npc.HateList = map[uint64]int32{killer.SessionID: 100}
			s.GiveDrops(killer, npc)
			got := int32(0)
			if item := killer.Inv.FindByItemID(world.AdenaItemID); item != nil {
				got = item.Count
			}
			if got != tc.want {
				t.Fatalf("quantity=%d want=%d", got, tc.want)
			}
		})
	}
}

// 查無地圖或未載入倍率表時不改變掉落（與 Java 查無地圖回 0 不同，見 JAVA_PARITY_AUDIT 1-18 備註）。
func TestJavaGiveDropsDefaultsWhenTablesMissing(t *testing.T) {
	s, killer, npc := newJavaLootTest(t)
	npc.NpcID = 45000
	path := filepath.Join(t.TempDir(), "drops.yaml")
	if err := os.WriteFile(path, []byte("drops:\n  - mob_id: 45000\n    items:\n      - item_id: 40308\n        min: 7\n        max: 7\n        chance: 1000000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var err error
	s.deps.Drops, err = data.LoadDropTable(path)
	if err != nil {
		t.Fatal(err)
	}
	s.deps.Config = &config.Config{Rates: config.RatesConfig{DropRate: 1, GoldRate: 1}}
	npc.HateList = map[uint64]int32{killer.SessionID: 100}
	s.GiveDrops(killer, npc)
	if item := killer.Inv.FindByItemID(world.AdenaItemID); item == nil || item.Count != 7 {
		t.Fatalf("missing tables should keep base drop: %+v", item)
	}
}

func ftoa(f float64) string {
	switch f {
	case 0:
		return "0"
	case 0.5:
		return "0.5"
	default:
		return "1"
	}
}
