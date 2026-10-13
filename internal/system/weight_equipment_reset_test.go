package system

import (
	"math"
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/world"
)

func TestJavaWeightArmorDataReachesPlayerCapacity(t *testing.T) {
	items, err := data.LoadItemTable("../../data/yaml/weapon_list.yaml", "../../data/yaml/armor_list.yaml", "../../data/yaml/etcitem_list.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for id, percent := range map[int32]int{20317: 40, 20320: 50, 20321: 20, 21025: 2, 21026: 5, 21027: 10, 21069: 8, 21070: 8, 21071: 8, 21072: 8, 21073: 8, 21074: 5, 21075: 5, 21076: 5, 21077: 5, 21078: 5, 21079: 5, 21080: 5, 21095: 30, 120317: 40, 120320: 50, 120321: 20} {
		info := items.Get(id)
		if info == nil || info.WeightReduction != percent {
			t.Fatalf("Java armor %d weight reduction missing: %+v", id, info)
		}
	}
	p := &world.PlayerInfo{ClassType: 1, Str: 18, Con: 14, OriginalStr: 18, OriginalCon: 14, Inv: world.NewInventory()}
	base := world.PlayerMaxWeight(p)
	belt := p.Inv.AddItem(20320, 1, "belt", 0, 50000, false, 1)
	p.Equip.Set(world.SlotBelt, belt)
	applyEquipStats(p, items, nil, nil)
	if math.Abs(world.PlayerMaxWeight(p)-base*1.5) > 1e-9 {
		t.Fatal("equipping Java 50% belt did not increase capacity")
	}
	p.Equip.Set(world.SlotBelt, nil)
	applyEquipStats(p, items, nil, nil)
	if world.PlayerMaxWeight(p) != base {
		t.Fatal("unequipping belt retained capacity bonus")
	}
}

func TestCharacterResetStoresCreationWeightStatsBeforeElixirs(t *testing.T) {
	ws := world.NewState()
	p := addSkillTestPlayer(ws, &world.PlayerInfo{SessionID: 1, Session: newSkillTestSession(t, 1), CharID: 1001, ClassType: 2, OriginalStr: 16, OriginalCon: 15})
	deps := newSkillTestSystem(t, ws).deps
	s := NewCharResetSystem(deps)
	c := deps.Scripting.GetCharCreateData(int(p.ClassType))
	str, con := int16(c.BaseSTR+c.BonusAmount), int16(c.BaseCON)
	s.ResetStage1(p.Session, p, str, int16(c.BaseINT), int16(c.BaseWIS), int16(c.BaseDEX), con, int16(c.BaseCHA))
	if p.OriginalStr != str || p.OriginalCon != con {
		t.Fatal("reset did not save the selected creation stats")
	}
	s.ResetStage3(p.Session, p, str+1, p.Intel, p.Wis, p.Dex, con+1, p.Cha)
	if p.Str != str+1 || p.Con != con+1 || p.OriginalStr != str || p.OriginalCon != con {
		t.Fatal("elixir stage must not rewrite creation stats")
	}
}
