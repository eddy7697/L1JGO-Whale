package data

import "testing"

func TestAdvancedPolymorphMapAndSeedAreCompleteWithoutReplacingBookConflict(t *testing.T) {
	items, err := LoadItemTable("../../data/yaml/weapon_list.yaml", "../../data/yaml/armor_list.yaml", "../../data/yaml/etcitem_list.yaml")
	if err != nil {
		t.Fatal(err)
	}
	polys, err := LoadPolymorphTable("../../data/yaml/polymorph_list.yaml")
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[int32]bool)
	for cardID := int32(600101); cardID <= 600150; cardID++ {
		polyID, ok := AdvancedPolymorphID(cardID)
		if !ok || seen[polyID] {
			t.Fatalf("卡片 %d 遺漏或重複變身 %d", cardID, polyID)
		}
		seen[polyID] = true
		if reverse, ok := AdvancedPolymorphCard(polyID); !ok || reverse != cardID {
			t.Fatalf("card/poly 反查不一致: %d/%d", cardID, polyID)
		}
		item := items.Get(cardID)
		if item == nil || item.Category != CategoryEtcItem || item.Stackable || item.UseType != "normal" || item.Material != "paper" || item.Bless != 1 {
			t.Fatalf("卡片資料未對齊 Java: %+v", item)
		}
		poly := polys.GetByID(polyID)
		if poly == nil || poly.MinLevel != 1 || poly.WeaponEquip != 1 || poly.ArmorEquip != 1 || !poly.CanUseSkill || poly.Cause != 1 {
			t.Fatalf("變身資料未對齊 Java: %+v", poly)
		}
	}
	if len(seen) != 50 {
		t.Fatalf("變身清單應有 50 種，got %d", len(seen))
	}
	if item := items.Get(600100); item == nil || item.Category != CategoryArmor || item.Name != "潘朵拉神聖涼鞋" {
		t.Fatalf("600100 原有涼鞋遭覆蓋: %+v", item)
	}
	for _, cardID := range []int32{600100, 600151, -1} {
		if _, ok := AdvancedPolymorphID(cardID); ok {
			t.Fatalf("未定義卡片 %d 不應被接受", cardID)
		}
	}
	if card, ok := AdvancedPolymorphCard(13715); !ok || card != 600123 {
		t.Fatal("王子對應必須為 600123/13715")
	}
}
