package world

import "testing"

func TestGMIdentityMatchesJavaCharacterAccessLevels(t *testing.T) {
	for _, level := range []int16{-1, 0, 99, 100, 199, 200, 201, 32767} {
		p := &PlayerInfo{AccessLevel: level}
		if p.IsGM() != (level == 200) || p.IsMonitor() != (level == 100) {
			t.Fatalf("角色權限 %d 不符合 Java 身分", level)
		}
		if p.CanUseGMCommands() != (level == 100 || level == 200) {
			t.Fatalf("非標準角色權限 %d 不應授權管理指令", level)
		}
	}
	p := &PlayerInfo{AccessLevel: 200, HP: 20, MaxHP: 100, MP: 20, MaxMP: 50}
	p.ApplyDamage(10)
	if p.HP != 10 {
		t.Fatal("GM 仍應承受非致命傷害")
	}
	p.ApplyDamage(10)
	if p.HP != 100 {
		t.Fatal("GM 致命傷應回滿 HP")
	}
	p.SpendMP(10)
	if p.MP != 50 {
		t.Fatal("GM 扣魔應回滿 MP")
	}
	p.SetGM(false)
	if p.IsGM() || !p.CanUseGMCommands() {
		t.Fatal("gm off 只關閉遊戲特權，仍能使用 gm on")
	}
	p.ApplyDamage(100)
	p.SpendMP(10)
	if p.HP != 0 || p.MP != 40 {
		t.Fatal("gm off 應恢復一般傷害及魔力消耗")
	}
	p.SetGM(true)
	if !p.IsGM() {
		t.Fatal("gm on 應恢復身分")
	}
}

func TestGMInvisibleSeparateFromSpellInvisibility(t *testing.T) {
	p := &PlayerInfo{CharID: 1, AccessLevel: 200, Invisible: true}
	viewer := &PlayerInfo{CharID: 2}
	if p.IsGMInvisible() {
		t.Fatal("GM 使用普通隱身術不應視為 GM 隱身")
	}
	if viewer.CanSeePlayer(p) {
		t.Fatal("一般觀察者不應看到隱身術")
	}
	viewer.GMFindInvis = true
	if !viewer.CanSeePlayer(p) {
		t.Fatal("findinvis 應看見普通隱身術")
	}
	p.GMInvisible = true
	p.Invisible = false
	if !p.IsInvisible() || viewer.CanSeePlayer(p) {
		t.Fatal("解除隱身術不能揭露 GM 隱身")
	}
	if !p.CanSeePlayer(p) {
		t.Fatal("GM 始終能看到自己")
	}
	p.GMInvisible = false
	if !viewer.CanSeePlayer(p) {
		t.Fatal("取消 GM 隱身後應恢復可見")
	}
	p.ShowID = 1
	if viewer.CanSeePlayer(p) {
		t.Fatal("findinvis 不可越過副本隔離")
	}
}
