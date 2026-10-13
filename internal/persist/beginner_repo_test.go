package persist

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Java Beginner.GiveItem 依 isCrown/isKnight/... 決定第二個 activate 代碼，
// 任何不認得的職業回退為 'A'。
func TestBeginnerActivateCode(t *testing.T) {
	for _, tc := range []struct {
		classType int16
		want      string
	}{
		{0, "P"}, {1, "K"}, {2, "E"}, {3, "W"}, {4, "D"}, {5, "R"}, {6, "I"},
		{7, "A"}, {-1, "A"},
	} {
		if got := beginnerActivateCode(tc.classType); got != tc.want {
			t.Errorf("classType=%d got=%q want=%q", tc.classType, got, tc.want)
		}
	}
}

// Java 插入 character_items 時：count/enchantlvl/charge_count 取自 beginner 列，
// bless 固定 1、未裝備、未鑑定(is_id=0)、耐久 0；資料表的 bless 欄位被忽略。
func TestBeginnerItemInsertValues(t *testing.T) {
	row := BeginnerRow{ItemID: 224, Count: 3, ChargeCount: 7, EnchantLvl: 2, Bless: 0, Activate: "W"}
	got := beginnerInsertValues(42, row)
	want := beginnerInsert{CharID: 42, ItemID: 224, Count: 3, EnchantLvl: 2, ChargeCount: 7, Bless: 1, Equipped: false, Identified: false, Durability: 0}
	if got != want {
		t.Fatalf("got=%+v want=%+v", got, want)
	}
}

// 選擇性整合測試：在獨立 schema 內跑完整 migration，驗證 beginner 資料表與
// GiveItems 寫入 character_items 的值與 Java 一致。production schema 不在 search_path。
func TestBeginnerPostgresGiveItems(t *testing.T) {
	dsn := os.Getenv("L1JGO_TEST_BEGINNER_DSN")
	if dsn == "" {
		t.Skip("set L1JGO_TEST_BEGINNER_DSN to a PostgreSQL DSN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("codex_beginner_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("fixture cleanup failed: %v", err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	var total int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM beginner`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 35 {
		t.Fatalf("beginner seed rows=%d want 35", total)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO accounts (name, password_hash) VALUES ('codex_beginner', 'x')`); err != nil {
		t.Fatal(err)
	}
	repo := &CharacterRepo{db: &DB{Pool: pool}}
	newChar := func(name string, classType int16) int32 {
		row := &CharacterRow{AccountName: "codex_beginner", Name: name, ClassType: classType, Level: 1, HP: 1, MP: 1, MaxHP: 1, MaxMP: 1, AC: 10, MapID: 2005}
		if err := repo.Create(ctx, row); err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	beginner := NewBeginnerRepo(&DB{Pool: pool})

	knight := newChar("codex_knight", 1)
	inserted, errs := beginner.GiveItems(ctx, knight, 1)
	if len(errs) != 0 || inserted != 10 {
		t.Fatalf("knight inserted=%d errs=%v want 10 (3 'A' + 7 'K')", inserted, errs)
	}
	rows, err := pool.Query(ctx, `SELECT item_id, count, enchant_lvl, bless, equipped, identified, durability, charge_count, obj_id FROM character_items WHERE char_id = $1 ORDER BY id`, knight)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []int32
	for rows.Next() {
		var itemID, count, objID int32
		var enchant, bless, durability, charge int16
		var equipped, identified bool
		if err := rows.Scan(&itemID, &count, &enchant, &bless, &equipped, &identified, &durability, &charge, &objID); err != nil {
			t.Fatal(err)
		}
		if count != 1 || enchant != 0 || bless != 1 || equipped || identified || durability != 0 || charge != 0 || objID != 0 {
			t.Errorf("item %d: count=%d enchant=%d bless=%d equipped=%v identified=%v durability=%d charge=%d obj_id=%d", itemID, count, enchant, bless, equipped, identified, durability, charge, objID)
		}
		got = append(got, itemID)
	}
	want := []int32{40005, 40005, 40641, 35, 48, 147, 105, 174, 7, 49309}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("knight items=%v want %v", got, want)
	}

	// 未知職業回退 'A'：只選到 3 件全職業物品（characters 表限制 class_type 0..6，
	// 故以查詢驗證而非建立角色）。
	if rows, err := beginner.LoadForClass(ctx, 9); err != nil || len(rows) != 3 {
		t.Fatalf("fallback rows=%d err=%v want 3", len(rows), err)
	}
}
