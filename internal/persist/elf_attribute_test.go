package persist

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Only a unique fixture schema is modified; never touches live characters.
func TestElfAttributePostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("L1JGO_TEST_POINTS_DSN")
	if dsn == "" {
		t.Skip("set L1JGO_TEST_POINTS_DSN for an isolated PostgreSQL fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("codex_elf_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("cleanup: %v", err)
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
	for _, file := range []string{"001_phase1.sql", "003_known_spells.sql", "037_original_weight_stats.sql", "038_hell_time.sql", "039_elf_attribute.sql"} {
		migration, err := migrations.ReadFile("migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, "INSERT INTO accounts(name,password_hash) VALUES('fixture','unused')"); err != nil {
		t.Fatal(err)
	}
	repo := NewCharacterRepo(&DB{Pool: pool})
	c := &CharacterRow{AccountName: "fixture", Name: "elf", ClassType: 2, Str: 12, Dex: 12, Con: 12, Wis: 12, Cha: 12, Intel: 12, HP: 20, MaxHP: 20, MP: 10, MaxMP: 10}
	if err = repo.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	assertState := func(attr int16, spells []int32) *CharacterRow {
		t.Helper()
		got, err := repo.LoadByName(ctx, "elf")
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || got.ElfAttr != attr {
			t.Fatalf("login row=%+v want attr=%d", got, attr)
		}
		list, err := repo.LoadByAccount(ctx, "fixture")
		if err != nil || len(list) != 1 || list[0].ElfAttr != attr {
			t.Fatalf("character list=%v err=%v", list, err)
		}
		learned, err := repo.LoadKnownSpells(ctx, "elf")
		if err != nil || !slices.Equal(learned, spells) {
			t.Fatalf("spells=%v err=%v want=%v", learned, err, spells)
		}
		return got
	}
	assertState(0, nil)
	if err = repo.SaveElfAttribute(ctx, "elf", 2, []int32{129, 148}); err != nil {
		t.Fatal(err)
	}
	c = assertState(2, []int32{129, 148})
	c.HP = 15
	if err = repo.SaveCharacter(ctx, c); err != nil {
		t.Fatal(err)
	}
	assertState(2, []int32{129, 148})
	if err = repo.SaveElfAttribute(ctx, "elf", 0, []int32{129}); err != nil {
		t.Fatal(err)
	}
	assertState(0, []int32{129})
	if err = repo.SaveElfAttribute(ctx, "elf", 3, nil); err == nil {
		t.Fatal("invalid element accepted")
	}
	assertState(0, []int32{129})
	if err = repo.SaveElfAttribute(ctx, "missing", 4, nil); err == nil {
		t.Fatal("missing character accepted")
	}
}
