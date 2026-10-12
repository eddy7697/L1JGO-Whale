package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/l1jgo/server/internal/handler"
	"github.com/l1jgo/server/internal/scripting"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

func TestGMSetLevelPreservesBonusesAndUsesBaseConWis(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "core"), 0755); err != nil {
		t.Fatal(err)
	}
	// 可預測且依體質/精神變動的公式，確認傳入的是基礎屬性。
	script := `function exp_for_level(level) return level * 100 end
function calc_init_hp(class, con) return con * 10 end
function calc_init_mp(class, wis) return wis * 10 end
function calc_level_up_hp(class, con) return con end
function calc_level_up_mp(class, wis) return wis end`
	if err := os.WriteFile(filepath.Join(dir, "core", "level.lua"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	engine, err := scripting.NewEngine(dir, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	sess := newSkillTestSession(t, 1)
	p := &world.PlayerInfo{CharID: 1, SessionID: 1, Session: sess, Inv: world.NewInventory(), Con: 21, Wis: 23,
		EquipBonuses: world.EquipStats{AddCon: 2, AddWis: 3, AddHP: 50, AddMP: 30}}
	p.AddBuff(&world.ActiveBuff{SkillID: 1, DeltaCon: 4, DeltaWis: 5, DeltaMaxHP: 100, DeltaMaxMP: 70})
	sys := NewGMCommandSystem(&handler.Deps{Scripting: engine})
	sys.SetLevel(sess, p, 3)
	// Base CON/WIS both 15: initial 150 + two levels * 15 = 180.
	if p.MaxHP != 330 || p.MaxMP != 280 || p.HP != 330 || p.MP != 280 {
		t.Fatalf("等級重算應保留 150 HP / 100 MP 外加值，got HP=%d/%d MP=%d/%d", p.HP, p.MaxHP, p.MP, p.MaxMP)
	}
	if p.Con != 21 || p.Wis != 23 || p.Level != 3 || p.Exp != 300 || !p.Dirty {
		t.Fatalf("重算不可修改現有體精，且應儲存新等級：%+v", p)
	}
	// 與正式存檔相同地扣除裝備/buff後，必須留下正確基礎值。
	if p.MaxHP-int32(p.EquipBonuses.AddHP)-p.GetBuff(1).DeltaMaxHP != 180 || p.MaxMP-int32(p.EquipBonuses.AddMP)-p.GetBuff(1).DeltaMaxMP != 180 {
		t.Fatal("移除加成後的基礎血魔不應因 .level 而被重複扣除")
	}
}
