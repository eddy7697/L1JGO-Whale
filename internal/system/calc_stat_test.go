package system

import (
	"testing"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/world"
)

// Java CalcStat.calcAc：10 - level/除數，除數依敏捷區間 8/7/6/5/4。
func TestCalcAcLikeJava(t *testing.T) {
	for _, tc := range []struct {
		level, dex int16
		want       int16
	}{
		{1, 9, 10}, {8, 9, 9}, {80, 9, 0},
		{7, 10, 9}, {7, 12, 9}, {6, 12, 10},
		{6, 13, 9}, {6, 15, 9},
		{5, 16, 9}, {5, 17, 9},
		{4, 18, 9}, {52, 25, -3}, {99, 35, -14},
	} {
		if got := CalcAcLikeJava(10, tc.level, tc.dex); got != tc.want {
			t.Errorf("calcAc(level=%d dex=%d)=%d want %d", tc.level, tc.dex, got, tc.want)
		}
	}
}

// Java CalcStat.calcStatMr 區間表。
func TestCalcStatMrLikeJava(t *testing.T) {
	for wis, want := range map[int16]int16{
		0: 0, 14: 0, 15: 3, 16: 3, 17: 6, 18: 10, 19: 15, 20: 21, 21: 28, 22: 37, 23: 47,
		24: 50, 29: 50, 30: 52, 34: 52, 35: 55, 39: 55, 40: 59, 43: 59, 44: 62, 46: 62, 47: 64, 49: 64, 50: 65, 60: 65,
	} {
		if got := CalcStatMrLikeJava(wis); got != want {
			t.Errorf("calcStatMr(%d)=%d want %d", wis, got, want)
		}
	}
}

// Java resetBaseMr：職業值 + calcStatMr(wis) + level/2。
func TestCalcBaseMrLikeJava(t *testing.T) {
	for _, tc := range []struct {
		class, level, wis int16
		want              int16
	}{
		{0, 1, 11, 10}, {1, 10, 12, 5}, {2, 50, 18, 60}, {3, 30, 20, 51}, {4, 9, 10, 14}, {5, 20, 16, 31}, {6, 51, 24, 95},
	} {
		if got := CalcBaseMrLikeJava(tc.class, tc.level, tc.wis); got != tc.want {
			t.Errorf("baseMr(class=%d level=%d wis=%d)=%d want %d", tc.class, tc.level, tc.wis, got, tc.want)
		}
	}
}

// ResetBaseAC/MR 只改動基礎值差額，裝備與 buff 加成保持不變。
func TestResetBaseStatsKeepsEquipmentAndBuffDeltas(t *testing.T) {
	deps := &handler.Deps{Config: &config.Config{Gameplay: config.GameplayConfig{BaseAC: 10}}}
	p := &world.PlayerInfo{ClassType: 2, Level: 1, Dex: 18, Wis: 18}
	ResetBaseStats(p, deps)
	if p.AC != 10 || p.BaseAC != 10 || p.MR != 35 || p.BaseMR != 35 {
		t.Fatalf("1 級妖精初始 AC/MR 錯誤: AC=%d base=%d MR=%d base=%d", p.AC, p.BaseAC, p.MR, p.BaseMR)
	}
	// 穿上 -5 AC、+10 MR、+2 WIS 的裝備後升到 52 級（DEX 18 → level/4 = 13）。
	p.AC -= 5
	p.MR += 10
	p.Wis += 2
	p.EquipBonuses.AddWis = 2
	p.Level = 52
	ResetBaseStats(p, deps)
	// AC: 10-13 = -3 基礎，再加裝備 -5。MR: 25 + calcStatMr(20)=21 + 26 = 72，再加裝備 10。
	if p.AC != -8 || p.BaseAC != -3 || p.MR != 82 || p.BaseMR != 72 {
		t.Fatalf("升級後 AC/MR 錯誤: AC=%d base=%d MR=%d base=%d", p.AC, p.BaseAC, p.MR, p.BaseMR)
	}
	// 基礎 AC 用原始敏捷：裝備 +4 DEX 不應改變基礎 AC。
	p.Dex += 4
	p.EquipBonuses.AddDex = 4
	ResetBaseAC(p, deps)
	if p.BaseAC != -3 || p.AC != -8 {
		t.Fatalf("裝備敏捷不應影響基礎 AC: AC=%d base=%d", p.AC, p.BaseAC)
	}
	if ResetBaseMR(p) {
		t.Fatal("無變化時 ResetBaseMR 應回 false")
	}
}

// Java ExpTable.getPenaltyRate：50 級前 1.0，之後 1/Lv{n}Exp。
func TestExpPenaltyRateLikeJava(t *testing.T) {
	for level, want := range map[int16]float64{
		1: 1, 49: 1, 50: 1, 64: 1, 65: 0.5, 69: 0.5, 70: 0.25, 74: 0.25, 75: 0.125, 79: 1.0 / 16, 80: 1.0 / 32, 90: 1.0 / 32768, 99: 1.0 / 16777216, 120: 1.0 / 16777216,
	} {
		if got := ExpPenaltyRateLikeJava(level, nil); got != want {
			t.Errorf("penalty(%d)=%v want %v", level, got, want)
		}
	}
	// 設定檔可覆寫除數表；不足部分沿用預設。
	override := []int{2, 0}
	if got := ExpPenaltyRateLikeJava(50, override); got != 0.5 {
		t.Fatalf("override level 50 = %v", got)
	}
	if got := ExpPenaltyRateLikeJava(51, override); got != 1 {
		t.Fatalf("zero divisor should fall back to default: %v", got)
	}
	if got := ExpPenaltyRateLikeJava(70, override); got != 0.25 {
		t.Fatalf("missing override should fall back to default: %v", got)
	}
}
