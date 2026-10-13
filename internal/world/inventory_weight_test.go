package world

import (
	"math"
	"testing"
)

// Java L1ItemInstance.getWeight() line 291-296: max(count * templateWeight / 1000, 1), 0 when template weight is 0.
func TestStackWeightLikeJavaItemInstance(t *testing.T) {
	tests := []struct {
		name       string
		unitWeight int32
		count      int32
		want       int32
	}{
		{"zero template weight stays 0", 0, 1000, 0},
		{"dagger 10000 x1 = 10", 10000, 1, 10},
		{"sub-unit stack floors to 1", 1, 1, 1},
		{"999 units of weight 1 still 1", 1, 999, 1},
		{"1000 units of weight 1 = 1", 1, 1000, 1},
		{"2500 units of weight 1000 = 2500", 1000, 2500, 2500},
		// 500,000 × 5000 = 2.5e9 overflows int32; must not wrap to the 1-unit floor.
		{"large stack does not wrap int32", 5000, 500_000, 2_500_000},
		{"int32 max count x 10000 clamps", 10000, math.MaxInt32, math.MaxInt32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StackWeight(tt.unitWeight, tt.count); got != tt.want {
				t.Fatalf("StackWeight(%d, %d) = %d, want %d", tt.unitWeight, tt.count, got, tt.want)
			}
		})
	}
}

// Java L1Inventory.getWeight() line 49-57 sums per-item getWeight(); a huge stack must count its real weight.
func TestTotalWeightSumsStacksWithoutOverflow(t *testing.T) {
	inv := NewInventory()
	inv.AddItem(1, 1, "dagger", 0, 10000, false, 0)           // 10
	inv.AddItem(40308, 2_000_000_000, "adena", 0, 0, true, 0) // 0
	inv.AddItem(40010, 500_000, "potion", 0, 5000, true, 0)   // 2_500_000 (wraps if computed in int32)
	if got := inv.TotalWeight(); got != 2_500_010 {
		t.Fatalf("TotalWeight = %d, want 2500010", got)
	}
}

// Java L1PcInstance.getMaxWeight() line 2939: 150 * floor(0.6*STR + 0.4*CON + 1).
func TestMaxWeightFormula(t *testing.T) {
	if got := MaxWeight(18, 18); got != 2850 {
		t.Fatalf("MaxWeight(18,18) = %d, want 2850", got)
	}
	if got := MaxWeight(20, 20); got != 3150 {
		t.Fatalf("MaxWeight(20,20) = %d, want 3150", got)
	}
	if got := MaxWeight(18, 14); got != 2550 {
		t.Fatalf("MaxWeight(18,14) = %d, want 2550", got)
	}
}

// Java L1PcInstance.getMaxWeight() lines 2936-2965:
// max = base * (1 + armor% + doll% + 0.04*(origStrWR+origConWR)); then +180 for DECREASE_WEIGHT; then * RATE_WEIGHT_LIMIT.
func TestPlayerMaxWeightLikeJavaGetMaxWeight(t *testing.T) {
	// Knight STR 18 / CON 14 earns no original-stat bonus (STR table is 0 for knights, CON needs >= 15).
	newKnight := func() *PlayerInfo { return &PlayerInfo{ClassType: classKnight, Str: 18, Con: 14} }

	p := newKnight()
	if got := PlayerMaxWeight(p); math.Abs(got-2550) > 1e-9 {
		t.Fatalf("plain knight = %g, want 2550", got)
	}

	p = newKnight()
	p.AddBuff(&ActiveBuff{SkillID: 14, TicksLeft: 10}) // DECREASE_WEIGHT: flat +180 after the multiplier
	if got := PlayerMaxWeight(p); math.Abs(got-2730) > 1e-9 {
		t.Fatalf("decrease weight = %g, want 2730", got)
	}

	p = newKnight()
	p.WeightBonus = 20 // doll 20%
	if got := PlayerMaxWeight(p); math.Abs(got-3060) > 1e-9 {
		t.Fatalf("doll 20%% = %g, want 3060", got)
	}

	p = newKnight()
	p.WeightBonus = 20
	p.EquipBonuses.WeightReduction = 10 // armor 10%
	p.AddBuff(&ActiveBuff{SkillID: 14, TicksLeft: 10})
	if got := PlayerMaxWeight(p); math.Abs(got-3495) > 1e-9 { // 2550*1.3 + 180
		t.Fatalf("doll+armor+magic = %g, want 3495", got)
	}

	// Prince STR 20 / CON 18: original STR 20 → 3, original CON >= 11 → 1 → +16%.
	p = &PlayerInfo{ClassType: classPrince, Str: 20, Con: 18}
	if got := PlayerMaxWeight(p); math.Abs(got-3480) > 1e-9 {
		t.Fatalf("prince original-stat bonus = %g, want 3480", got)
	}

	// Config.RATE_WEIGHT_LIMIT multiplies the final value; 0 zeroes capacity (and the bar).
	defer func(old float64) { WeightLimitRate = old }(WeightLimitRate)
	WeightLimitRate = 2
	if got := PlayerMaxWeight(newKnight()); math.Abs(got-5100) > 1e-9 {
		t.Fatalf("rate 2 = %g, want 5100", got)
	}
	WeightLimitRate = 0
	p = newKnight()
	p.Inv = NewInventory()
	p.Inv.AddItem(1, 1, "dagger", 0, 10000, false, 0)
	if got := PlayerMaxWeight(p); math.Abs(got-0) > 1e-9 {
		t.Fatalf("rate 0 = %g, want 0", got)
	}
	if got := p.Inv.Weight242(PlayerMaxWeight(p)); got != 0 {
		t.Fatalf("rate 0 bar = %d, want 0 (Java calcWeight242 line 86-88)", got)
	}
}

// Java L1PcInstance.resetOriginalStrWeightReduction (3750-3797) / resetOriginalConWeightReduction (3860-3905).
func TestOriginalStatWeightReductionTables(t *testing.T) {
	cases := []struct {
		name     string
		class    int16
		str, con int16
		wantStr  int
		wantCon  int
	}{
		{"prince 14/11", classPrince, 14, 11, 1, 1},
		{"prince 17/10", classPrince, 17, 10, 2, 0},
		{"prince 20/20", classPrince, 20, 20, 3, 1},
		{"prince 21", classPrince, 21, 5, 0, 0},
		{"knight 20/15", classKnight, 20, 15, 0, 1},
		{"knight 20/14", classKnight, 20, 14, 0, 0},
		{"elf 16/15", classElf, 16, 15, 2, 2},
		{"elf 15/14", classElf, 15, 14, 0, 0},
		{"darkelf 13/9", classDarkElf, 13, 9, 2, 1},
		{"darkelf 16/8", classDarkElf, 16, 8, 3, 0},
		{"wizard 9/13", classWizard, 9, 13, 1, 1},
		{"wizard 8/15", classWizard, 8, 15, 0, 2},
		{"dragonknight 16/20", classDragonKnight, 16, 20, 1, 0},
		{"illusionist 18/17", classIllusionist, 18, 17, 1, 1},
		{"illusionist 19/18", classIllusionist, 19, 18, 0, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &PlayerInfo{ClassType: tc.class, Str: tc.str, Con: tc.con}
			if got := OriginalStrWeightReduction(p); got != tc.wantStr {
				t.Fatalf("str reduction = %d, want %d", got, tc.wantStr)
			}
			if got := OriginalConWeightReduction(p); got != tc.wantCon {
				t.Fatalf("con reduction = %d, want %d", got, tc.wantCon)
			}
		})
	}

	// Original stats exclude equipment and buff contributions (Java uses the creation value).
	p := &PlayerInfo{ClassType: classElf, Str: 20, Con: 17}
	p.EquipBonuses.AddStr = 5                                        // original 15 → elf needs 16
	p.AddBuff(&ActiveBuff{SkillID: 999, TicksLeft: 10, DeltaCon: 3}) // original 14 → elf needs 15
	if got := OriginalStrWeightReduction(p); got != 0 {
		t.Fatalf("equipment STR must not count, got %d", got)
	}
	if got := OriginalConWeightReduction(p); got != 0 {
		t.Fatalf("buff CON must not count, got %d", got)
	}
}

// Java L1PcInventory.calcWeight242 lines 73-90.
func TestCalcWeight242Scale(t *testing.T) {
	tests := []struct {
		name   string
		weight int64
		max    float64
		want   byte
	}{
		{"empty", 0, 3000, 0},
		{"no capacity", 100, 0, 0},
		{"half", 1500, 3000, 121},
		{"fractional capacity must not be rounded before ratio", 50, 100.49, 120},
		{"full", 3000, 3000, 242},
		{"over", 9000, 3000, 242},
		{"mp regen stop boundary 120 (1488/3000*242=120.03)", 1488, 3000, 120},
		{"hp regen stop boundary 121 (1500/3000*242=121)", 1500, 3000, 121},
		{"attack stop boundary 197 (2442/3000*242=196.99)", 2442, 3000, 197},
		{"just under attack stop (2430/3000*242=196.02)", 2430, 3000, 196},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalcWeight242(tt.weight, tt.max); got != tt.want {
				t.Fatalf("CalcWeight242(%d, %g) = %d, want %d", tt.weight, tt.max, got, tt.want)
			}
		})
	}
}

// Java L1PcInventory.checkAddItem() lines 110-122:
// weight = getWeight() + templateWeight*count/1000 + 1; negative → WEIGHT_OVER; calcWeight242(weight) >= 242 → WEIGHT_OVER.
func TestIsOverWeightLikeJavaCheckAddItem(t *testing.T) {
	inv := NewInventory()
	inv.AddItem(1, 1, "weights", 0, 1000, true, 0) // 1 unit
	const maxW = 100

	if inv.IsOverWeightAdd(1000, 50, maxW) {
		t.Fatal("1 + 50 + 1 of 100 must not be over weight")
	}
	if inv.IsOverWeightAdd(1000, 97, maxW) {
		t.Fatal("1 + 97 + 1 = 99 of 100 rounds to 240 and must be allowed")
	}
	if !inv.IsOverWeightAdd(1000, 98, maxW) {
		t.Fatal("1 + 98 + 1 = 100 reaches the full bar (242) and must be refused")
	}
	// Sub-unit additions have no 1-unit floor in checkAddItem (only the +1 safety margin).
	if inv.IsOverWeightAdd(1, 1, 3) {
		t.Fatal("adding 1/1000 of a unit must use 0 extra weight like Java checkAddItem")
	}
	// Products that overflow Java's int multiplication are refused (line 111), never wrapped.
	if got := AddWeight(10000, 300_000); got != -1 {
		t.Fatalf("overflowing product should report -1, got %d", got)
	}
	if !inv.IsOverWeightAdd(10000, 300_000, math.MaxInt32) {
		t.Fatal("overflowing product must be over weight regardless of capacity")
	}
	if !inv.IsOverWeight(-1, maxW) {
		t.Fatal("negative add weight must be treated as over weight")
	}
	if inv.IsOverWeightAdd(5000, 400_000, math.MaxInt32) {
		t.Fatal("2e6 units against int32-max capacity must not be reported as over weight")
	}
	if !inv.IsOverWeightAdd(5000, 400_000, 1_000_000) {
		t.Fatal("2e6 units must exceed a 1e6 capacity (int32 wrap would have allowed it)")
	}
}

// Java L1PcInventory.checkEquipped(itemId).
func TestHasEquipped(t *testing.T) {
	inv := NewInventory()
	wing := inv.AddItem(20049, 1, "golden wing", 0, 1000, false, 0)
	if inv.HasEquipped(20049) {
		t.Fatal("unequipped item must not count")
	}
	wing.Equipped = true
	if !inv.HasEquipped(20049) {
		t.Fatal("equipped item must count")
	}
	var nilInv *Inventory
	if nilInv.HasEquipped(20049) {
		t.Fatal("nil inventory must be safe")
	}
}

func TestOriginalWeightBonusDoesNotFollowPermanentOrTemporaryStats(t *testing.T) {
	p := &PlayerInfo{ClassType: classElf, Str: 16, Con: 15, OriginalStr: 15, OriginalCon: 14}
	for _, stat := range []int16{16, 20, 35} {
		p.Str, p.Con = stat, stat
		if OriginalStrWeightReduction(p) != 0 || OriginalConWeightReduction(p) != 0 {
			t.Fatal("level/elixir/equipment/doll stats must not increase the original-stat bonus")
		}
	}
	p.OriginalStr, p.OriginalCon = 16, 15
	if OriginalStrWeightReduction(p) != 2 || OriginalConWeightReduction(p) != 2 {
		t.Fatal("reset creation stats must replace the original bonuses")
	}
	p = &PlayerInfo{ClassType: classKnight, Str: 18, Con: 14}
	p.AddBuff(&ActiveBuff{SkillID: 218, TicksLeft: 10})
	if PlayerMaxWeight(p) != 2550 {
		t.Fatal("Joy of Pain is not Decrease Weight")
	}
	defer func(old float64) { WeightLimitRate = old }(WeightLimitRate)
	WeightLimitRate = 1.0001
	if math.Abs(PlayerMaxWeight(p)-2550.255) > 1e-9 {
		t.Fatal("capacity multiplier must retain Java double precision")
	}
}
