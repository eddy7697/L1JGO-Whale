package system

import (
	"testing"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

// 測試用物品（data/yaml）：
//   31    長劍（safe_enchant 6，非象牙塔）
//   7     象牙塔短劍（safe_enchant 6，象牙塔武器白名單）
//   65    天空之劍（safe_enchant -1，不可強化）
//   20028 修練者皮頭盔（safe_enchant 0，象牙塔防具白名單）
//   20282 修練者銀戒指（ring, grade 2 初等）
//   20252 蕾雅項鍊（amulet, grade 1 中等）
//   20250 變形怪首領項鍊（amulet, grade 0 上等）
//   20251 無 grade 的項鍊（grade -1）

func newEnchantScrollTestSystem(t *testing.T, ws *world.State) *ItemUseSystem {
	t.Helper()
	items, err := data.LoadItemTable("../../data/yaml/weapon_list.yaml", "../../data/yaml/armor_list.yaml", "../../data/yaml/etcitem_list.yaml")
	if err != nil {
		t.Fatalf("載入物品表失敗: %v", err)
	}
	deps := &handler.Deps{
		World:  ws,
		Items:  items,
		Config: &config.Config{Enchant: config.EnchantConfig{WeaponChance: 0.10, ArmorChance: 0.10, AttrChance: 0.10}},
		Log:    zap.NewNop(),
	}
	deps.Equip = NewEquipSystem(deps)
	return NewItemUseSystem(deps)
}

func newEnchantScrollTestPlayer(t *testing.T, ws *world.State) *world.PlayerInfo {
	t.Helper()
	return addSkillTestPlayer(ws, &world.PlayerInfo{
		SessionID: 1,
		Session:   newSkillTestSession(t, 1),
		CharID:    1001,
		Name:      "enchanter",
		MaxMP:     50,
		MP:        50,
	})
}

func addEnchantScrollTestItem(s *ItemUseSystem, player *world.PlayerInfo, objID, itemID, count int32) *world.InvItem {
	info := s.deps.Items.Get(itemID)
	it := player.Inv.AddItemWithID(objID, itemID, count, info.Name, info.InvGfx, info.Weight, info.Stackable, 1)
	it.Identified = true
	return it
}

func scrollTestUse(t *testing.T, s *ItemUseSystem, player *world.PlayerInfo, scroll, target *world.InvItem, roll int) bool {
	t.Helper()
	return s.EnchantSpecialScrollWithRoll(player.Session, player, scroll, s.deps.Items.Get(scroll.ItemID), target.ObjectID, roll)
}

// ---------- 武器屬性強化卷軸 41429-41432 ----------

func TestAttrScrollRejectsNonWeaponWithoutConsuming(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantWeaponWind, 2)
	helm := addEnchantScrollTestItem(s, player, 7002, 20028, 1)

	if scrollTestUse(t, s, player, scroll, helm, 1) {
		t.Fatal("非武器目標不得消耗卷軸")
	}
	if scroll.Count != 2 || helm.AttrEnchantLevel != 0 {
		t.Fatalf("非武器目標不應改變狀態 scroll=%d attr=%d", scroll.Count, helm.AttrEnchantLevel)
	}
	if !hasServerMessage(drainSkillTestPackets(player.Session), 79) {
		t.Fatal("非武器目標應送 79")
	}
}

func TestAttrScrollRejectsUnenchantableWeaponWith1453(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantWeaponFire, 1)
	sword := addEnchantScrollTestItem(s, player, 7002, 65, 1) // safe_enchant -1

	if scrollTestUse(t, s, player, scroll, sword, 1) {
		t.Fatal("safe_enchant<0 的武器不得消耗卷軸")
	}
	if !hasServerMessage(drainSkillTestPackets(player.Session), 1453) {
		t.Fatal("safe_enchant<0 應送 1453")
	}
}

func TestAttrScrollSuccessSetsKindLevelAndConsumes(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantWeaponWind, 2)
	sword := addEnchantScrollTestItem(s, player, 7002, 31, 1)

	// AttrChance 0.10 → 10 >= roll 10 成功
	if !scrollTestUse(t, s, player, scroll, sword, 10) {
		t.Fatal("roll=10 應成功並消耗卷軸")
	}
	if sword.AttrEnchantKind != world.AttrKindWind || sword.AttrEnchantLevel != 1 {
		t.Fatalf("成功後應為風屬性 1 級 kind=%d level=%d", sword.AttrEnchantKind, sword.AttrEnchantLevel)
	}
	if scroll.Count != 1 {
		t.Fatalf("成功後應消耗 1 張卷軸 got=%d", scroll.Count)
	}
	if !hasServerMessage(drainSkillTestPackets(player.Session), 1410) {
		t.Fatal("成功應送 1410")
	}
	if name := handler.BuildViewName(sword, s.deps.Items.Get(31)); len(name) < 6 || name[:6] != "$6121 " {
		t.Fatalf("成功後名稱應帶風 1 級前綴 $6121，got=%q", name)
	}
}

func TestAttrScrollFailureConsumesWithoutChange(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantWeaponEarth, 1)
	sword := addEnchantScrollTestItem(s, player, 7002, 31, 1)

	if !scrollTestUse(t, s, player, scroll, sword, 11) {
		t.Fatal("roll=11 失敗仍應消耗卷軸")
	}
	if sword.AttrEnchantKind != 0 || sword.AttrEnchantLevel != 0 {
		t.Fatalf("失敗不應改變屬性 kind=%d level=%d", sword.AttrEnchantKind, sword.AttrEnchantLevel)
	}
	if player.Inv.FindByObjectID(scroll.ObjectID) != nil {
		t.Fatal("最後一張卷軸失敗後應從背包移除")
	}
	if !hasServerMessage(drainSkillTestPackets(player.Session), 1411) {
		t.Fatal("失敗應送 1411")
	}
}

func TestAttrScrollSameAttrIncrementsAndCapsAtThree(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantWeaponFire, 5)
	sword := addEnchantScrollTestItem(s, player, 7002, 31, 1)
	sword.AttrEnchantKind = world.AttrKindFire
	sword.AttrEnchantLevel = 2

	if !scrollTestUse(t, s, player, scroll, sword, 1) {
		t.Fatal("同屬性 +2 → +3 應成功")
	}
	if sword.AttrEnchantLevel != 3 || sword.AttrEnchantKind != world.AttrKindFire {
		t.Fatalf("同屬性應 +1 → 3 級，got kind=%d level=%d", sword.AttrEnchantKind, sword.AttrEnchantLevel)
	}
	drainSkillTestPackets(player.Session)

	if scrollTestUse(t, s, player, scroll, sword, 1) {
		t.Fatal("同屬性已達 3 級不得消耗卷軸")
	}
	if sword.AttrEnchantLevel != 3 || scroll.Count != 4 {
		t.Fatalf("上限時不應改變 level=%d scroll=%d", sword.AttrEnchantLevel, scroll.Count)
	}
	if !hasServerMessage(drainSkillTestPackets(player.Session), 1453) {
		t.Fatal("上限應送 1453")
	}
}

func TestAttrScrollDifferentAttrResetsToLevelOne(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantWeaponWater, 1)
	sword := addEnchantScrollTestItem(s, player, 7002, 31, 1)
	sword.AttrEnchantKind = world.AttrKindFire
	sword.AttrEnchantLevel = 3

	if !scrollTestUse(t, s, player, scroll, sword, 5) {
		t.Fatal("不同屬性在 3 級仍可使用")
	}
	if sword.AttrEnchantKind != world.AttrKindWater || sword.AttrEnchantLevel != 1 {
		t.Fatalf("不同屬性應改為水 1 級 kind=%d level=%d", sword.AttrEnchantKind, sword.AttrEnchantLevel)
	}
}

func TestAttrScrollRejectsSealedWeapon(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantWeaponWater, 1)
	sword := addEnchantScrollTestItem(s, player, 7002, 31, 1)
	sword.Bless = 129

	if scrollTestUse(t, s, player, scroll, sword, 1) {
		t.Fatal("封印武器不得消耗卷軸")
	}
	if !hasServerMessage(drainSkillTestPackets(player.Session), 79) {
		t.Fatal("封印武器應送 79")
	}
}

// ---------- 象牙塔卷軸 49311 / 49312 ----------

func TestIvoryTowerWeaponScrollRejectsNonIvoryWeapon(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantWeaponIvoryTower, 1)
	sword := addEnchantScrollTestItem(s, player, 7002, 31, 1)

	if scrollTestUse(t, s, player, scroll, sword, 1) {
		t.Fatal("非象牙塔武器不得消耗卷軸")
	}
	if sword.EnchantLvl != 0 || scroll.Count != 1 {
		t.Fatalf("非象牙塔武器不應改變 enchant=%d scroll=%d", sword.EnchantLvl, scroll.Count)
	}
	if !hasServerMessage(drainSkillTestPackets(player.Session), 79) {
		t.Fatal("非象牙塔武器應送 79")
	}
}

func TestIvoryTowerWeaponScrollAlwaysSucceedsBelowSafeEnchant(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantWeaponIvoryTower, 2)
	dagger := addEnchantScrollTestItem(s, player, 7002, 7, 1) // safe_enchant 6
	dagger.EnchantLvl = 5

	if !scrollTestUse(t, s, player, scroll, dagger, 100) {
		t.Fatal("低於安定值應必定成功")
	}
	if dagger.EnchantLvl != 6 || scroll.Count != 1 {
		t.Fatalf("成功後應 +1 並消耗卷軸 enchant=%d scroll=%d", dagger.EnchantLvl, scroll.Count)
	}
	if !hasServerMessage(drainSkillTestPackets(player.Session), 161) {
		t.Fatal("成功應送 161")
	}

	// 已達安定值 → 79 且不消耗
	if scrollTestUse(t, s, player, scroll, dagger, 1) {
		t.Fatal("達安定值不得消耗卷軸")
	}
	if dagger.EnchantLvl != 6 || scroll.Count != 1 {
		t.Fatalf("達安定值不應改變 enchant=%d scroll=%d", dagger.EnchantLvl, scroll.Count)
	}
	if !hasServerMessage(drainSkillTestPackets(player.Session), 79) {
		t.Fatal("達安定值應送 79")
	}
}

func TestIvoryTowerWeaponScrollRejectsArmorTarget(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantWeaponIvoryTower, 1)
	helm := addEnchantScrollTestItem(s, player, 7002, 20028, 1)
	helm.EnchantLvl = -1

	if scrollTestUse(t, s, player, scroll, helm, 1) {
		t.Fatal("武器象牙塔卷軸不得用於防具")
	}
}

func TestIvoryTowerArmorScrollFollowsSafeEnchantAndWhitelist(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantArmorIvoryTower, 3)
	helm := addEnchantScrollTestItem(s, player, 7002, 20028, 1) // safe_enchant 0
	ring := addEnchantScrollTestItem(s, player, 7003, 20282, 1) // 非象牙塔防具白名單

	// safe_enchant 0 且 +0：不低於安定值 → 79
	if scrollTestUse(t, s, player, scroll, helm, 1) {
		t.Fatal("+0 >= safe 0 不得成功")
	}
	drainSkillTestPackets(player.Session)
	// -1 < 0 → 成功 +1
	helm.EnchantLvl = -1
	if !scrollTestUse(t, s, player, scroll, helm, 100) {
		t.Fatal("低於安定值應成功")
	}
	if helm.EnchantLvl != 0 || scroll.Count != 2 {
		t.Fatalf("成功後 enchant=%d scroll=%d", helm.EnchantLvl, scroll.Count)
	}
	// 白名單外
	ring.EnchantLvl = -1
	if scrollTestUse(t, s, player, scroll, ring, 100) {
		t.Fatal("白名單外防具不得成功")
	}
	if scroll.Count != 2 {
		t.Fatalf("白名單外不應消耗 scroll=%d", scroll.Count)
	}
}

// ---------- 飾品強化卷軸 49148 ----------

func TestAccessoryEnchantChanceMatchesJava(t *testing.T) {
	// 門檻值（roll < 門檻才成功，實際機率為門檻 -1%）
	want := map[int]int{0: 85, 1: 60, 2: 52, 3: 48, 4: 45, 5: 44, 6: 43, 7: 42, 8: 41, 9: 40}
	for lvl, chance := range want {
		if got := accessoryEnchantChance(lvl); got != chance {
			t.Errorf("+%d 成功率 got=%d want=%d", lvl, got, chance)
		}
	}
}

func TestAccessoryScrollRejectsInvalidTargets(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantAccessory, 1)
	helm := addEnchantScrollTestItem(s, player, 7002, 20028, 1)
	noGrade := addEnchantScrollTestItem(s, player, 7003, 20251, 1)
	sword := addEnchantScrollTestItem(s, player, 7004, 31, 1)
	maxed := addEnchantScrollTestItem(s, player, 7005, 20282, 1)
	maxed.EnchantLvl = 10
	sealed := addEnchantScrollTestItem(s, player, 7006, 20282, 1)
	sealed.Bless = 128

	for name, target := range map[string]*world.InvItem{"非飾品防具": helm, "grade -1": noGrade, "武器": sword, "+10": maxed, "封印": sealed} {
		if scrollTestUse(t, s, player, scroll, target, 1) {
			t.Fatalf("%s 不得消耗卷軸", name)
		}
		if !hasServerMessage(drainSkillTestPackets(player.Session), 79) {
			t.Fatalf("%s 應送 79", name)
		}
	}
	if scroll.Count != 1 {
		t.Fatalf("無效目標不應消耗 scroll=%d", scroll.Count)
	}
}

func TestAccessoryScrollSuccessGradeTwoAddsMPAndAwardsSPAtSix(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantAccessory, 3)
	ring := addEnchantScrollTestItem(s, player, 7002, 20282, 1) // grade 2 初等

	if !scrollTestUse(t, s, player, scroll, ring, 1) {
		t.Fatal("roll=1 應成功")
	}
	if ring.EnchantLvl != 1 || ring.Accessory.AddMP != 1 || ring.Accessory.AddSP != 0 {
		t.Fatalf("初等 +1 應 MP+1 無 SP，got enchant=%d acc=%+v", ring.EnchantLvl, ring.Accessory)
	}
	if scroll.Count != 2 {
		t.Fatalf("成功應消耗 1 張 scroll=%d", scroll.Count)
	}
	pkts := drainSkillTestPackets(player.Session)
	if !hasServerMessage(pkts, 161) {
		t.Fatal("成功應送 161")
	}

	ring.EnchantLvl = 5
	if !scrollTestUse(t, s, player, scroll, ring, 1) {
		t.Fatal("+5 → +6 應成功")
	}
	if ring.EnchantLvl != 6 || ring.Accessory.AddMP != 2 || ring.Accessory.AddSP != 1 {
		t.Fatalf("+6 獎勵應 SP+1，got enchant=%d acc=%+v", ring.EnchantLvl, ring.Accessory)
	}
}

func TestAccessoryScrollGradeOneAndZeroBonuses(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantAccessory, 4)
	mid := addEnchantScrollTestItem(s, player, 7002, 20252, 1)  // grade 1 中等
	high := addEnchantScrollTestItem(s, player, 7003, 20250, 1) // grade 0 上等
	mid.EnchantLvl = 5
	high.EnchantLvl = 5

	if !scrollTestUse(t, s, player, scroll, mid, 1) {
		t.Fatal("中等 +5 應成功")
	}
	if mid.Accessory.AddHP != 2 || mid.Accessory.MDef != 1 {
		t.Fatalf("中等 +6 應 HP+2 MR+1，got %+v", mid.Accessory)
	}
	if !scrollTestUse(t, s, player, scroll, high, 1) {
		t.Fatal("上等 +5 應成功")
	}
	acc := high.Accessory
	if acc.EarthMr != 1 || acc.FireMr != 1 || acc.WaterMr != 1 || acc.WindMr != 1 || acc.HPR != 1 || acc.MPR != 1 {
		t.Fatalf("上等 +6 應四屬性+1 且 HPR/MPR+1，got %+v", acc)
	}
}

func TestAccessoryScrollFailureDestroysAccessoryAndConsumesScroll(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantAccessory, 2)
	ring := addEnchantScrollTestItem(s, player, 7002, 20282, 1)

	// +0 門檻 85：roll 85 → 失敗、roll 84 才成功（Java rnd < chance，實際 84%）
	if !scrollTestUse(t, s, player, scroll, ring, 85) {
		t.Fatal("失敗仍應消耗卷軸")
	}
	if player.Inv.FindByObjectID(ring.ObjectID) != nil {
		t.Fatal("失敗後飾品應消失")
	}
	if scroll.Count != 1 {
		t.Fatalf("失敗應消耗卷軸 scroll=%d", scroll.Count)
	}
	if !hasServerMessage(drainSkillTestPackets(player.Session), 164) {
		t.Fatal("失敗應送 164")
	}
}

func TestAccessoryScrollEquippedSuccessUpdatesPlayerStatsImmediately(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantAccessory, 1)
	ring := addEnchantScrollTestItem(s, player, 7002, 20282, 1)
	ring.Equipped = true
	player.Equip.Set(world.SlotRing1, ring)
	s.deps.Equip.(*EquipSystem).InitEquipStats(player)
	baseMaxMP := player.MaxMP

	if !scrollTestUse(t, s, player, scroll, ring, 1) {
		t.Fatal("裝備中成功")
	}
	if player.MaxMP != baseMaxMP+1 {
		t.Fatalf("裝備中成功應立即 MaxMP+1，got=%d want=%d", player.MaxMP, baseMaxMP+1)
	}
	if player.EquipBonuses.AddMP != 1 {
		t.Fatalf("EquipBonuses.AddMP 應為 1，got=%d", player.EquipBonuses.AddMP)
	}
}

func TestAccessoryScrollEquippedFailureUnequipsAndRemoves(t *testing.T) {
	ws := world.NewState()
	player := newEnchantScrollTestPlayer(t, ws)
	s := newEnchantScrollTestSystem(t, ws)
	scroll := addEnchantScrollTestItem(s, player, 7001, world.ScrollEnchantAccessory, 1)
	ring := addEnchantScrollTestItem(s, player, 7002, 20282, 1)
	ring.Accessory.AddMP = 3
	ring.Equipped = true
	player.Equip.Set(world.SlotRing1, ring)
	s.deps.Equip.(*EquipSystem).InitEquipStats(player)
	if player.EquipBonuses.AddMP != 3 {
		t.Fatalf("前置：裝備中飾品加成應套用 AddMP=%d", player.EquipBonuses.AddMP)
	}

	if !scrollTestUse(t, s, player, scroll, ring, 100) {
		t.Fatal("失敗仍應消耗卷軸")
	}
	if player.Equip.Get(world.SlotRing1) != nil {
		t.Fatal("失敗後應解除裝備")
	}
	if player.Inv.FindByObjectID(ring.ObjectID) != nil {
		t.Fatal("失敗後飾品應消失")
	}
	if player.EquipBonuses.AddMP != 0 {
		t.Fatalf("失敗後飾品加成應移除 AddMP=%d", player.EquipBonuses.AddMP)
	}
}

// ---------- 存檔往返（persist row ↔ InvItem 欄位） ----------

func TestAccessoryBonusSurvivesWarehouseRoundTrip(t *testing.T) {
	src := &world.InvItem{ObjectID: 1, ItemID: 20282, Count: 1, Accessory: world.AccessoryBonus{AddMP: 2, AddSP: 1}}
	row := warehouseItemFromInvItem("acct", "char", 3, src, 1)
	if row.Accessory != src.Accessory {
		t.Fatalf("倉庫列應保留飾品加成 got=%+v", row.Accessory)
	}
	cache := warehouseCacheFromPersistItem(row, nil)
	dst := &world.InvItem{}
	copyWarehouseCacheState(dst, cache)
	if dst.Accessory != src.Accessory {
		t.Fatalf("倉庫取回應保留飾品加成 got=%+v", dst.Accessory)
	}
}

// ---------- 戰鬥：屬性強化附加傷害 ----------

func TestWeaponAttrEnchantDamageMatchesJavaFormula(t *testing.T) {
	cases := []struct {
		level  int8
		resist int
		want   int
	}{
		{1, 0, 1}, {2, 0, 3}, {3, 0, 5}, {0, 0, 0}, {4, 0, 0},
		{3, -50, 7},   // weak_attr：16/32=0.5 → 5*1.5=7.5 → 7
		{2, 20, 2},    // int(6.4)=6 → 1-6/32=0.8125 → 3*0.8125=2.43 → 2
		{3, 100, 0},   // 32/32 → 係數 0
		{3, -100, 10}, // -32 → 係數 2
	}
	for _, c := range cases {
		if got := world.WeaponAttrEnchantDamage(c.level, c.resist); got != c.want {
			t.Errorf("level=%d resist=%d got=%d want=%d", c.level, c.resist, got, c.want)
		}
	}
}

func TestWeaponAttrEnchantDamageVsNpcUsesWeakAttr(t *testing.T) {
	player := &world.PlayerInfo{}
	sword := &world.InvItem{ItemID: 31, AttrEnchantKind: world.AttrKindFire, AttrEnchantLevel: 3}
	player.Equip.Set(world.SlotWeapon, sword)

	if got := weaponAttrEnchantDamageVsNpc(player, &world.NpcInfo{WeakAttr: 2}); got != 7 {
		t.Fatalf("弱火 NPC 應 7，got=%d", got)
	}
	if got := weaponAttrEnchantDamageVsNpc(player, &world.NpcInfo{WeakAttr: 4}); got != 5 {
		t.Fatalf("非弱火 NPC 應 5，got=%d", got)
	}
	if got := weaponAttrEnchantDamageVsNpc(player, &world.NpcInfo{WeakAttr: 6}); got != 5 {
		t.Fatalf("Java 用等值比較而非位元遮罩，weak_attr=6 應 5，got=%d", got)
	}
	if got := weaponAttrEnchantDamageVsNpc(&world.PlayerInfo{}, &world.NpcInfo{WeakAttr: 2}); got != 0 {
		t.Fatalf("空手應 0，got=%d", got)
	}
}

func TestWeaponAttrEnchantDamageVsPcUsesTargetResist(t *testing.T) {
	attacker := &world.PlayerInfo{}
	attacker.Equip.Set(world.SlotWeapon, &world.InvItem{ItemID: 31, AttrEnchantKind: world.AttrKindWater, AttrEnchantLevel: 2})
	target := &world.PlayerInfo{WaterRes: 20, FireRes: 100}

	if got := weaponAttrEnchantDamageVsPc(attacker, target); got != 2 {
		t.Fatalf("水抗 20 應 2，got=%d", got)
	}
	target.WaterRes = 100
	if got := weaponAttrEnchantDamageVsPc(attacker, target); got != 0 {
		t.Fatalf("水抗 100 應 0，got=%d", got)
	}
}

func TestPvPMeleeContextIncludesAttrEnchantDamage(t *testing.T) {
	attacker := &world.PlayerInfo{DmgMod: 4}
	attacker.Equip.Set(world.SlotWeapon, &world.InvItem{ItemID: 31, AttrEnchantKind: world.AttrKindEarth, AttrEnchantLevel: 3})
	target := &world.PlayerInfo{}
	ctx := buildPvPMeleeCombatContext(attacker, target, 10)
	if ctx.AttackerDmgMod != 9 {
		t.Fatalf("PvP 近戰 dmg_mod 應含屬性傷害 4+5=9，got=%d", ctx.AttackerDmgMod)
	}
	rctx := buildPvPRangedCombatContext(attacker, target, 10, 2)
	if rctx.AttackerBowDmgMod != 5 {
		t.Fatalf("PvP 遠程 bow_dmg_mod 應含屬性傷害 5，got=%d", rctx.AttackerBowDmgMod)
	}
}
