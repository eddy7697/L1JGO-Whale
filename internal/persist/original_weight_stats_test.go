package persist

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Opt-in PostgreSQL round trip. Only a unique fixture schema is modified.
func TestOriginalWeightStatsPostgresRoundTrip(t *testing.T) {
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
	schema := fmt.Sprintf("codex_weight_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("fixture cleanup: %v", err)
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
	apply := func(file string) {
		t.Helper()
		migration, err := migrations.ReadFile("migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
			t.Fatal(err)
		}
	}
	apply("001_phase1.sql")
	_, err = pool.Exec(ctx, `INSERT INTO accounts(name,password_hash) VALUES('fixture','unused');
		INSERT INTO characters(account_name,name,class_type,sex,class_id,str,dex,con,wis,cha,intel,hp,mp,max_hp,max_mp)
		VALUES('fixture','legacy',2,0,0,21,12,17,12,9,12,20,10,20,10)`)
	if err != nil {
		t.Fatal(err)
	}
	apply("037_original_weight_stats.sql")
	repo := NewCharacterRepo(&DB{Pool: pool})
	assertOriginal := func(name string, str, con int16) CharacterRow {
		t.Helper()
		login, err := repo.LoadByName(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		if login == nil || login.OriginalStr != str || login.OriginalCon != con {
			t.Fatalf("login did not load original stats for %s: %+v", name, login)
		}
		rows, err := repo.LoadByAccount(ctx, "fixture")
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range rows {
			if c.Name == name {
				if c.OriginalStr != str || c.OriginalCon != con {
					t.Fatalf("%s original=%d/%d want=%d/%d", name, c.OriginalStr, c.OriginalCon, str, con)
				}
				return c
			}
		}
		t.Fatalf("missing character %s", name)
		return CharacterRow{}
	}
	assertOriginal("legacy", 21, 17) // Old schemas have no historical creation stats.
	c := CharacterRow{AccountName: "fixture", Name: "new", ClassType: 2, Str: 15, Con: 14, Dex: 12, Wis: 12, Cha: 9, Intel: 12, HP: 20, MP: 10, MaxHP: 20, MaxMP: 10}
	if err = repo.Create(ctx, &c); err != nil {
		t.Fatal(err)
	}
	c = assertOriginal("new", 15, 14)
	c.Str, c.Con = 25, 24 // Level points/elixirs must not change creation bonuses.
	if err = repo.SaveCharacter(ctx, &c); err != nil {
		t.Fatal(err)
	}
	c = assertOriginal("new", 15, 14)
	if c.Str != 25 || c.Con != 24 {
		t.Fatal("permanent stats did not save")
	}
	c.OriginalStr, c.OriginalCon = 16, 15 // A real character reset replaces originals.
	if err = repo.SaveCharacter(ctx, &c); err != nil {
		t.Fatal(err)
	}
	c = assertOriginal("new", 16, 15)
	c.OriginalStr, c.OriginalCon = 0, 0 // A legacy partial save must not erase them.
	if err = repo.SaveCharacter(ctx, &c); err != nil {
		t.Fatal(err)
	}
	assertOriginal("new", 16, 15)
}
