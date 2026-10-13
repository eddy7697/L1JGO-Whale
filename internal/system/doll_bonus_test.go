package system

import (
	stdnet "net"
	"testing"

	"github.com/l1jgo/server/internal/handler"
	l1net "github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

func newDollTestSession(t *testing.T, id uint64) *l1net.Session {
	t.Helper()
	client, server := stdnet.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	sess := l1net.NewSession(server, id, 8, 8, 0, zap.NewNop())
	t.Cleanup(sess.Close)
	// 在背景把 pipe 客戶端讀光，避免測試發包阻塞。
	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := client.Read(buf); err != nil {
				return
			}
		}
	}()
	return sess
}

// MISS-P1-006: 確認 weight / dmg_reduction 兩個 doll power 真的會套用到玩家狀態。
func TestApplyDollBonuses_WeightAndDmgReduction(t *testing.T) {
	sys := &DollSystem{}
	player := &world.PlayerInfo{
		Str: 30, Con: 30,
	}
	doll := &world.DollInfo{
		BonusWeight:    200,
		BonusDmgReduce: 3,
	}

	sys.applyDollBonuses(player, doll)

	if player.WeightBonus != 200 {
		t.Fatalf("WeightBonus 應為 200，實際 %d", player.WeightBonus)
	}
	if player.EquipBonuses.DmgReduction != 3 {
		t.Fatalf("EquipBonuses.DmgReduction 應為 3，實際 %d", player.EquipBonuses.DmgReduction)
	}

	sys.removeDollBonuses(player, doll)

	if player.WeightBonus != 0 {
		t.Fatalf("解散後 WeightBonus 應歸零，實際 %d", player.WeightBonus)
	}
	if player.EquipBonuses.DmgReduction != 0 {
		t.Fatalf("解散後 DmgReduction 應歸零，實際 %d", player.EquipBonuses.DmgReduction)
	}
}

// Java L1PcInstance.getMaxWeight 第 2944-2946、2956-2958 行：娃娃負重值是百分比，
// 與其他百分比加總後乘上 150*floor(0.6*STR+0.4*CON+1)。
func TestPlayerMaxWeight_IncludesWeightBonus(t *testing.T) {
	p := &world.PlayerInfo{Str: 30, Con: 30}
	base := world.PlayerMaxWeight(p)
	p.WeightBonus = 20
	withBonus := world.PlayerMaxWeight(p)
	want := float64(world.MaxWeight(30, 30)) * 20 / 100
	if withBonus-base != want {
		t.Fatalf("娃娃 weight 20 應讓最大負重 +20%% 基礎值（%g），實際差距 %g", want, withBonus-base)
	}
}

// MISS-P1-006: hp_regen_tick 每 Interval ticks 觸發一次，回復 Amount 點到主人 HP。
func TestTickDollRegen_HPApplied(t *testing.T) {
	sys := &CompanionAISystem{}
	sess := newDollTestSession(t, 1)
	master := &world.PlayerInfo{
		Session: sess,
		HP:      50,
		MaxHP:   100,
		MP:      0,
		MaxMP:   100,
	}
	doll := &world.DollInfo{
		RegenHPAmount:   20,
		RegenHPInterval: 5,
	}

	// 前 4 個 tick 不會觸發
	for i := 0; i < 4; i++ {
		sys.tickDollRegen(doll, master)
	}
	if master.HP != 50 {
		t.Fatalf("Interval 未到時不應回血，實際 HP=%d", master.HP)
	}

	// 第 5 個 tick 觸發回復
	sys.tickDollRegen(doll, master)
	if master.HP != 70 {
		t.Fatalf("第 5 個 tick 應回 20 HP（50→70），實際 %d", master.HP)
	}
	if doll.RegenHPCounter != 0 {
		t.Fatalf("觸發後 Counter 應歸零，實際 %d", doll.RegenHPCounter)
	}
}

// MISS-P1-006: 回復量超過 MaxHP 時必須截斷。
func TestTickDollRegen_HPCapsAtMax(t *testing.T) {
	sys := &CompanionAISystem{}
	master := &world.PlayerInfo{
		Session: newDollTestSession(t, 2),
		HP:      95,
		MaxHP:   100,
	}
	doll := &world.DollInfo{
		RegenHPAmount:   20,
		RegenHPInterval: 1,
	}
	sys.tickDollRegen(doll, master)
	if master.HP != 100 {
		t.Fatalf("HP 應截斷在 MaxHP=100，實際 %d", master.HP)
	}
}

// MISS-P1-006: mp_regen_tick 邏輯與 HP 對稱。
func TestTickDollRegen_MPApplied(t *testing.T) {
	sys := &CompanionAISystem{}
	master := &world.PlayerInfo{
		Session: newDollTestSession(t, 3),
		HP:      100,
		MaxHP:   100,
		MP:      30,
		MaxMP:   100,
	}
	doll := &world.DollInfo{
		RegenMPAmount:   10,
		RegenMPInterval: 3,
	}
	for i := 0; i < 2; i++ {
		sys.tickDollRegen(doll, master)
	}
	if master.MP != 30 {
		t.Fatalf("Interval 未到時不應回 MP，實際 %d", master.MP)
	}
	sys.tickDollRegen(doll, master)
	if master.MP != 40 {
		t.Fatalf("第 3 tick 應回 10 MP（30→40），實際 %d", master.MP)
	}
}

// MISS-P1-006: Amount=0 或 Interval=0 不應觸發回復。
func TestTickDollRegen_NoOpWhenDisabled(t *testing.T) {
	sys := &CompanionAISystem{}
	master := &world.PlayerInfo{
		Session: newDollTestSession(t, 4),
		HP:      50,
		MaxHP:   100,
		MP:      50,
		MaxMP:   100,
	}
	doll := &world.DollInfo{
		// 完全不啟用週期性回復
	}
	for i := 0; i < 10; i++ {
		sys.tickDollRegen(doll, master)
	}
	if master.HP != 50 || master.MP != 50 {
		t.Fatalf("關閉週期回復時 HP/MP 不應變動，實際 HP=%d MP=%d", master.HP, master.MP)
	}
	if doll.RegenHPCounter != 0 || doll.RegenMPCounter != 0 {
		t.Fatalf("關閉時 Counter 也不應遞增，實際 HP=%d MP=%d", doll.RegenHPCounter, doll.RegenMPCounter)
	}
}

// MISS-P1-006: 主人死亡時暫停回復（Java 行為對照）。
func TestTickDollRegen_PausedWhenMasterDead(t *testing.T) {
	sys := &CompanionAISystem{}
	master := &world.PlayerInfo{
		Session: newDollTestSession(t, 5),
		HP:      0,
		MaxHP:   100,
		Dead:    true,
	}
	doll := &world.DollInfo{
		RegenHPAmount:   20,
		RegenHPInterval: 1,
	}
	sys.tickDollRegen(doll, master)
	if master.HP != 0 {
		t.Fatalf("主人死亡時不應回血，實際 HP=%d", master.HP)
	}
}

// 確保此 helper 真的被引用，避免 lint 抱怨。
var _ = handler.SendHpUpdate
