package handler

import (
	"bytes"
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/world"
)

func loadEnchantScrollItemTable(t *testing.T) *data.ItemTable {
	t.Helper()
	items, err := data.LoadItemTable("../../data/yaml/weapon_list.yaml", "../../data/yaml/armor_list.yaml", "../../data/yaml/etcitem_list.yaml")
	if err != nil {
		t.Fatalf("載入物品表失敗: %v", err)
	}
	return items
}

func TestBuildViewNameAddsAttrEnchantPrefixForIdentifiedWeapon(t *testing.T) {
	items := loadEnchantScrollItemTable(t)
	info := items.Get(31)
	sword := &world.InvItem{ItemID: 31, Name: info.Name, Count: 1, Identified: true, EnchantLvl: 7, AttrEnchantKind: world.AttrKindWind, AttrEnchantLevel: 2}

	if got, want := buildViewName(sword, info), "$6122 +7 "+info.Name; got != want {
		t.Fatalf("已鑑定風 2 級名稱 got=%q want=%q", got, want)
	}
	sword.Identified = false
	if got := buildViewName(sword, info); bytes.Contains([]byte(got), []byte("$6122")) {
		t.Fatalf("未鑑定不應顯示屬性前綴 got=%q", got)
	}
	sword.Identified = true
	sword.AttrEnchantLevel = 0
	if got := buildViewName(sword, info); bytes.Contains([]byte(got), []byte("$61")) {
		t.Fatalf("無屬性不應顯示前綴 got=%q", got)
	}
}

func TestAttrEnchantNamePrefixTable(t *testing.T) {
	cases := map[[2]int8]string{
		{world.AttrKindEarth, 1}: "$6124", {world.AttrKindEarth, 3}: "$6126",
		{world.AttrKindFire, 1}: "$6115", {world.AttrKindFire, 2}: "$6116",
		{world.AttrKindWater, 3}: "$6120", {world.AttrKindWind, 1}: "$6121",
		{world.AttrKindWind, 4}: "", {world.AttrKindNone, 1}: "",
	}
	for k, want := range cases {
		if got := world.AttrEnchantNamePrefix(k[0], k[1]); got != want {
			t.Errorf("kind=%d level=%d got=%q want=%q", k[0], k[1], got, want)
		}
	}
}

func TestStatusBytesIncludeAccessoryEnchantBonuses(t *testing.T) {
	items := loadEnchantScrollItemTable(t)
	info := items.Get(20282) // 修練者銀戒指（grade 2）
	ring := &world.InvItem{ItemID: 20282, Name: info.Name, Count: 1, Identified: true}
	base := buildStatusBytes(ring, info)

	ring.Accessory = world.AccessoryBonus{AddMP: 2, AddSP: 1, AddHP: 4, HPR: 1, MPR: 1, MDef: 1, FireMr: 3, WaterMr: 3, WindMr: 3, EarthMr: 3}
	got := buildStatusBytes(ring, info)
	if bytes.Equal(base, got) {
		t.Fatal("飾品加成應改變 status bytes")
	}
	suffix := got[len(got)-len(got):] // 完整緩衝
	for _, tlv := range [][]byte{
		{14, byte(info.AddHP + 4), 0}, // HP uint16
		{32, byte(info.AddMP + 2)},
		{17, byte(info.AddSP + 1)},
		{37, byte(info.AddHPR + 1)},
		{38, byte(info.AddMPR + 1)},
		{15, byte(info.MDef + 1), 0}, // MR uint16（Java getMr 含飾品 m_def）
		{27, byte(info.DefFire + 3)},
		{28, byte(info.DefWater + 3)},
		{29, byte(info.DefWind + 3)},
		{30, byte(info.DefEarth + 3)},
	} {
		if !bytes.Contains(suffix, tlv) {
			t.Errorf("status bytes 缺少 TLV %v: %v", tlv, got)
		}
	}
}

func TestArmorStatusHeaderSendsJavaGradeByte(t *testing.T) {
	items := loadEnchantScrollItemTable(t)
	ring := &world.InvItem{ItemID: 20282, Count: 1, Identified: true}
	if got := buildStatusBytes(ring, items.Get(20282)); len(got) < 4 || got[0] != 19 || got[3] != 2 {
		t.Fatalf("grade 2 飾品 header 第 4 位元組應為 2，got=%v", got[:4])
	}
	helm := &world.InvItem{ItemID: 20028, Count: 1, Identified: true}
	if got := buildStatusBytes(helm, items.Get(20028)); len(got) < 4 || got[3] != 0xFF {
		t.Fatalf("非飾品 header grade 應為 -1(0xFF)，got=%v", got[:4])
	}
}

func TestArmorGradeLoadedFromYAML(t *testing.T) {
	items := loadEnchantScrollItemTable(t)
	cases := map[int32]int{20282: 2, 20252: 1, 20250: 0, 20251: -1, 20028: -1, 31: -1, 49148: -1}
	for id, want := range cases {
		info := items.Get(id)
		if info == nil {
			t.Fatalf("物品 %d 不存在", id)
		}
		if info.Grade != want {
			t.Errorf("物品 %d grade got=%d want=%d", id, info.Grade, want)
		}
	}
}
