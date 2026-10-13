package scripting

import (
	"testing"

	"go.uber.org/zap"
)

// Java HpRegeneration.regenHp lines 127-134 + isOverWeight lines 198-209 (threshold 121,
// bypass by EXOTIC_VITALIZE / ADDITIONAL_FIRE / equipped 20049; BERSERKERS also zeroes bonus).
func TestHPRegenWeightGatingLikeEddyJava(t *testing.T) {
	engine, err := NewEngine("../../scripts", zap.NewNop())
	if err != nil {
		t.Fatalf("建立 Lua engine 失敗: %v", err)
	}
	t.Cleanup(engine.Close)

	base := HPRegenContext{Level: 1, Con: 10, HPR: 5, Food: 40}
	run := func(mut func(*HPRegenContext)) int {
		ctx := base
		mut(&ctx)
		return engine.CalcHPRegenAmount(ctx)
	}

	if got := run(func(c *HPRegenContext) { c.WeightPct = 120 }); got < 6 {
		t.Fatalf("weight 120 is below the HP threshold (121); want bonus+hpr >= 6, got %d", got)
	}
	if got := run(func(c *HPRegenContext) { c.WeightPct = 121 }); got != 0 {
		t.Fatalf("weight 121 must stop HP regen and drop positive equipment HPR, got %d", got)
	}
	if got := run(func(c *HPRegenContext) { c.WeightPct = 121; c.HPR = -2 }); got != -2 {
		t.Fatalf("negative equipment HPR still applies while overweight, got %d", got)
	}
	for name, mut := range map[string]func(*HPRegenContext){
		"exotic vitalize": func(c *HPRegenContext) { c.WeightPct = 242; c.HasExoticVitalize = true },
		"additional fire": func(c *HPRegenContext) { c.WeightPct = 242; c.HasAdditionalFire = true },
		"golden wing":     func(c *HPRegenContext) { c.WeightPct = 242; c.HasGoldenWing = true },
	} {
		if got := run(mut); got < 6 {
			t.Fatalf("%s must bypass the weight check, got %d", name, got)
		}
	}
	// Food is checked independently of the weight bypass (Java: food < 3 || isOverWeight || BERSERKERS).
	if got := run(func(c *HPRegenContext) { c.WeightPct = 242; c.HasExoticVitalize = true; c.Food = 2 }); got != 0 {
		t.Fatalf("hungry player must not regen even with the weight bypass, got %d", got)
	}
	if got := run(func(c *HPRegenContext) { c.HasBerserkers = true }); got != 0 {
		t.Fatalf("BERSERKERS zeroes the HP bonus, got %d", got)
	}
}

// Java MpRegeneration.regenMp lines 104-109 + isOverWeight lines 118-126 (threshold 120,
// bypass by EXOTIC_VITALIZE / ADDITIONAL_FIRE only).
func TestMPRegenWeightGatingLikeEddyJava(t *testing.T) {
	engine, err := NewEngine("../../scripts", zap.NewNop())
	if err != nil {
		t.Fatalf("建立 Lua engine 失敗: %v", err)
	}
	t.Cleanup(engine.Close)

	base := MPRegenContext{Wis: 12, MPR: 4, Food: 40}
	run := func(mut func(*MPRegenContext)) int {
		ctx := base
		mut(&ctx)
		return engine.CalcMPRegenAmount(ctx)
	}

	if got := run(func(c *MPRegenContext) { c.WeightPct = 119 }); got != 5 {
		t.Fatalf("weight 119: want base 1 + mpr 4 = 5, got %d", got)
	}
	if got := run(func(c *MPRegenContext) { c.WeightPct = 120 }); got != 0 {
		t.Fatalf("weight 120 must stop MP regen, got %d", got)
	}
	if got := run(func(c *MPRegenContext) { c.WeightPct = 242; c.HasAdditionalFire = true }); got != 5 {
		t.Fatalf("ADDITIONAL_FIRE must bypass the MP weight check, got %d", got)
	}
	if got := run(func(c *MPRegenContext) { c.WeightPct = 242; c.HasExoticVitalize = true; c.Food = 2 }); got != 0 {
		t.Fatalf("hungry player must not regen MP even with the weight bypass, got %d", got)
	}
}
