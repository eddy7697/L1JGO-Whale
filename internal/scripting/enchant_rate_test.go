package scripting

import (
	"testing"

	"go.uber.org/zap"
)

// Java Enchant.scrollOfEnchantWeapon：安定值以上成功率為固定值，不隨目前等級變化。
// C=10 時 +8 以下 (100+30)/3=43 → rnd(1..100)<43 成功率 42%；
// +9 以上 (100+30)/6=21 → 成功 20%，21%≤rnd<42 為「藍光無事」。
func TestWeaponEnchantRateIsFlatLikeJava(t *testing.T) {
	engine, err := NewEngine("../../scripts", zap.NewNop())
	if err != nil {
		t.Fatalf("建立 Lua engine 失敗: %v", err)
	}
	t.Cleanup(engine.Close)

	const trials = 20000
	rate := func(lvl int) (success, nochange float64) {
		var s, n int
		for i := 0; i < trials; i++ {
			r := engine.CalcEnchant(EnchantContext{
				ScrollBless: 1, EnchantLvl: lvl, SafeEnchant: 6, Category: 1,
				WeaponChance: 0.10, ArmorChance: 0.10,
			})
			switch r.Result {
			case "success":
				s++
			case "nochange":
				n++
			}
		}
		return float64(s) / trials, float64(n) / trials
	}

	for _, lvl := range []int{6, 7, 8} {
		s, n := rate(lvl)
		if s < 0.38 || s > 0.46 {
			t.Fatalf("+%d 成功率應約 42%%（Java 固定值），got %.3f", lvl, s)
		}
		if n != 0 {
			t.Fatalf("+%d 不應有藍光無事，got %.3f", lvl, n)
		}
	}
	for _, lvl := range []int{9, 12} {
		s, n := rate(lvl)
		if s < 0.16 || s > 0.24 {
			t.Fatalf("+%d 成功率應約 20%%，got %.3f", lvl, s)
		}
		if n < 0.17 || n > 0.25 {
			t.Fatalf("+%d 藍光無事應約 21%%，got %.3f", lvl, n)
		}
	}
}
