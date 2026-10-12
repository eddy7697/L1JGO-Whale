package system

import (
	"testing"

	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/world"
)

func TestJavaDebuffDiceAndClassAdjustment(t *testing.T) {
	sk := &data.SkillInfo{SkillID: 33, ProbabilityDice: 8}
	roll := func(n int) int { return n - 1 }
	// Level 40 wizard: 10 magic levels + INT 18 bonus 3 + wizard 1 => 14d8.
	p := &world.PlayerInfo{Level: 40, ClassType: 3, Intel: 18, OriginalMagicHit: 2}
	if got := javaPlayerDebuffProbability(p, 40, 60, sk, roll); got != 56 {
		t.Fatalf("probability=%d want=56", got)
	}
	if got := javaPlayerDebuffProbability(p, 40, 999, sk, roll); got >= 0 {
		t.Fatal("high MR must not retain an artificial 10% floor")
	}
}

func TestJavaDebuffLevelFormulaAndStatusResistances(t *testing.T) {
	sk := &data.SkillInfo{SkillID: 157, ProbabilityDice: 20, ProbabilityValue: 30}
	prob := javaDebuffProbability(sk, 60, 50, 0, 0, 0, 2, 999, 10, func(int) int { t.Fatal("level formula must not roll dice"); return 0 })
	if prob != 54 {
		t.Fatalf("earth bind chance=%d want=54", prob)
	}
	p := &world.PlayerInfo{RegistSustain: 1, RegistStun: 2, RegistStone: 3, RegistSleep: 4, RegistFreeze: 5, RegistBlind: 6}
	for id, want := range map[int32]int{157: 49, 87: 46, 33: 47, 66: 46, 50: 45, 20: 44, 40: 44, 103: 44} {
		if got := javaApplyStatusResistance(50, id, p); got != want {
			t.Fatalf("skill %d chance=%d want=%d", id, got, want)
		}
	}
}

func TestJavaDebuffHighLevelAndIntelligence(t *testing.T) {
	sk := &data.SkillInfo{SkillID: 33, ProbabilityDice: 8}
	for _, tc := range []struct {
		class int16
		intel int16
		want  int
	}{
		{3, 18, 112}, // wizard: (10 + 3 + 1) d8
		{6, 18, 64},  // illusionist: (6 + 3 - 1) d8
		{3, 51, 296}, // INT > 50 uses INT - 25, without a bonus cap
	} {
		p := &world.PlayerInfo{Level: 80, ClassType: tc.class, Intel: tc.intel}
		if got := javaPlayerDebuffProbability(p, 80, 0, sk, func(n int) int { return n - 1 }); got != tc.want {
			t.Fatalf("class=%d INT=%d probability=%d want=%d", tc.class, tc.intel, got, tc.want)
		}
	}
}

func TestJavaHighStoneResistanceBlocksMonsterCurse(t *testing.T) {
	s, _, p := newJavaPotionTest(t)
	p.RegistStone = 10000
	npc := &world.NpcInfo{Level: 50, Intel: 18}
	s.ApplyNpcDebuffFrom(npc, p, s.deps.Skills.Get(33), 10)
	if p.CurseType != 0 || p.HasBuff(33) {
		t.Fatal("monster curse bypassed stone resistance")
	}
}
